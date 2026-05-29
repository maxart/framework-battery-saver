package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// MetricCard is a small bordered panel showing a labelled value with an
// optional sub-line (sparkline, meter, or detail text).
type MetricCard struct {
	Title  string
	Value  string
	Sub    string
	Accent lipgloss.Color
}

func (c MetricCard) View(width int) string {
	// Total width = text + padding (2) + border (2). lipgloss .Width() counts
	// padding, so it gets text+padding; the border is added on top.
	textW := width - 4
	if textW < 1 {
		textW = 1
	}

	title := mutedStyle.Render(truncate(c.Title, textW))
	value := lipgloss.NewStyle().Foreground(c.Accent).Bold(true).Render(truncate(c.Value, textW))
	sub := lipgloss.NewStyle().Foreground(colorText).Render(truncate(c.Sub, textW))

	body := lipgloss.JoinVertical(lipgloss.Left, title, value, sub)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Padding(0, 1).
		Width(width - 2).
		Render(body)
}

// Helpers

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}
