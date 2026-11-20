package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func runTUI() {
	ensureRoot()
	hw, err := findHwmon()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	restore := makeRaw()
	defer restore()

	events := make(chan string, 32)
	go readKeys(events)

	items := []string{
		"Refresh",
		"Quiet mode: ON  (low noise)",
		"Quiet mode: OFF (restore)",
		"Max fan: 30s",
		"Profile: low-power",
		"Profile: balanced",
		"Profile: performance",
		"Profile: custom",
		"Quit",
	}
	sel := 1
	msg := "arrows/jk move, Enter run, q quit"

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		draw(readStatus(hw), items, sel, msg)
		select {
		case ev := <-events:
			switch ev {
			case "up":
				if sel > 0 {
					sel--
				}
			case "down":
				if sel < len(items)-1 {
					sel++
				}
			case "quit":
				return
			case "enter":
				switch sel {
				case 0:
					msg = "Refreshed."
				case 1:
					if e := quietOn(hw); e != nil {
						msg = "Error: " + e.Error()
					} else {
						msg = "Quiet mode ON."
					}
				case 2:
					if e := quietOff(hw); e != nil {
						msg = "Error: " + e.Error()
					} else {
						msg = "Quiet mode OFF."
					}
				case 3:
					err := maxFan(hw, 30, func(rem int, s Status) {
						draw(s, items, sel, fmt.Sprintf("Max fan running: %ds left", rem))
					})
					if err != nil {
						msg = "Error: " + err.Error()
					} else {
						msg = "Max fan done, restored."
					}
				case 4:
					msg = applyProfile("low-power")
				case 5:
					msg = applyProfile("balanced")
				case 6:
					msg = applyProfile("performance")
				case 7:
					msg = applyProfile("custom")
				case 8:
					return
				}
			}
		case <-ticker.C:
		}
	}
}

func applyProfile(name string) string {
	if err := setProfile(name); err != nil {
		return "Error: " + err.Error()
	}
	return "profile set to " + name
}

func draw(st Status, items []string, sel int, msg string) {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString("r9000p-fan-manager\n")
	b.WriteString("------------------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("profile : %-10s power: %-12s gov: %s\n", st.Profile, st.PPD, st.Governor))
	b.WriteString(fmt.Sprintf("fan     : %d/%d RPM        temp: %d/%d C\n", st.Fan1, st.Fan2, st.Temp1, st.Temp2))
	b.WriteString("------------------------------------------------------------\n")
	for i, it := range items {
		cur := "  "
		if i == sel {
			cur = "> "
		}
		b.WriteString(cur + it + "\n")
	}
	b.WriteString("------------------------------------------------------------\n")
	b.WriteString(msg + "\n")
	_, _ = os.Stdout.WriteString(b.String())
}

func readKeys(ch chan<- string) {
	r := bufio.NewReader(os.Stdin)
	for {
		b, err := r.ReadByte()
		if err != nil {
			close(ch)
			return
		}
		switch b {
		case 'q', 'Q':
			ch <- "quit"
		case 'k':
			ch <- "up"
		case 'j':
			ch <- "down"
		case '\r', '\n':
			ch <- "enter"
		case 27: // ESC sequence (arrow keys)
			b2, err2 := r.ReadByte()
			if err2 != nil {
				continue
			}
			if b2 == '[' {
				b3, err3 := r.ReadByte()
				if err3 != nil {
					continue
				}
				switch b3 {
				case 'A':
					ch <- "up"
				case 'B':
					ch <- "down"
				}
			}
		}
	}
}

// makeRaw puts the controlling terminal in raw mode and returns a restore func.
func makeRaw() func() {
	saved, _ := exec.Command("stty", "-F", "/dev/tty", "-g").Output()
	_ = exec.Command("stty", "-F", "/dev/tty", "raw", "-echo").Run()
	fmt.Print("\x1b[?25l") // hide cursor
	return func() {
		_ = exec.Command("stty", "-F", "/dev/tty", strings.TrimSpace(string(saved))).Run()
		fmt.Print("\x1b[?25h\x1b[2J\x1b[H") // show cursor, clear
	}
}
