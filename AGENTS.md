# AGENTS.md

## Project Overview

FBS (Framework Battery Saver) is a power-management tool for the Framework
Laptop 13 (AMD Ryzen AI 9 HX 370) on Arch / Omarchy. It has two parts:

- **`battery-saver.sh`** — a privileged CLI that toggles power-saving on/off:
  it caps the CPU frequency, sets the `powersave` governor, switches the
  power-profiles-daemon profile, and reversibly disables ACPI wakeup sources.
  It also installs a systemd boot unit so those settings survive a reboot.
- **`fbs`** — an unprivileged Bubble Tea TUI that shows live power draw, CPU
  frequency, battery (with time estimate), and the active power profile, plus a
  large toggle. It reads everything from sysfs/procfs and shells out to the
  script for the privileged toggle.

## Code Architecture

```
cmd/fbs/main.go          Entry point; just calls ui.Run().
internal/power/          Metric reading + script control (no UI, no root).
  scraper.go               Reads sysfs/procfs into Snapshot; ring-buffer history.
  control.go               Locates battery-saver.sh, detects saver state, reads
                           the power profile over D-Bus (busctl) and the boot
                           unit's enable state (systemctl is-enabled).
  devices.go               Reads backlight % and rfkill (Wi-Fi/Bluetooth) state.
  ring.go                  Fixed-size ring buffer for sparkline history.
internal/ui/             Bubble Tea program and rendering.
  app.go                   Root tea.Model: tick loop, key/mouse handling, view
                           mode, the sudo toggles via tea.ExecProcess.
  dashboard.go             Pure view over a Snapshot: header, metric cards,
                           toggle, footer; responsive width math.
  extras.go                Secondary panel (key `e`): Wi-Fi/Bluetooth toggles,
                           brightness control, and the boot-persistence status.
  metric_card.go           Bordered title/value/sub panel.
  toggle.go                The large on/off button (with hit-test height).
  styles.go                Palette, sparkline, and meter helpers.
battery-saver.sh         The privileged on/off/status CLI.
install.sh               curl|sh installer (downloads a release tarball).
```

## Design and Concepts

### Privilege model

The TUI runs as the normal user and never needs root: all metrics come from
world-readable sysfs/procfs (battery gauge, `cpufreq`, RAPL where permitted).
Only the *toggle* is privileged, so the TUI shells out to
`sudo battery-saver.sh on|off` via `tea.ExecProcess`, which suspends the TUI,
lets `sudo` prompt on the real terminal, then resumes. Keep this split — do not
move privileged operations into the Go binary.

### Saver state and wakeup tracking

The state file `~/.local/state/battery-saver/disabled-wakeups` is the single
authoritative marker of saver mode: `on` creates it, `off` removes it, and both
the script and the TUI (`power.SaverActive`) treat its *presence* as "saver on".
The file records exactly which `/proc/acpi/wakeup` devices `on` disabled, so
`off` re-enables only those (writing to that file *toggles*, so state must be
tracked). When run via `sudo` the script resolves the invoking user
(`SUDO_USER`) and `chown`s the state dir back so the unprivileged TUI can read
and remove it.

### Boot persistence

The freq cap (`scaling_max_freq`) and the ACPI wakeup disables do **not** survive
a reboot — the kernel resets them to firmware defaults — but the marker file in
`~/.local/state` does. Without re-applying, the TUI would (correctly) read "saver
on" while the cap is gone. Two mechanisms close that gap:

- **systemd boot unit** (`/etc/systemd/system/fbs-restore.service`): a root
  oneshot that runs `battery-saver.sh restore <user>` at boot, ordered
  `After=power-profiles-daemon.service` and guarded by `ConditionPathExists` on
  the marker — so it is a no-op while saver is off, and `off` needn't touch it.
  `cmd_on` installs+enables it idempotently (auto, since `on` already runs under
  sudo); `uninstall-service` removes it. `restore` re-runs the shared `apply_on`
  only when the marker is present, taking the target user as an argument because
  it runs as root with no `SUDO_USER`. All elevation in this path goes through
  `run_priv` (sudo only when not already root) so the boot run never prompts.
- **TUI launch drift-check** (belt-and-suspenders): `Snapshot.CapDrifted()`
  reports saver-on-but-cap-not-applied by comparing the active cap against the
  stable ceiling (`amd_pstate_max_freq`, *not* the dynamic `cpuinfo_max_freq`);
  on the first scrape `app.go` re-applies once via `sudo battery-saver.sh on`.

