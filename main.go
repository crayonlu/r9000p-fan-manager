package main

import (
	"fmt"
	"os"
	"strconv"
)

var version = "dev"

func usage() {
	fmt.Print(`r9000p-fan-manager (command: fan) - fan/thermal control for Lenovo Legion R9000P

Usage:
  fan                    Interactive TUI
  fan status             Print profile / power / governor / fan / temp
  fan quiet on|off       Night/quiet mode: power-saver + gentle fan curve
  fan max [seconds]      Max fan for N seconds (default 60), then restore
  fan profile <name>     Set platform profile: low-power|balanced|performance|custom
  fan version            Show version

Run as a normal user; the tool re-execs itself with sudo when needed.
`)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func printStatus() {
	hw, _ := findHwmon()
	st := readStatus(hw)
	fmt.Printf("profile : %s\npower   : %s\ngovernor: %s\nfan     : %d/%d RPM\ntemp    : %d/%d C\n",
		st.Profile, st.PPD, st.Governor, st.Fan1, st.Fan2, st.Temp1, st.Temp2)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runTUI()
		return
	}
	switch args[0] {
	case "status":
		printStatus()
	case "quiet":
		if len(args) < 2 || (args[1] != "on" && args[1] != "off") {
			fmt.Println("usage: fan quiet on|off")
			os.Exit(1)
		}
		ensureRoot()
		hw, err := findHwmon()
		if err != nil {
			die(err)
		}
		if args[1] == "on" {
			if e := quietOn(hw); e != nil {
				die(e)
			}
			fmt.Println("Quiet mode: ON")
		} else {
			if e := quietOff(hw); e != nil {
				die(e)
			}
			fmt.Println("Quiet mode: OFF")
		}
	case "max":
		sec := 60
		if len(args) >= 2 {
			if v, e := strconv.Atoi(args[1]); e == nil && v > 0 {
				sec = v
			}
		}
		ensureRoot()
		hw, err := findHwmon()
		if err != nil {
			die(err)
		}
		fmt.Printf("Max fan for %ds...\n", sec)
		err = maxFan(hw, sec, func(rem int, st Status) {
			fmt.Printf("\r  %2ds left   fan %d/%d RPM   %d/%d C   ", rem, st.Fan1, st.Fan2, st.Temp1, st.Temp2)
		})
		fmt.Println()
		if err != nil {
			die(err)
		}
		fmt.Println("Done, previous state restored.")
	case "profile":
		if len(args) < 2 {
			fmt.Println("usage: fan profile <low-power|balanced|performance|custom>")
			os.Exit(1)
		}
		ensureRoot()
		if e := setProfile(args[1]); e != nil {
			die(e)
		}
		fmt.Printf("profile set to %s\n", args[1])
	case "version", "-v", "--version":
		fmt.Println("fan", version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
	}
}
