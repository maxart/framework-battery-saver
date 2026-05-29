package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"framework-battery-saver/internal/power"
)

func extrasSnapshot() power.Snapshot {
	s := sampleSnapshot(true)
	s.BrightnessPresent = true
	s.BrightnessPct = 27
	s.Wifi = power.RadioState{Present: true, Off: false}
	s.Bluetooth = power.RadioState{Present: true, Off: true}
	s.BootService = "enabled"
	return s
}

func TestExtrasRenders(t *testing.T) {
	e := Extras{snap: extrasSnapshot(), width: 72}
	view := e.View()
	for _, want := range []string{"Extras", "Wi-Fi", "Bluetooth", "Brightness", "27%", "ON", "OFF", "Persistence", "enabled"} {
		if !strings.Contains(view, want) {
			t.Errorf("extras view missing %q", want)
		}
	}
	t.Logf("\n%s", view)
}

func TestExtrasNoOverflow(t *testing.T) {
	for _, w := range []int{40, 60, 100, 140} {
		e := Extras{snap: extrasSnapshot(), width: w}
		maxW := 0
		for _, ln := range strings.Split(e.View(), "\n") {
			if x := lipgloss.Width(ln); x > maxW {
				maxW = x
			}
		}
		if maxW > w {
			t.Errorf("width=%d produced a line of width %d (overflow)", w, maxW)
		}
	}
}

func TestRadioBadge(t *testing.T) {
	cases := []struct {
		st   power.RadioState
		want string
	}{
		{power.RadioState{Present: false}, "n/a"},
		{power.RadioState{Present: true, Off: false}, "ON"},
		{power.RadioState{Present: true, Off: true}, "OFF"},
		{power.RadioState{Present: true, Off: true, HardBlock: true}, "OFF (hw)"},
	}
	for _, c := range cases {
		got, _ := radioBadge(c.st)
		if !strings.Contains(got, c.want) {
			t.Errorf("radioBadge(%+v) = %q, want substring %q", c.st, got, c.want)
		}
	}
}

func TestBootBadge(t *testing.T) {
	cases := []struct{ state, want string }{
		{"enabled", "enabled"},
		{"disabled", "disabled"},
		{"not installed", "not installed"},
		{"", "not installed"},
		{"static", "static"},
	}
	for _, c := range cases {
		got, _ := bootBadge(c.state)
		if !strings.Contains(got, c.want) {
			t.Errorf("bootBadge(%q) = %q, want substring %q", c.state, got, c.want)
		}
	}
}