The Extras panel (key `e`) shows the boot unit's `systemctl is-enabled` state
(`power.BootServiceState`, read-only) so the user can confirm persistence is set
up; it is informational only — the unit is installed automatically by `on`.

The power profile and `powersave` governor are *not* re-applied for persistence:
power-profiles-daemon already saves the profile across reboots, and `powersave`
is the amd_pstate active-mode default the governor returns to on its own.

### Radios and brightness

Wi-Fi and Bluetooth state is read root-free from `/sys/class/rfkill/*` (a radio
is "off" when soft- or hard-blocked; hard blocks can't be cleared in software).
Toggling them needs root, so it goes through `battery-saver.sh wifi|bluetooth
on|off` (rfkill) over the same suspend-and-`sudo` path as the saver toggle.
Brightness is read from `/sys/class/backlight/*` but *changed* via
`brightnessctl`, which works unprivileged via logind — so brightness keys run
in-process without suspending the TUI or prompting for a password.

### amd_pstate quirks (the HX 370)

The CPU uses the `amd_pstate` driver in **active** mode. Two behaviours the code
deliberately works around — preserve these:

- The governor is always `powersave` by design; performance is steered by the
  EPP hint (driven by the power profile), not by switching to `performance`.
  Capping the max frequency is where the battery win comes from.
- `cpuinfo_max_freq` is **dynamic** — it tracks the current scaling cap, so it
  reads 2.0 GHz while capped. To uncap, read `amd_pstate_max_freq` (the stable
  ~5.16 GHz ceiling) instead, and never infer saver state from the cap.

### Reading the power profile

Read the active profile over the system D-Bus with `busctl`
(`org.freedesktop.UPower.PowerProfiles` → `ActiveProfile`, with the legacy
`net.hadess.PowerProfiles` fallback). Avoid `powerprofilesctl get`: it is a
Python script that prints `?` when the shell carries a stray
`PYTHONHOME`/`PYTHONPATH` (e.g. leaked from an AppImage). Both `control.go` and
`battery-saver.sh`'s `status` use the busctl path.

## Build Commands

```bash
make build      # CGO_ENABLED=0 build -> bin/fbs
make run        # build + launch the TUI
make test       # go test ./...
make fmt        # gofmt -w internal cmd
make vet        # go vet ./...
make clean      # rm -rf bin
```

Run a single test:

```bash
go test ./internal/power -run TestParseBusctlString -v
```

Lint the shell script:

```bash
bash -n battery-saver.sh        # syntax
shellcheck battery-saver.sh     # if installed
```

## Coding Style

**Go**

- Idiomatic, minimal comments. Comment the non-obvious *why* (sysfs quirks,
  amd_pstate behaviour, unit conversions), not the obvious *what*.
- Group imports in three blocks: stdlib, third-party, then the
  `framework-battery-saver/...` project packages.
- Order struct methods public-first, then a `// Private` divider, then a
  `// Helpers` section for free functions at the bottom of the file. Match the
  existing files (`scraper.go`, `control.go`, `dashboard.go`).
- Tests use the standard library `testing` package, table-driven where it fits
  (see `profile_test.go`). No third-party assertion libraries.
- Stay on the stable Bubble Tea / Lip Gloss v1 APIs already in `go.mod`; do not
  pull in alpha/v2 lines.
- Units: sysfs reports kHz, µA, µV, µW, µWh, µAh — convert at the edge and keep
  the `Snapshot` fields in human units (MHz, W, %). Read metrics root-free.

**Shell (`battery-saver.sh`)**

- `set -euo pipefail`; use the `log`/`warn` helpers for output.
- Elevate only the parts that need it with `sudo` on demand; keep the script
  runnable as the normal user.
- Read-modify-write of `/proc/acpi/wakeup` happens inside a single root shell so
  it stays consistent.

## Agent behaviour

- **Never** add `Co-Authored-By` or other tool-attribution trailers to commit
  messages. Keep messages concise: a clear subject line and a short body
  explaining the why when it isn't obvious.
- Commit and push when asked; otherwise leave the tree for the maintainer.
- Build output (`bin/`, `dist/`) is gitignored — never commit it. Release
  tarballs bundle `fbs` and `battery-saver.sh` side by side so the binary's
  script lookup finds its neighbour.
