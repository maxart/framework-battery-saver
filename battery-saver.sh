#!/usr/bin/env bash
#
# battery-saver.sh — power management toggle for the Framework Laptop 13
#                    (AMD Ryzen AI 9 HX 370) on Arch / Omarchy.
#
# Usage:
#   ./battery-saver.sh on       Apply power-saving settings
#   ./battery-saver.sh off      Restore balanced defaults
#   ./battery-saver.sh status   Show current power state
#
# Root is only needed for cpupower and /proc/acpi/wakeup; the script
# elevates those parts with sudo on demand, so run it as your normal user
# (powerprofilesctl talks to power-profiles-daemon over polkit).

set -euo pipefail

# Max frequency cap applied in "on" mode. The HX 370 boosts well past this;
# capping it is where most of the battery win comes from.
readonly FREQ_CAP="2.0GHz"

# The systemd unit that re-applies saver settings at boot (the freq cap and
# wakeup disables do not survive a reboot, but the saver marker does). Installed
# automatically by `on`, removed by `uninstall-service`.
readonly SERVICE_NAME="fbs-restore.service"
readonly SERVICE_PATH="/etc/systemd/system/$SERVICE_NAME"

# State paths are resolved per-invocation by init_state_paths (below) rather than
# fixed at load time, so `restore` — which the boot unit runs as root, with no
# SUDO_USER — can target the right user's home.
REAL_USER="" REAL_HOME="" STATE_DIR="" WAKEUP_STATE=""

log()  { printf '  %s\n' "$*"; }
warn() { printf '  ! %s\n' "$*" >&2; }

# Resolve the user whose home holds the saver state, and the derived paths. When
# invoked via sudo the script runs as root ($HOME=/root), so we resolve the
# *invoking* user and write state into their home, where the unprivileged TUI
# looks for it. An explicit $1 overrides (the boot unit passes the target user).
init_state_paths() {
    REAL_USER="${1:-${SUDO_USER:-$(id -un)}}"
    local home
    home="$(getent passwd "$REAL_USER" | cut -d: -f6)"
    REAL_HOME="${home:-$HOME}"
    # Where we remember which wakeup devices we disabled, so "off" can re-enable
    # exactly those (writing to /proc/acpi/wakeup toggles, so we track state).
    STATE_DIR="$REAL_HOME/.local/state/battery-saver"
    WAKEUP_STATE="$STATE_DIR/disabled-wakeups"
}

# Run a command as root: directly when already root (the boot unit), otherwise
# via sudo (the interactive on/off/radio paths). Keeps `restore` prompt-free.
run_priv() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    else
        sudo "$@"
    fi
}

