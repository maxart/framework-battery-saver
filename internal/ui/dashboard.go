package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"framework-battery-saver/internal/power"
)

// minContentWidth keeps the two-column card layout legible on very narrow
// terminals; below this the cards would collapse.
const minContentWidth = 36

// Dashboard renders the live metrics and the toggle. It is a pure view over a
// Snapshot plus the transient "busy" flag owned by App.
type Dashboard struct {
	snap  power.Snapshot
	busy  bool
	width int
}

func (d Dashboard) View() string {
	return strings.Join([]string{
		d.header(),
		"",
		d.cards(),
		"",
		d.toggle().View(),
		"",
		d.footer(),
	}, "\n")
}

// ToggleBounds returns the 0-indexed row where the toggle begins and its
// height, so App can hit-test mouse clicks against it.
func (d Dashboard) ToggleBounds() (top, height int) {
	above := strings.Join([]string{d.header(), "", d.cards(), ""}, "\n")
	return lipgloss.Height(above), d.toggle().Height()
}

// Private

func (d Dashboard) header() string {
	source := "on battery"
	if d.snap.OnAC {
		source = "plugged in"
	}
	left := titleStyle.Render("⚡ Framework Battery Saver")
	right := mutedStyle.Render(source)
	gap := d.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (d Dashboard) cards() string {
	// Split the full width into two columns plus a 1-cell gap, filling exactly
	// (the columns may differ by one cell on odd widths).
	total := d.contentWidth()
	leftW := (total - 1) / 2
	rightW := total - 1 - leftW

	powerCard := MetricCard{
		Title:  "Power draw",
		Value:  fmt.Sprintf("%.1f W", d.snap.PowerW),
		Sub:    sparkline(d.snap.PowerHistory, leftW-4),
		Accent: powerAccent(d.snap.PowerW),
	}
	if d.snap.PowerSource == "n/a" {
		powerCard.Value = "n/a"
	}

	battery := MetricCard{
		Title:  "Battery · " + d.snap.BatteryStatus,
		Value:  batteryValue(d.snap),
		Sub:    meter(float64(d.snap.BatteryPct), rightW-4),
		Accent: batteryAccent(d.snap),
	}

	cpu := MetricCard{
		Title:  "CPU frequency",
		Value:  fmt.Sprintf("%.2f GHz", d.snap.FreqMHz/1000),
		Sub:    fmt.Sprintf("%s · cap %.1fGHz", emptyDash(d.snap.Governor), d.snap.CapFreqMHz/1000),
		Accent: colorPrimary,
	}

	profile := MetricCard{
		Title:  "Power profile",
		Value:  emptyDash(d.snap.Profile),
		Sub:    saverLabel(d.snap.SaverOn),
		Accent: profileAccent(d.snap),
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top, powerCard.View(leftW), " ", battery.View(rightW))
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, cpu.View(leftW), " ", profile.View(rightW))
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (d Dashboard) toggle() Toggle {
	return Toggle{On: d.snap.SaverOn, Busy: d.busy, Width: d.contentWidth()}
}

func (d Dashboard) footer() string {
	return helpStyle.Render(truncate("space toggle · e extras · r refresh · q quit", d.contentWidth()))
}

func (d Dashboard) contentWidth() int {
	if d.width < minContentWidth {
		return minContentWidth
	}
	return d.width
}

// Helpers

func powerAccent(w float64) lipgloss.Color {
	switch {
	case w >= 25:
		return colorError
	case w >= 12:
		return colorWarn
	default:
		return colorGood
	}
}

func batteryAccent(s power.Snapshot) lipgloss.Color {
	if s.OnAC || s.BatteryStatus == "Charging" {
		return colorGood
	}
	switch {
	case s.BatteryPct <= 15:
		return colorError
	case s.BatteryPct <= 35:
		return colorWarn
	default:
		return colorGood
	}
}

func profileAccent(s power.Snapshot) lipgloss.Color {
	if s.Profile == "power-saver" {
		return colorGood
	}
	return colorPrimary
}

func batteryValue(s power.Snapshot) string {
	v := fmt.Sprintf("%d%%", s.BatteryPct)
	if s.BatteryETAValid {
		suffix := "left"
		if s.BatteryStatus == "Charging" {
			suffix = "to full"
		}
		v += " · " + fmtDuration(s.BatteryETA) + " " + suffix
	}
	return v
}

func fmtDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func saverLabel(on bool) string {
	if on {
		return "saver active"
	}
	return "saver off"
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
