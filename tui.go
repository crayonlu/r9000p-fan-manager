package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewKind int

const (
	vDash viewKind = iota
	vAdv
)

type tickMsg time.Time
type doneMsg struct {
	text string
	err  bool
}

var (
	stTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	stMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	stDiv    = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	stGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	stRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	stActive = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	stSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	stSelBg  = lipgloss.NewStyle().Background(lipgloss.Color("236")).Bold(true)
)

type model struct {
	v         viewKind
	w, h      int
	hw        string
	st        Status
	curDash   int
	curAdv    int
	toast     string
	toastErr  bool
	busy      bool
	busyMsg   string
	countdown int
	cancel    chan struct{}
	updated   time.Time
}

func newModel(hw string) model {
	return model{hw: hw, st: readStatus(hw), updated: time.Now()}
}

func (m model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func runMaxCmd(hw string, sec int, cancel <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if err := maxFanCancelable(hw, sec, cancel); err != nil {
			return doneMsg{"Error: " + err.Error(), true}
		}
		return doneMsg{"Max fan stopped, previous state restored", false}
	}
}

// ---- update ----

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.st = readStatus(m.hw)
		m.updated = time.Now()
		if m.busy && m.countdown > 0 {
			m.countdown--
			if m.countdown == 0 {
				m.busyMsg = "restoring…"
			}
		}
		return m, tick()
	case doneMsg:
		m.busy = false
		m.cancel = nil
		m.countdown = 0
		m.toast = msg.text
		m.toastErr = msg.err
		m.st = readStatus(m.hw)
		return m, nil
	case tea.KeyMsg:
		if m.busy {
			if msg.String() == "esc" {
				if m.cancel != nil {
					close(m.cancel)
					m.cancel = nil
				}
				m.busyMsg = "cancelling…"
			}
			return m, nil
		}
		if m.v == vDash {
			return m.updateDash(msg)
		}
		return m.updateAdv(msg)
	}
	return m, nil
}

func (m model) updateDash(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.curDash > 0 {
			m.curDash--
		}
	case "down", "j":
		if m.curDash < 4 {
			m.curDash++
		}
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		m.toast, m.toastErr = "", false
		switch m.curDash {
		case 0:
			m.toast, m.toastErr = apply(func() error { return setMode(m.hw, modeQuiet) }, "Quiet mode applied")
		case 1:
			m.toast, m.toastErr = apply(func() error { return setMode(m.hw, modeBalanced) }, "Balanced mode applied")
		case 2:
			m.toast, m.toastErr = apply(func() error { return setMode(m.hw, modePerformance) }, "Performance mode applied")
		case 3:
			ch := make(chan struct{})
			m.cancel = ch
			m.busy = true
			m.countdown = 30
			m.busyMsg = "Max fan running"
			return m, tea.Batch(tick(), runMaxCmd(m.hw, 30, ch))
		case 4:
			m.v = vAdv
			m.curAdv = 0
		}
		m.st = readStatus(m.hw)
	}
	return m, nil
}

func (m model) updateAdv(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.curAdv > 0 {
			m.curAdv--
		}
	case "down", "j":
		if m.curAdv < 10 {
			m.curAdv++
		}
	case "esc", "left", "h", "backspace":
		m.v = vDash
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.toast, m.toastErr = "", false
		switch m.curAdv {
		case 0, 1, 2, 3:
			names := []string{"low-power", "balanced", "performance", "custom"}
			m.toast, m.toastErr = apply(func() error { return setProfile(names[m.curAdv]) }, "profile set to "+names[m.curAdv])
		case 4:
			m.toast, m.toastErr = apply(func() error { return setCurvePreset(m.hw, "default") }, "fan curve: default")
		case 5:
			m.toast, m.toastErr = apply(func() error { return setCurvePreset(m.hw, "quiet") }, "fan curve: quiet")
		case 6:
			m.toast, m.toastErr = apply(func() error { return setCurvePreset(m.hw, "max") }, "fan curve: max")
		case 7:
			setGovernor("powersave")
			m.toast = "governor: powersave"
		case 8:
			setGovernor("performance")
			m.toast = "governor: performance"
		case 9:
			m.toast = fmt.Sprintf("raw: profile=%s ppd=%s gov=%s fan=%d/%d temp=%d/%d",
				m.st.Profile, m.st.PPD, m.st.Governor, m.st.Fan1, m.st.Fan2, m.st.Temp1, m.st.Temp2)
		case 10:
			m.v = vDash
		}
		m.st = readStatus(m.hw)
	}
	return m, nil
}

func apply(fn func() error, okMsg string) (string, bool) {
	if err := fn(); err != nil {
		return "Error: " + err.Error(), true
	}
	return okMsg, false
}

// ---- render ----

func (m model) width() int {
	w := m.w
	if w <= 0 {
		w = 66
	}
	if w > 74 {
		w = 74
	}
	if w < 40 {
		w = 40
	}
	return w
}

