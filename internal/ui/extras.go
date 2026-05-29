package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"framework-battery-saver/internal/power"
)

// Extras is the secondary panel for the wireless radios and screen brightness.
// Like Dashboard it is a pure view over a Snapshot plus the transient busy flag.
type Extras struct {
	snap  power.Snapshot
	busy  bool
	width int
}

func (e Extras) View() string {
	width := e.width
	if width < minContentWidth {
		width = minContentWidth
	}
	inner := width - 4 // border (2) + horizontal padding (2)

	rows := strings.Join([]string{
		e.radioRow("Wi-Fi", "[w]", e.snap.Wifi),
		e.radioRow("Bluetooth", "[b]", e.snap.Bluetooth),
		e.brightnessRow(inner),
		"",
		e.bootRow(inner),
	}, "\n")

	body := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Extras"),
		mutedStyle.Render("radios, brightness & reboot persistence"),
		"",
		rows,
	)

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Width(width - 2).
		Render(body)

	hint := helpStyle.Render(truncate("w/b toggle · ←/→ brightness · e/esc back · q quit", width))
	if e.busy {
		hint = lipgloss.NewStyle().Foreground(colorWarn).
			Render(truncate("applying… (enter password if prompted)", width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, panel, "", hint)
}

// Private

func (e Extras) radioRow(label, key string, st power.RadioState) string {
	text, color := radioBadge(st)
	left := lipgloss.NewStyle().Foreground(colorText).Width(14).Render(label)
	mid := lipgloss.NewStyle().Foreground(color).Bold(true).Width(10).Render(text)
	return left + mid + helpStyle.Render(key+" toggle")
}

// bootRow shows whether the systemd boot unit is set up, so the user can
// confirm the cap and wakeup disables will survive a reboot. It is informational
// only — the unit is installed automatically when the saver is turned on.
func (e Extras) bootRow(inner int) string {
	text, color := bootBadge(e.snap.BootService)
	left := lipgloss.NewStyle().Foreground(colorText).Width(14).Render("Persistence")
	mid := lipgloss.NewStyle().Foreground(color).Bold(true).Render(text)
	row := left + mid
	// Append the explanatory hint only when it fits; on narrow terminals the
	// badge alone still answers "is it set up?".
	hint := bootHint(e.snap.BootService)
	if lipgloss.Width(row)+1+lipgloss.Width(hint) <= inner {
		row += " " + helpStyle.Render(hint)
	}
	return row
}

func (e Extras) brightnessRow(inner int) string {
	left := lipgloss.NewStyle().Foreground(colorText).Width(14).Render("Brightness")
	if !e.snap.BrightnessPresent {
		return left + mutedStyle.Render("n/a")
	}
	hint := helpStyle.Render("←/→ adjust")
	pct := lipgloss.NewStyle().Foreground(colorText).Render(fmt.Sprintf(" %3d%% ", e.snap.BrightnessPct))
	barW := inner - 14 - lipgloss.Width(pct) - lipgloss.Width(hint)
	if barW < 6 {
		barW = 6
	}
	bar := lipgloss.NewStyle().Foreground(colorPrimary).Render(meter(float64(e.snap.BrightnessPct), barW))
	return left + bar + pct + hint
}

// Helpers

// bootBadge maps the boot-unit state to a label and accent. Only "enabled"
// means settings will be restored at boot; anything else is a warning.
func bootBadge(state string) (string, lipgloss.TerminalColor) {
	switch state {
	case "enabled":
		return "● enabled", colorGood
	case "", "not installed":
		return "✗ not installed", colorWarn
	default: // disabled, static, linked, …
		return "○ " + state, colorWarn
	}
}

// bootHint explains what the boot-unit state means for persistence.
func bootHint(state string) string {
	if state == "enabled" {
		return "saver settings restored at boot"
	}
	return "auto-enabled when saver is turned on"
}

// radioBadge maps a radio's state to a label and its accent color.
func radioBadge(st power.RadioState) (string, lipgloss.TerminalColor) {
	switch {
	case !st.Present:
		return "n/a", colorMuted
	case st.HardBlock:
		return "OFF (hw)", colorWarn
	case st.Off:
		return "○ OFF", colorMuted
	default:
		return "● ON", colorGood
	}
}
