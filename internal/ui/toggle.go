package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Toggle renders the large power-saver on/off button. busy shows a transient
// "applying…" state while the privileged script runs.
type Toggle struct {
	On    bool
	Busy  bool
	Width int
}

func (t Toggle) View() string {
	label := "  POWER SAVER: OFF  "
	var accent lipgloss.TerminalColor = colorMuted
	if t.On {
		label = "  POWER SAVER: ON  "
		accent = colorGood
	}
	if t.Busy {
		label = "  APPLYING…  "
		accent = colorWarn
	}

	inner := t.Width - 4
	if inner < lipgloss.Width(label) {
		inner = lipgloss.Width(label)
	}

	btn := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("231")).
		Background(accent).
		Padding(1, 0).
		Width(inner).
		Align(lipgloss.Center).
		Render(label)

	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(accent).
		Render(btn)

	hint := helpStyle.Render("click, or press <space> to toggle")
	return lipgloss.JoinVertical(lipgloss.Center, box, hint)
}

// Height is the number of terminal rows the toggle occupies, used for mouse
// hit-testing against its rendered position.
func (t Toggle) Height() int {
	return lipgloss.Height(t.View())
}
