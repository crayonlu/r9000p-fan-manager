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
	vSensors
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
)

type advItem struct{ label, kind string }

var advItems = []advItem{
	{"Platform profile · low-power", "profile:low-power"},
	{"Platform profile · balanced", "profile:balanced"},
	{"Platform profile · performance", "profile:performance"},
	{"Platform profile · custom", "profile:custom"},
	{"Fan curve · default (EC)", "curve:default"},
	{"Fan curve · quiet", "curve:quiet"},
	{"Fan curve · max", "curve:max"},
	{"CPU governor · powersave", "gov:powersave"},
	{"CPU governor · performance", "gov:performance"},
	{"Sensors ›", "sensors"},
	{"Diagnostics (raw values)", "diag"},
	{"‹ Back", "back"},
}

type model struct {
	v         viewKind
	w, h      int
	hw        string
	st        Status
	curDash   int
	curAdv    int
	sensors   []Sensor
	curSens   int
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.st = readStatus(m.hw)
		if m.v == vSensors {
			m.sensors = readSensors()
		}
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
		switch m.v {
		case vDash:
			return m.updateDash(msg)
		case vAdv:
			return m.updateAdv(msg)
		default:
			return m.updateSensors(msg)
		}
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
		if m.curAdv < len(advItems)-1 {
			m.curAdv++
		}
	case "esc", "left", "h", "backspace":
		m.v = vDash
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.toast, m.toastErr = "", false
		it := advItems[m.curAdv]
		switch {
		case strings.HasPrefix(it.kind, "profile:"):
			name := strings.TrimPrefix(it.kind, "profile:")
			m.toast, m.toastErr = apply(func() error { return setProfile(name) }, "profile set to "+name)
		case strings.HasPrefix(it.kind, "curve:"):
			name := strings.TrimPrefix(it.kind, "curve:")
			m.toast, m.toastErr = apply(func() error { return setCurvePreset(m.hw, name) }, "fan curve: "+name)
		case strings.HasPrefix(it.kind, "gov:"):
			setGovernor(strings.TrimPrefix(it.kind, "gov:"))
			m.toast = "governor: " + strings.TrimPrefix(it.kind, "gov:")
		case it.kind == "sensors":
			m.sensors = readSensors()
			m.curSens = 0
			m.v = vSensors
		case it.kind == "diag":
			m.toast = fmt.Sprintf("profile=%s ppd=%s gov=%s fan=%d/%d temp=%d/%d",
				m.st.Profile, m.st.PPD, m.st.Governor, m.st.Fan1, m.st.Fan2, m.st.Temp1, m.st.Temp2)
		case it.kind == "back":
			m.v = vDash
		}
		m.st = readStatus(m.hw)
	}
	return m, nil
}

func (m model) updateSensors(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.curSens > 0 {
			m.curSens--
		}
	case "down", "j":
		if m.curSens < len(m.sensors)-1 {
			m.curSens++
		}
	case "esc", "left", "h", "backspace":
		m.v = vAdv
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func apply(fn func() error, okMsg string) (string, bool) {
	if err := fn(); err != nil {
		return "Error: " + err.Error(), true
	}
	return okMsg, false
}

// ---- render helpers ----

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

// ---- views ----

func (m model) View() string {
	switch m.v {
	case vAdv:
		return m.viewAdv()
	case vSensors:
		return m.viewSensors()
	default:
		return m.viewDash()
	}
}

func (m model) viewDash() string {
	w := m.width()
	div := stDiv.Render(strings.Repeat("─", w))
	hl, hs := healthOf(m.st)

	var b strings.Builder
	b.WriteString(spread(stTitle.Render("● R9000P Fan"), hs.Render(hl), w) + "\n")
	b.WriteString(div + "\n")
	b.WriteString(fmt.Sprintf("FAN  %d / %d rpm %s\n", m.st.Fan1, m.st.Fan2, bar(m.st.Fan1)))
	b.WriteString(fmt.Sprintf("TEMP %s / %s      POWER %s\n",
		tempStyle(m.st.Temp1).Render(fmt.Sprintf("CPU %d°C", m.st.Temp1)),
		tempStyle(m.st.Temp2).Render(fmt.Sprintf("GPU %d°C", m.st.Temp2)), m.st.PPD))
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
	b.WriteString(m.rowLine(4, "Advanced ›      sensors · fan curve · profile", false) + "\n")
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
	b.WriteString(spread(stMuted.Render("↑↓ move  ↵ apply  q quit"),
		stMuted.Render(fmt.Sprintf("updated %ds ago", ago)), w) + "\n")
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
	var b strings.Builder
	b.WriteString(spread(stTitle.Render("R9000P Fan · Advanced"), stMuted.Render("esc back"), w) + "\n")
	b.WriteString(div + "\n")
	for i, it := range advItems {
		mark := "  "
		if i == m.curAdv {
			mark = "▸ "
		}
		line := mark + it.label
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

func (m model) viewSensors() string {
	w := m.width()
	div := stDiv.Render(strings.Repeat("─", w))
	var b strings.Builder
	b.WriteString(spread(stTitle.Render("R9000P Fan · Sensors"), stMuted.Render("esc back"), w) + "\n")
	b.WriteString(div + "\n")

	visible := m.h - 6
	if visible < 5 {
		visible = 5
	}
	n := len(m.sensors)
	off := 0
	if m.curSens >= visible {
		off = m.curSens - visible + 1
	}
	end := off + visible
	if end > n {
		end = n
	}
	if n == 0 {
		b.WriteString(stMuted.Render("no sensors found") + "\n")
	}
	for i := off; i < end; i++ {
		s := m.sensors[i]
		mark := "  "
		if i == m.curSens {
			mark = "▸ "
		}
		name := fmt.Sprintf("%-8s %-22s", sensorTitle(s), s.Label)
		line := mark + name + tempStyle(s.Temp).Render(fmt.Sprintf("%3d°C", s.Temp))
		if i == m.curSens {
			b.WriteString(stSel.Render(mark+name) + tempStyle(s.Temp).Render(fmt.Sprintf("%3d°C", s.Temp)) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString(div + "\n")
	pos := ""
	if n > 0 {
		pos = fmt.Sprintf("%d/%d", m.curSens+1, n)
	}
	b.WriteString(spread(stMuted.Render("↑↓ scroll   esc/← back   q quit"), stMuted.Render(pos), w) + "\n")
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
