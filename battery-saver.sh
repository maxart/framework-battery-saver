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

# When invoked via sudo the script runs as root, so $HOME would be /root.
# Resolve the *invoking* user and write state into their home, where the
# unprivileged TUI looks for it.
readonly REAL_USER="${SUDO_USER:-$(id -un)}"
_real_home="$(getent passwd "$REAL_USER" | cut -d: -f6)"
readonly REAL_HOME="${_real_home:-$HOME}"

# Where we remember which wakeup devices we disabled, so "off" can re-enable
# exactly those (writing to /proc/acpi/wakeup toggles, so we must track state).
readonly STATE_DIR="$REAL_HOME/.local/state/battery-saver"
readonly WAKEUP_STATE="$STATE_DIR/disabled-wakeups"

log()  { printf '  %s\n' "$*"; }
warn() { printf '  ! %s\n' "$*" >&2; }

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

cmd_on() {
    log "Capping max CPU frequency to $FREQ_CAP"
    sudo cpupower frequency-set -u "$FREQ_CAP" >/dev/null

    log "Setting CPU governor to powersave"
    sudo cpupower frequency-set -g powersave >/dev/null

    log "Setting platform power profile to power-saver"
    powerprofilesctl set power-saver

    log "Disabling ACPI wakeup sources"
    mkdir -p "$STATE_DIR"
    # Snapshot currently-enabled wakeup devices, then disable each one.
    # Done in a single root shell so the read-modify-write is consistent.
    sudo bash -c '
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
    # under sudo) so the unprivileged TUI can read and remove it.
    sudo chown -R "$REAL_USER" "$STATE_DIR" 2>/dev/null || true

    local n=0
    [[ -f "$WAKEUP_STATE" ]] && n=$(wc -l < "$WAKEUP_STATE")
    log "Disabled $n wakeup source(s)"
    echo
    log "Power-saving mode active."
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
        sudo cpupower frequency-set -u "${maxfreq}" >/dev/null
    else
        warn "could not read hardware max frequency; leaving frequency cap as-is"
    fi

    # amd_pstate "active" mode uses the powersave governor by design; the EPP
    # hint (driven by the power profile below) is what actually scales perf.
    log "Setting CPU governor to powersave (amd_pstate default)"
    sudo cpupower frequency-set -g powersave >/dev/null

    log "Setting platform power profile to balanced"
    powerprofilesctl set balanced

    if [[ -f "$WAKEUP_STATE" ]]; then
        # Re-enable only when we actually recorded devices; the file can be
        # empty (no wakeups were enabled when we turned on).
        if [[ -s "$WAKEUP_STATE" ]]; then
            log "Re-enabling previously disabled wakeup sources"
            sudo bash -c '
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
    prof=$(powerprofilesctl get 2>/dev/null || echo "?")

    log "CPU governor    : $gov"
    log "Max freq (cap)  : $((maxf / 1000)) MHz"
    log "Current freq    : $((curf / 1000)) MHz (cpu0)"
    log "Power profile   : $prof"

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
}

main() {
    require_tools
    case "${1:-}" in
        on)     cmd_on ;;
        off)    cmd_off ;;
        status) cmd_status ;;
        *)
            cat >&2 <<EOF
Usage: $(basename "$0") {on|off|status}

  on      Cap CPU at $FREQ_CAP, powersave governor, power-saver profile,
          and disable ACPI wakeup sources.
  off     Restore balanced defaults and re-enable saved wakeup sources.
  status  Show current CPU governor, frequency cap, power profile, wakeups.
EOF
            exit 2
            ;;
    esac
}

main "$@"
