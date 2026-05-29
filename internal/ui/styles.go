package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Semantic palette. The accent greens/oranges/reds drive the health coloring;
// muted/text are adaptive so they stay legible on light and dark terminals.
var (
	colorPrimary = lipgloss.Color("39")  // cyan-blue
	colorGood    = lipgloss.Color("42")  // green
	colorWarn    = lipgloss.Color("214") // orange
	colorError   = lipgloss.Color("203") // red
	colorMuted   = lipgloss.AdaptiveColor{Light: "245", Dark: "240"}
	colorText    = lipgloss.AdaptiveColor{Light: "236", Dark: "252"}
)

var (
	titleStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	helpStyle  = lipgloss.NewStyle().Foreground(colorMuted)
)

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// sparkline maps the tail of data onto block runes, scaled to the window's own
// max so movement stays visible.
func sparkline(data []float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(data) > width {
		data = data[len(data)-width:]
	}
	maxV := 0.0
	for _, v := range data {
		if v > maxV {
			maxV = v
		}
	}
	var b strings.Builder
	for _, v := range data {
		idx := 0
		if maxV > 0 {
			idx = int(v / maxV * float64(len(sparkRunes)-1))
			if idx < 0 {
				idx = 0
			} else if idx > len(sparkRunes)-1 {
				idx = len(sparkRunes) - 1
			}
		}
		b.WriteRune(sparkRunes[idx])
	}
	// Left-pad with low blocks so the chart width is stable.
	if pad := width - len([]rune(b.String())); pad > 0 {
		return strings.Repeat(string(sparkRunes[0]), pad) + b.String()
	}
	return b.String()
}

// meter renders a [####----] style bar filled to pct (0..100).
func meter(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100 * float64(width))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
