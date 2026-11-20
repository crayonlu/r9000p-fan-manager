package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type act int

const (
	actQuietOn act = iota
	actQuietOff
	actMax30
	actLowPower
	actBalanced
	actPerformance
	actCustom
	actRefresh
)

type menuItem struct {
	title, desc string
	action      act
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.desc }
func (i menuItem) FilterValue() string { return i.title }

type tickMsg time.Time
type doneMsg struct {
	text string
	err  bool
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	statStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

type model struct {
	list   list.Model
	spin   spinner.Model
	hw     string
	status Status
	busy   bool
	msg    string
	isErr  bool
}

func newModel(hw string) model {
	items := []list.Item{
		menuItem{"Quiet mode: ON", "power-saver + gentle fan curve (low noise)", actQuietOn},
		menuItem{"Quiet mode: OFF", "restore previous profile and fan curve", actQuietOff},
		menuItem{"Max fan: 30s", "spin fans to max for 30s, then restore", actMax30},
		menuItem{"Profile: low-power", "set platform profile to low-power", actLowPower},
		menuItem{"Profile: balanced", "set platform profile to balanced", actBalanced},
		menuItem{"Profile: performance", "set platform profile to performance", actPerformance},
		menuItem{"Profile: custom", "set platform profile to custom", actCustom},
		menuItem{"Refresh", "re-read fan / temperature", actRefresh},
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "r9000p-fan-manager"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle

	sp := spinner.New()
	sp.Spinner = spinner.Line
	return model{list: l, spin: sp, hw: hw, status: readStatus(hw)}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tick(), m.spin.Tick)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func runAction(hw string, a act) tea.Cmd {
	return func() tea.Msg {
		var err error
		var text string
		switch a {
		case actQuietOn:
			err, text = quietOn(hw), "Quiet mode: ON"
		case actQuietOff:
			err, text = quietOff(hw), "Quiet mode: OFF"
		case actMax30:
			if err = maxFan(hw, 30, nil); err == nil {
				text = "Max fan done, previous state restored"
			}
		case actLowPower:
			err, text = setProfile("low-power"), "profile set to low-power"
		case actBalanced:
			err, text = setProfile("balanced"), "profile set to balanced"
		case actPerformance:
			err, text = setProfile("performance"), "profile set to performance"
		case actCustom:
			err, text = setProfile("custom"), "profile set to custom"
		case actRefresh:
			text = "Refreshed"
		}
		if err != nil {
			return doneMsg{text: "Error: " + err.Error(), err: true}
		}
		return doneMsg{text: text}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - 8
		if h < 5 {
			h = 5
		}
		m.list.SetSize(msg.Width, h)
		return m, nil

	case tickMsg:
		m.status = readStatus(m.hw)
		return m, tick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case doneMsg:
		m.busy = false
		m.status = readStatus(m.hw)
		m.msg = msg.text
		m.isErr = msg.err
		return m, nil

	case tea.KeyMsg:
		if m.busy {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(menuItem); ok {
				m.busy = true
				m.msg = "working..."
				m.isErr = false
				return m, tea.Batch(m.spin.Tick, runAction(m.hw, it.action))
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	head := titleStyle.Render("r9000p-fan-manager") + "  " +
		statStyle.Render("q: quit · enter: run")
	stats := statStyle.Render(fmt.Sprintf(
		"profile %s  •  power %s  •  governor %s", m.status.Profile, m.status.PPD, m.status.Governor)) + "\n" +
		statStyle.Render(fmt.Sprintf(
			"fan %d/%d RPM  •  temp %d/%d °C", m.status.Fan1, m.status.Fan2, m.status.Temp1, m.status.Temp2))

	body := m.list.View()
	if m.busy {
		body = m.spin.View() + " " + m.msg
	}

	footer := ""
	if m.msg != "" && !m.busy {
		if m.isErr {
			footer = errStyle.Render(m.msg)
		} else {
			footer = okStyle.Render(m.msg)
		}
	}
	return head + "\n" + stats + "\n\n" + body + "\n" + footer
}

func runTUI() {
	ensureRoot()
	hw, err := findHwmon()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	p := tea.NewProgram(newModel(hw), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("TUI error:", err)
	}
}