# Read the active power-profiles-daemon profile over the system D-Bus. We use
# busctl (ships with systemd) rather than `powerprofilesctl get` because the
# latter is a Python script that breaks when the shell carries a stray
# PYTHONHOME/PYTHONPATH (e.g. leaked from an AppImage), printing "?" instead.
read_profile() {
    local out
    out=$(busctl --system get-property \
        org.freedesktop.UPower.PowerProfiles \
        /org/freedesktop/UPower/PowerProfiles \
        org.freedesktop.UPower.PowerProfiles ActiveProfile 2>/dev/null) ||
    out=$(busctl --system get-property \
        net.hadess.PowerProfiles \
        /net/hadess/PowerProfiles \
        net.hadess.PowerProfiles ActiveProfile 2>/dev/null) || true
    # busctl prints: s "balanced"
    out=${out#s }
    out=${out//\"/}
    out=$(printf '%s' "$out" | tr -d '[:space:]')
    if [[ -n "$out" ]]; then
        printf '%s' "$out"
    else
        powerprofilesctl get 2>/dev/null || printf '?'
    fi
}

require_tools() {
    local missing=()
    for t in cpupower powerprofilesctl; do
        command -v "$t" >/dev/null 2>&1 || missing+=("$t")
    done
    if ((${#missing[@]})); then
        warn "missing required tools: ${missing[*]}"
        warn "install with: sudo pacman -S cpupower power-profiles-daemon"
        exit 1
    fi
}

# apply_on applies every power-saving setting and records the wakeup state. It
# is shared by `on` (interactive) and `restore` (boot), so it must be idempotent
# and must not prompt — all elevation goes through run_priv.
apply_on() {
    log "Capping max CPU frequency to $FREQ_CAP"
    run_priv cpupower frequency-set -u "$FREQ_CAP" >/dev/null

    log "Setting CPU governor to powersave"
    run_priv cpupower frequency-set -g powersave >/dev/null

    log "Setting platform power profile to power-saver"
    # power-profiles-daemon persists this across reboots itself; tolerate failure
    # so a hiccup here never aborts the cap/wakeup steps that don't persist.
    powerprofilesctl set power-saver || warn "could not set power-saver profile"

    log "Disabling ACPI wakeup sources"
    mkdir -p "$STATE_DIR"
    # Snapshot currently-enabled wakeup devices, then disable each one. Done in a
    # single root shell so the read-modify-write is consistent. /proc/acpi/wakeup
    # resets to firmware defaults on every boot, so re-running this at boot
    # reproduces the disabled set.
    run_priv bash -c '
        state_file="$1"
        : > "$state_file"
        while read -r name _sstate status _rest; do
            [ "$status" = "*enabled" ] || continue
            if echo "$name" > /proc/acpi/wakeup 2>/dev/null; then
                echo "$name" >> "$state_file"
            fi
        done < <(grep -E "^[A-Z0-9]" /proc/acpi/wakeup)
    ' _ "$WAKEUP_STATE"
    # Hand the state dir back to the invoking user (we may be running as root
    # under sudo or from the boot unit) so the unprivileged TUI can read/remove it.
    run_priv chown -R "$REAL_USER" "$STATE_DIR" 2>/dev/null || true

    local n=0
    [[ -f "$WAKEUP_STATE" ]] && n=$(wc -l < "$WAKEUP_STATE")
    log "Disabled $n wakeup source(s)"
}

cmd_on() {
    apply_on
    # The cap and wakeup disables don't survive a reboot, so install a boot unit
    # that re-applies them while the saver marker is present.
    ensure_boot_service
    echo
    log "Power-saving mode active."
}

# ensure_boot_service installs and enables the systemd unit that re-applies
# saver settings at boot. Idempotent and quiet when already current. The unit is
# guarded by ConditionPathExists on the marker, so it is a no-op while saver is
# off — `off` therefore needn't touch it.
ensure_boot_service() {
    command -v systemctl >/dev/null 2>&1 || return 0
    local self desired
    self="$(readlink -f "$0")"
    desired="[Unit]
Description=Restore Framework Battery Saver settings at boot
After=power-profiles-daemon.service
Wants=power-profiles-daemon.service
ConditionPathExists=$WAKEUP_STATE

[Service]
Type=oneshot
ExecStart=$self restore $REAL_USER

[Install]
WantedBy=multi-user.target"
    if [[ "$(cat "$SERVICE_PATH" 2>/dev/null)" != "$desired" ]]; then
        log "Enabling boot-persistence service ($SERVICE_NAME)"
        printf '%s\n' "$desired" | run_priv tee "$SERVICE_PATH" >/dev/null
        run_priv systemctl daemon-reload
    fi
    systemctl is-enabled "$SERVICE_NAME" >/dev/null 2>&1 ||
        run_priv systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 ||
        warn "could not enable $SERVICE_NAME"
}

# cmd_restore re-applies saver settings at boot when the marker is present. The
# boot unit runs it as root and passes the target user (root has no SUDO_USER).
cmd_restore() {
    init_state_paths "${1:-}"
    if [[ ! -f "$WAKEUP_STATE" ]]; then
        log "No saver marker for $REAL_USER; nothing to restore."
        return 0
    fi
    log "Restoring power-saving settings for $REAL_USER"
    apply_on
    echo
    log "Power-saving settings restored."
}

cmd_uninstall_service() {
    command -v systemctl >/dev/null 2>&1 || { warn "systemctl not found"; exit 1; }
    if [[ -f "$SERVICE_PATH" ]]; then
        log "Removing boot-persistence service ($SERVICE_NAME)"
        run_priv systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
        run_priv rm -f "$SERVICE_PATH"
        run_priv systemctl daemon-reload
    else
        log "$SERVICE_NAME is not installed"
    fi
}

cmd_off() {
    local cpufreq="/sys/devices/system/cpu/cpu0/cpufreq"
    local maxfreq
    # Uncap to the CPU's true hardware maximum. On amd_pstate, cpuinfo_max_freq
    # tracks the *current* scaling cap (so it reads 2.0GHz while capped, which
    # would make this a no-op); amd_pstate_max_freq is the stable ceiling.
    maxfreq=$(cat "$cpufreq/amd_pstate_max_freq" 2>/dev/null || true)
    [[ -z "$maxfreq" ]] && maxfreq=$(cat "$cpufreq/cpuinfo_max_freq" 2>/dev/null || true)

    if [[ -n "$maxfreq" ]]; then
        log "Removing frequency cap (up to $((maxfreq / 1000)) MHz)"
        run_priv cpupower frequency-set -u "${maxfreq}" >/dev/null
    else
        warn "could not read hardware max frequency; leaving frequency cap as-is"
    fi

    # amd_pstate "active" mode uses the powersave governor by design; the EPP
    # hint (driven by the power profile below) is what actually scales perf.
    log "Setting CPU governor to powersave (amd_pstate default)"
    run_priv cpupower frequency-set -g powersave >/dev/null

    log "Setting platform power profile to balanced"
    powerprofilesctl set balanced

    if [[ -f "$WAKEUP_STATE" ]]; then
        # Re-enable only when we actually recorded devices; the file can be
        # empty (no wakeups were enabled when we turned on).
        if [[ -s "$WAKEUP_STATE" ]]; then
            log "Re-enabling previously disabled wakeup sources"
            run_priv bash -c '
                state_file="$1"
                while read -r name; do
                    [ -n "$name" ] || continue
                    # Only toggle back on if it is currently disabled.
                    if grep -qE "^${name}\b.*\bdisabled" /proc/acpi/wakeup; then
                        echo "$name" > /proc/acpi/wakeup 2>/dev/null || true
                    fi
                done < "$state_file"
            ' _ "$WAKEUP_STATE"
        fi
        # Always clear the marker so saver state reads as off.
        rm -f "$WAKEUP_STATE"
    else
        log "No saved wakeup state to restore"
    fi
    echo
    log "Balanced defaults restored."
}

cmd_status() {
    local gov maxf curf prof
    gov=$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor 2>/dev/null || echo "?")
    maxf=$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_max_freq 2>/dev/null || echo 0)
    curf=$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq 2>/dev/null || echo 0)
    prof=$(read_profile)

    log "CPU governor    : $gov"
    log "Max freq (cap)  : $((maxf / 1000)) MHz"
    log "Current freq    : $((curf / 1000)) MHz (cpu0)"
    log "Power profile   : $prof"
    log "Wi-Fi           : $(radio_state wlan)"
    log "Bluetooth       : $(radio_state bluetooth)"
    log "Brightness      : $(brightness_str)"

    local enabled
    # grep -c already prints 0 when there are no matches (and exits 1); the
    # `|| true` just keeps `set -e` happy without adding a second "0".
    enabled=$(grep -cE "\*enabled" /proc/acpi/wakeup 2>/dev/null || true)
    log "Wakeup sources  : ${enabled:-0} enabled"

    if [[ -f "$WAKEUP_STATE" ]]; then
        log "Saver state     : ON ($(wc -l < "$WAKEUP_STATE") wakeups disabled by saver)"
    else
        log "Saver state     : OFF (no saved wakeup state)"
    fi

    if command -v systemctl >/dev/null 2>&1 && [[ -f "$SERVICE_PATH" ]]; then
        log "Boot service    : $(systemctl is-enabled "$SERVICE_NAME" 2>/dev/null || echo installed)"
    else
        log "Boot service    : not installed"
    fi
}

# Read a radio's soft-block state from sysfs (root-free). $1 is the rfkill
# "type" as it appears in sysfs: "wlan" for Wi-Fi, "bluetooth" for Bluetooth.
radio_state() {
    local want="$1" r typ soft hard
    for r in /sys/class/rfkill/rfkill*; do
        [[ -e "$r/type" ]] || continue
        typ=$(cat "$r/type" 2>/dev/null)
        [[ "$typ" == "$want" ]] || continue
        soft=$(cat "$r/soft" 2>/dev/null)
        hard=$(cat "$r/hard" 2>/dev/null)
        [[ "$hard" == "1" ]] && { echo "off (hw block)"; return; }
        [[ "$soft" == "0" ]] && echo "on" || echo "off"
        return
    done
    echo "n/a"
}

# Current backlight level as a percentage, or "n/a" if no backlight (root-free).
brightness_str() {
    local d cur max
    for d in /sys/class/backlight/*; do
        [[ -e "$d/brightness" ]] || continue
        cur=$(cat "$d/brightness" 2>/dev/null) || continue
        max=$(cat "$d/max_brightness" 2>/dev/null) || continue
        [[ "$max" -gt 0 ]] 2>/dev/null || continue
        echo "$(( (cur * 100 + max / 2) / max ))%"
        return
    done
    echo "n/a"
}

# Enable/disable a radio via rfkill (needs root). $1 is the rfkill identifier
# ("wifi" or "bluetooth"); "on" unblocks, "off" soft-blocks.
cmd_radio() {
    local kind="$1" action="${2:-}"
    command -v rfkill >/dev/null 2>&1 || { warn "rfkill not found (install util-linux)"; exit 1; }
    case "$action" in
        on)  log "Enabling $kind";  run_priv rfkill unblock "$kind" ;;
        off) log "Disabling $kind"; run_priv rfkill block "$kind" ;;
        *)   warn "usage: $(basename "$0") $kind {on|off}"; exit 2 ;;
    esac
}

main() {
    require_tools
    init_state_paths
    case "${1:-}" in
        on)                cmd_on ;;
        off)               cmd_off ;;
        status)            cmd_status ;;
        restore)           cmd_restore "${2:-}" ;;
        install-service)   ensure_boot_service ;;
        uninstall-service) cmd_uninstall_service ;;
        wifi)              cmd_radio wifi "${2:-}" ;;
        bluetooth)         cmd_radio bluetooth "${2:-}" ;;
        *)
            cat >&2 <<EOF
Usage: $(basename "$0") {on|off|status|wifi|bluetooth|install-service|uninstall-service}

  on              Cap CPU at $FREQ_CAP, powersave governor, power-saver profile,
                  and disable ACPI wakeup sources. Also installs a systemd unit
                  that re-applies these at boot (they don't survive a reboot).
  off             Restore balanced defaults and re-enable saved wakeup sources.
  status          Show CPU, profile, wakeups, radios, brightness, boot service.
  wifi {on|off}   Enable or disable Wi-Fi (rfkill).
  bluetooth {on|off}
                  Enable or disable Bluetooth (rfkill).
  install-service     Install/enable the boot-persistence unit ($SERVICE_NAME).
  uninstall-service   Disable and remove the boot-persistence unit.

  restore [USER]  Re-apply saver settings if the marker is present. Run by the
                  boot unit as root; not normally invoked by hand.

Brightness is adjusted from the TUI via brightnessctl (no root needed).
EOF
            exit 2
            ;;
    esac
}

main "$@"
