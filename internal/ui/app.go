package ui

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"framework-battery-saver/internal/power"
)

const scrapeInterval = time.Second

type (
	tickMsg       struct{}
	snapshotMsg   struct{ snap power.Snapshot }
	toggleDoneMsg struct{ err error }
)

// App is the root Bubble Tea model. It drives the scrape ticker and the
// privileged toggle, delegating rendering to Dashboard.
type App struct {
	scraper *power.Scraper
	snap    power.Snapshot
	width   int
	height  int
	busy    bool
	err     error
}

func NewApp() *App {
	return &App{scraper: power.NewScraper()}
}

func (m *App) Init() tea.Cmd {
	return tea.Batch(m.scrape(), tick())
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case " ", "enter":
			return m, m.startToggle()
		case "r":
			return m, m.scrape()
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if m.clickedToggle(msg.Y) {
				return m, m.startToggle()
			}
		}

	case tickMsg:
		return m, tea.Batch(m.scrape(), tick())

	case snapshotMsg:
		m.snap = msg.snap

	case toggleDoneMsg:
		m.busy = false
		m.err = msg.err
		return m, m.scrape()
	}

	return m, nil
}

func (m *App) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	body := m.dashboard().View()
	if m.err != nil {
		body += "\n" + lipgloss.NewStyle().Foreground(colorError).Render("error: "+m.err.Error())
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}

// Run launches the program.
func Run() error {
	p := tea.NewProgram(NewApp(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// Private

func (m *App) dashboard() Dashboard {
	return Dashboard{snap: m.snap, busy: m.busy, width: m.width - 4} // padding(1,2)
}

func (m *App) scrape() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scrapeInterval)
		defer cancel()
		m.scraper.Scrape(ctx)
		return snapshotMsg{m.scraper.Snapshot()}
	}
}

func (m *App) startToggle() tea.Cmd {
	if m.busy {
		return nil
	}
	script, err := power.ScriptPath()
	if err != nil {
		m.err = fmt.Errorf("cannot find battery-saver.sh: %w", err)
		return nil
	}
	arg := "on"
	if m.snap.SaverOn {
		arg = "off"
	}
	m.busy = true
	m.err = nil

	cmd := exec.Command("sudo", script, arg)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return toggleDoneMsg{err: err}
	})
}

// clickedToggle reports whether row y (1-indexed terminal row) falls within the
// toggle's rendered band, accounting for the outer Padding(1,2).
func (m *App) clickedToggle(y int) bool {
	const topPad = 1
	top, height := m.dashboard().ToggleBounds()
	start := topPad + top
	return y >= start && y < start+height
}

// Helpers

func tick() tea.Cmd {
	return tea.Tick(scrapeInterval, func(time.Time) tea.Msg { return tickMsg{} })
}
