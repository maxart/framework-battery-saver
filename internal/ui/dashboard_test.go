package ui

import (
	"strings"
	"testing"
	"time"

	"framework-battery-saver/internal/power"
)

func sampleSnapshot(saverOn bool) power.Snapshot {
	return power.Snapshot{
		PowerW:          13.2,
		PowerSource:     "battery",
		FreqMHz:         2012,
		CapFreqMHz:      2000,
		MaxFreqMHz:      5100,
		Governor:        "powersave",
		BatteryPct:      92,
		BatteryStatus:   "Discharging",
		BatteryETA:      3*time.Hour + 42*time.Minute,
		BatteryETAValid: true,
		Profile:         "power-saver",
		SaverOn:         saverOn,
		PowerHistory:    []float64{10, 11, 13, 12, 14, 13.2},
		FreqHistory:     []float64{1800, 1900, 2000, 2012},
	}
}

func TestDashboardRenders(t *testing.T) {
	for _, on := range []bool{true, false} {
		d := Dashboard{snap: sampleSnapshot(on), width: 72}
		view := d.View()
		if !strings.Contains(view, "Framework Battery Saver") {
			t.Error("missing title")
		}
		if !strings.Contains(view, "13.2 W") {
			t.Error("missing power value")
		}
		if !strings.Contains(view, "92%") {
			t.Error("missing battery value")
		}
		if !strings.Contains(view, "3h 42m left") {
			t.Error("missing battery time estimate")
		}
		t.Logf("\n%s", view)
	}
}

func TestToggleBoundsPositive(t *testing.T) {
	d := Dashboard{snap: sampleSnapshot(true), width: 72}
	top, height := d.ToggleBounds()
	if top <= 0 || height <= 0 {
		t.Errorf("expected positive bounds, got top=%d height=%d", top, height)
	}
}