func healthOf(st Status) (string, lipgloss.Style) {
	if st.Temp1 == 0 && st.Temp2 == 0 {
		return "N/A", stMuted
	}
	t := st.Temp1
	if st.Temp2 > t {
		t = st.Temp2
	}
	switch {
	case t >= 85:
		return "HOT", stRed
	case t >= 70:
		return "WARM", stYellow
	default:
		return "HEALTHY", stGreen
	}
}

func tempStyle(t int) lipgloss.Style {
	switch {
	case t >= 85:
		return stRed
	case t >= 70:
		return stYellow
	default:
		return stGreen
	}
}

func bar(rpm int) string {
	n := rpm * 10 / 5500
	if n < 0 {
		n = 0
	}
	if n > 10 {
		n = 10
	}
	return "[" + strings.Repeat("▓", n) + strings.Repeat("░", 10-n) + "]"
}

func spread(l, r string, w int) string {
	pad := w - lipgloss.Width(l) - lipgloss.Width(r)
	if pad < 1 {
		pad = 1
	}
	return l + strings.Repeat(" ", pad) + r
}

func (m model) View() string {
	if m.v == vAdv {
		return m.viewAdv()
	}
	return m.viewDash()
}

func (m model) viewDash() string {
	w := m.width()
	div := stDiv.Render(strings.Repeat("─", w))
	hl, hs := healthOf(m.st)

	var b strings.Builder
	b.WriteString(spread(stTitle.Render("● R9000P Fan"), hs.Render(hl), w) + "\n")
	b.WriteString(div + "\n")

	fanLine := fmt.Sprintf("FAN  %d / %d rpm %s", m.st.Fan1, m.st.Fan2, bar(m.st.Fan1))
	b.WriteString(fanLine + "\n")
	b.WriteString(fmt.Sprintf("TEMP %s / %s      POWER %s", tempStyle(m.st.Temp1).Render(fmt.Sprintf("%d°C", m.st.Temp1)),
		tempStyle(m.st.Temp2).Render(fmt.Sprintf("%d°C", m.st.Temp2)), m.st.PPD) + "\n")
	b.WriteString(div + "\n")

	active := currentMode(m.st)
	modes := []struct{ key, label, desc string }{
		{modeQuiet, "Quiet", "low noise (power-saver)"},
		{modeBalanced, "Balanced", "everyday"},
		{modePerformance, "Performance", "max perf (louder)"},
	}
	b.WriteString(stMuted.Render("Mode") + "\n")
	for i, mo := range modes {
		b.WriteString(m.rowLine(i, fmt.Sprintf("%-12s %s", mo.label, mo.desc), active == mo.key) + "\n")
	}
	b.WriteString("\n" + stMuted.Render("Actions") + "\n")
	b.WriteString(m.rowLine(3, "Max fan 30s     spin to max, then restore", false) + "\n")
	b.WriteString(m.rowLine(4, "Advanced ›      fan curve · raw profile · governor", false) + "\n")
	b.WriteString(div + "\n")

	if m.busy {
		b.WriteString(stSel.Render(fmt.Sprintf("%s… %ds left (Esc to cancel)", m.busyMsg, m.countdown)) + "\n")
	} else if m.toast != "" {
		st := stGreen
		if m.toastErr {
			st = stRed
		}
		b.WriteString(st.Render(m.toast) + "\n")
	} else {
		b.WriteString("\n")
	}
	ago := int(time.Since(m.updated).Seconds())
	help := stMuted.Render("↑↓ move  ↵ apply  q quit")
	b.WriteString(spread(help, stMuted.Render(fmt.Sprintf("updated %ds ago", ago)), w) + "\n")
	return b.String()
}

func (m model) rowLine(idx int, text string, active bool) string {
	sel := m.curDash == idx
	mark := "  "
	if sel {
		mark = "▸ "
	}
	check := "○ "
	if active {
		check = "● "
	}
	line := mark + check + text
	if active {
		return stActive.Render(line)
	}
	if sel {
		return stSel.Render(line)
	}
	return line
}

func (m model) viewAdv() string {
	w := m.width()
	div := stDiv.Render(strings.Repeat("─", w))
	rows := []string{
		"Platform profile · low-power",
		"Platform profile · balanced",
		"Platform profile · performance",
		"Platform profile · custom",
		"Fan curve · default (EC)",
		"Fan curve · quiet",
		"Fan curve · max",
		"CPU governor · powersave",
		"CPU governor · performance",
		"Diagnostics (show raw values)",
		"‹ Back",
	}
	var b strings.Builder
	b.WriteString(spread(stTitle.Render("R9000P Fan · Advanced"), stMuted.Render("esc back"), w) + "\n")
	b.WriteString(div + "\n")
	for i, r := range rows {
		mark := "  "
		if i == m.curAdv {
			mark = "▸ "
		}
		line := mark + r
		if i == m.curAdv {
			b.WriteString(stSel.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString(div + "\n")
	if m.toast != "" {
		st := stGreen
		if m.toastErr {
			st = stRed
		}
		b.WriteString(st.Render(m.toast) + "\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString(stMuted.Render("↑↓ move   ↵ apply   esc/← back   q quit") + "\n")
	return b.String()
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
