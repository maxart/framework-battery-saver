# framework-battery-saver

Power management for the Framework Laptop 13 (AMD Ryzen AI 9 HX 370) on
Arch / Omarchy. Two pieces:

- **`battery-saver.sh`** — a CLI toggle that caps the CPU, sets the governor and
  power profile, and disables ACPI wakeup sources (all reversibly).
- **`fbs`** — a terminal dashboard showing live power draw, CPU frequency,
  battery (with time estimate), and power profile, plus a big on/off toggle.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/maxart/framework-battery-saver/main/install.sh | sh
```

This downloads the right binary for your platform, installs `fbs` and its
`battery-saver.sh` helper to `~/.local/bin`, and launches the dashboard.
Override the location with `FBS_INSTALL_DIR`, or skip launch with
`FBS_NO_LAUNCH=1`.

## TUI (`fbs`)

```
┌ Framework Battery Saver ──────────────────────────────────┐
│ Power draw            │ Battery · Discharging              │
│ 13.2 W                │ 92% · 3h 42m left                  │
│ ▁▁▁▁▁▁▁▁▁▁▁▁▆▆▇▇█▇    │ ████████████████████████████░░░    │
├───────────────────────┼────────────────────────────────────┤
│ CPU frequency         │ Power profile                      │
│ 2.01 GHz              │ power-saver                        │
│ powersave · cap 2.0GHz│ saver active                       │
└───────────────────────┴────────────────────────────────────┘
        ┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
        ┃         POWER SAVER: ON          ┃
        ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
          click, or press <space> to toggle
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) +
[Lip Gloss](https://github.com/charmbracelet/lipgloss). Resizes to fill the
terminal width.

```bash
make run        # build + launch (-> bin/fbs)
make test       # run unit tests
```

Run as your normal user — all metrics are read root-free. Toggling shells out to
`sudo ./battery-saver.sh on|off`, so the TUI briefly drops to a sudo prompt and
then resumes.

| Key             | Action               |
|-----------------|----------------------|
| `space`/`enter` | toggle power saver   |
| left click      | toggle (the button)  |
| `r`             | refresh now          |
| `q`/`esc`/`^C`  | quit                 |

### What it reads

| Metric        | Source                                                              |
|---------------|---------------------------------------------------------------------|
| Power draw    | battery `current_now × voltage_now` when discharging; RAPL `energy_uj` otherwise |
| CPU frequency | `cpufreq/scaling_cur_freq` (avg of online cores), `scaling_max_freq` (cap) |
| Governor      | `cpufreq/scaling_governor`                                          |
| Battery + ETA | `/sys/class/power_supply/BAT*` (energy/power or charge/current)     |
| Power profile | power-profiles-daemon `ActiveProfile` over D-Bus (`busctl`)         |
| Saver state   | presence of `~/.local/state/battery-saver/disabled-wakeups`         |

> RAPL `energy_uj` is root-only on most kernels, so on AC the power figure may
> show `n/a`; on battery it always comes from the battery gauge.

`fbs` finds the script via `$FBS_SCRIPT`, then next to the binary, then one
directory up (`bin/fbs` → repo root), then the working directory.

## CLI (`battery-saver.sh`)

```bash
./battery-saver.sh on       # cap 2.0GHz, powersave governor, power-saver profile, disable wakeups
./battery-saver.sh off      # restore: uncap to hardware max, balanced profile, re-enable wakeups
./battery-saver.sh status   # show current CPU/profile/wakeup state
```

Needs `cpupower` and `power-profiles-daemon`:

```bash
sudo pacman -S cpupower power-profiles-daemon
```

### amd_pstate notes (HX 370)

The CPU runs the `amd_pstate` driver in **active** mode, which has two quirks the
tooling accounts for:

- The governor is always `powersave` by design — perf is steered by the EPP hint
  (driven by the power profile), not by switching to `performance`. Capping the
  max frequency is where most of the battery win comes from.
- `cpuinfo_max_freq` is **dynamic**: it tracks the current scaling cap, so it
  reads 2.0 GHz while capped. `off` therefore uncaps using `amd_pstate_max_freq`
  (the stable ~5.16 GHz ceiling), and saver state is tracked via the state file
  rather than inferred from the cap.

Wakeup changes are reversible: `on` records exactly which `/proc/acpi/wakeup`
devices it disabled, and `off` re-enables only those.

## License

[MIT](LICENSE) © MAXART
