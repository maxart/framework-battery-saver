package power

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const scriptName = "battery-saver.sh"

// ScriptPath locates the privileged battery-saver.sh helper. It checks
// $FBS_SCRIPT, then alongside and one level above the running binary (bin/fbs
// -> repo root), then the working directory.
func ScriptPath() (string, error) {
	if env := os.Getenv("FBS_SCRIPT"); env != "" {
		if exists(env) {
			return env, nil
		}
	}

	var candidates []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, scriptName),
			filepath.Join(dir, "..", scriptName),
		)
	}
	candidates = append(candidates, scriptName)

	for _, c := range candidates {
		if exists(c) {
			return filepath.Abs(c)
		}
	}
	return "", os.ErrNotExist
}

// StateFilePath is where battery-saver.sh records the wakeup devices it
// disabled; its existence marks saver mode as active.
func StateFilePath() string {
	if paths := stateFileCandidates(); len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// SaverActive reports whether battery-saver.sh has saver mode applied. It
// checks every candidate location since the script (run via sudo) may resolve
// the home directory differently than the TUI.
func SaverActive() bool {
	for _, p := range stateFileCandidates() {
		if exists(p) {
			return true
		}
	}
	return false
}

// Helpers

func stateFileCandidates() []string {
	const rel = "battery-saver/disabled-wakeups"
	var dirs []string
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		dirs = append(dirs, xdg)
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "state"))
	}

	seen := make(map[string]bool)
	var paths []string
	for _, d := range dirs {
		p := filepath.Join(d, rel)
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// readProfile queries power-profiles-daemon's ActiveProfile over the system
// D-Bus via busctl. We avoid `powerprofilesctl get` because it is a Python
// script that breaks when the environment carries a stray PYTHONHOME/PYTHONPATH
// (e.g. leaked from an AppImage), whereas busctl ships with systemd and has no
// such dependency.
func readProfile(ctx context.Context) string {
	for _, d := range ppdEndpoints {
		out, err := exec.CommandContext(ctx, "busctl", "--system", "get-property",
			d.name, d.path, d.iface, "ActiveProfile").Output()
		if err != nil {
			continue
		}
		if p := parseBusctlString(out); p != "" {
			return p
		}
	}
	return "unknown"
}

var ppdEndpoints = []struct{ name, path, iface string }{
	{"org.freedesktop.UPower.PowerProfiles", "/org/freedesktop/UPower/PowerProfiles", "org.freedesktop.UPower.PowerProfiles"},
	{"net.hadess.PowerProfiles", "/net/hadess/PowerProfiles", "net.hadess.PowerProfiles"},
}

// parseBusctlString extracts the value from busctl's `s "value"` output.
func parseBusctlString(out []byte) string {
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "s ")
	return strings.Trim(strings.TrimSpace(s), `"`)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
