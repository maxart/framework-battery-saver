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

const (
	scrapeInterval = time.Second
	brightnessStep = 10 // percent per key press
	brightnessMin  = 5  // never let the screen go fully dark
)

type viewMode int

const (
	modeDashboard viewMode = iota
	modeExtras
)

type (
	tickMsg       struct{}
	snapshotMsg   struct{ snap power.Snapshot }
	toggleDoneMsg struct{ err error }
)

// App is the root Bubble Tea model. It drives the scrape ticker and the
// privileged toggles, delegating rendering to Dashboard or Extras.
type App struct {
	scraper *power.Scraper
	snap    power.Snapshot
	mode    viewMode
	width   int
	height  int
	busy    bool
	err     error

	// reconciled guards the one-shot launch drift-check (see Update) so we
	// re-apply at most once per session and never loop on a cancelled sudo.
	reconciled bool
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
		return m.handleKey(msg)

	case tea.MouseMsg:
		if m.mode == modeDashboard &&
			msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if m.clickedToggle(msg.Y) {
				return m, m.startToggle()
			}
		}

	case tickMsg:
		return m, tea.Batch(m.scrape(), tick())

	case snapshotMsg:
		m.snap = msg.snap
		// Belt-and-suspenders for the boot service: if the marker says saver is
		// on but the cap isn't actually applied (e.g. the service didn't run),
		// re-apply once. The boot unit normally makes this a no-op.
		if !m.reconciled && !m.busy && m.snap.CapDrifted() {
			m.reconciled = true
			return m, m.startReapply()
		}

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
	var body string
	if m.mode == modeExtras {
		body = m.extras().View()
	} else {
		body = m.dashboard().View()
	}
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

func (m *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.mode == modeExtras {
		switch key {
		case "esc", "e":
			m.mode = modeDashboard
		case "q":
			return m, tea.Quit
		case "w":
			return m, m.startRadioToggle("wifi")
		case "b":
			return m, m.startRadioToggle("bluetooth")
		case "left", "h", "-", "_":
			return m, m.setBrightness(-brightnessStep)
		case "right", "l", "+", "=":
			return m, m.setBrightness(brightnessStep)
		}
		return m, nil
	}

	switch key {
	case "q", "esc":
		return m, tea.Quit
	case " ", "enter":
		return m, m.startToggle()
	case "r":
		return m, m.scrape()
	case "e":
		m.mode = modeExtras
	}
	return m, nil
}

func (m *App) dashboard() Dashboard {
	return Dashboard{snap: m.snap, busy: m.busy, width: m.width - 4} // padding(1,2)
}

func (m *App) extras() Extras {
	return Extras{snap: m.snap, busy: m.busy, width: m.width - 4}
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

// startReapply re-runs `on` to restore saver settings that didn't survive a
// reboot (the freq cap, the wakeup disables). Unlike startToggle it always runs
// `on` rather than flipping on the current marker state. It also re-ensures the
// boot service, so an outdated install heals itself on next launch.
func (m *App) startReapply() tea.Cmd {
	if m.busy {
		return nil
	}
	script, err := power.ScriptPath()
	if err != nil {
		m.err = fmt.Errorf("cannot find battery-saver.sh: %w", err)
		return nil
	}
	m.busy = true
	m.err = nil

	cmd := exec.Command("sudo", script, "on")
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return toggleDoneMsg{err: err}
	})
}

// startRadioToggle flips Wi-Fi or Bluetooth via the privileged script (rfkill
// needs root), so it goes through the same suspend-and-sudo path as the saver
// toggle. kind is "wifi" or "bluetooth".
func (m *App) startRadioToggle(kind string) tea.Cmd {
	if m.busy {
		return nil
	}
	st := m.snap.Wifi
	if kind == "bluetooth" {
		st = m.snap.Bluetooth
	}
	switch {
	case !st.Present:
		m.err = fmt.Errorf("%s: no device found", kind)
		return nil
	case st.HardBlock:
		m.err = fmt.Errorf("%s is hardware-blocked; software can't enable it", kind)
		return nil
	}
	script, err := power.ScriptPath()
	if err != nil {
		m.err = fmt.Errorf("cannot find battery-saver.sh: %w", err)
		return nil
	}
	arg := "off"
	if st.Off {
		arg = "on"
	}
	m.busy = true
	m.err = nil

	cmd := exec.Command("sudo", script, kind, arg)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return toggleDoneMsg{err: err}
	})
}

// setBrightness nudges the backlight by delta percent via brightnessctl (no
// root needed), clamps to [brightnessMin, 100], then re-scrapes so the bar
// reflects the change without waiting for the next tick.
func (m *App) setBrightness(delta int) tea.Cmd {
	if !m.snap.BrightnessPresent {
		return nil
	}
	target := m.snap.BrightnessPct + delta
	if target < brightnessMin {
		target = brightnessMin
	}
	if target > 100 {
		target = 100
	}
	arg := fmt.Sprintf("%d%%", target)
	return func() tea.Msg {
		_ = exec.Command("brightnessctl", "set", arg).Run()
		ctx, cancel := context.WithTimeout(context.Background(), scrapeInterval)
		defer cancel()
		m.scraper.Scrape(ctx)
		return snapshotMsg{m.scraper.Snapshot()}
	}
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
