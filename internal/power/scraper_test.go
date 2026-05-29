package power

import (
	"context"
	"testing"
	"time"
)

func TestScrapeReadsLiveValues(t *testing.T) {
	s := NewScraper()
	s.Scrape(context.Background())
	snap := s.Snapshot()

	if snap.FreqMHz <= 0 {
		t.Errorf("expected a positive CPU frequency, got %.1f", snap.FreqMHz)
	}
	if snap.Governor == "" {
		t.Error("expected a governor name")
	}
	if snap.BatteryPct < 0 || snap.BatteryPct > 100 {
		t.Errorf("battery pct out of range: %d", snap.BatteryPct)
	}

	t.Logf("power=%.1fW (%s) freq=%.0fMHz cap=%.0f max=%.0f gov=%s batt=%d%% %s eta=%v(%v) onAC=%v profile=%q saver=%v",
		snap.PowerW, snap.PowerSource, snap.FreqMHz, snap.CapFreqMHz, snap.MaxFreqMHz,
		snap.Governor, snap.BatteryPct, snap.BatteryStatus, snap.BatteryETA.Round(time.Minute), snap.BatteryETAValid,
		snap.OnAC, snap.Profile, snap.SaverOn)
}

func TestRingChronologicalOrder(t *testing.T) {
	r := newRing(3)
	r.push(1)
	r.push(2)
	r.push(3)
	r.push(4) // evicts 1

	got := r.values()
	want := []float64{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("values[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
