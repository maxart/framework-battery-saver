package power

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScriptPathFromEnv(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, scriptName)
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FBS_SCRIPT", script)

	got, err := ScriptPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != script {
		t.Errorf("ScriptPath() = %q, want %q", got, script)
	}
}

func TestStateFilePathRespectsXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	want := "/tmp/xdg-state/battery-saver/disabled-wakeups"
	if got := StateFilePath(); got != want {
		t.Errorf("StateFilePath() = %q, want %q", got, want)
	}
}
