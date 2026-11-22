package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Paths are variables rather than constants so tests can point them at a fake
// sysfs tree.
var (
	profileClassPath = "/sys/class/platform-profile/platform-profile-0/profile"
	profileACPIpath  = "/sys/firmware/acpi/platform_profile"
	statePath        = "/var/tmp/r9000p-fan-manager.state"
)

// Quiet curve: lower fan at low/mid temperatures, keep strong cooling at high
// temps. Values below ~25 are rejected by the EC, so keep point 1 at 25.
var quietCurve = []int{25, 35, 50, 70, 95, 153, 178, 204, 204, 204}

// Status is a snapshot of thermal/fan state.
type Status struct {
	Profile  string
	PPD      string
	Governor string
	Fan1     int
	Fan2     int
	Temp1    int
	Temp2    int
}

type savedState struct {
	Profile string
	PPD     string
	Curve   []int
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func profilePath() string {
	if fileExists(profileClassPath) {
		return profileClassPath
	}
	return profileACPIpath
}

func readStr(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readInt(p string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(readStr(p)))
	return v
}

func readIntMillis(p string) int {
	return readInt(p) / 1000
}

func findHwmon() (string, error) {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, d := range dirs {
		if readStr(filepath.Join(d, "name")) == "legion_hwmon" {
			return d, nil
		}
	}
	return "", errors.New("legion_hwmon not found (is the legion_laptop module loaded?)")
}

func readStatus(hw string) Status {
	s := Status{
		Profile:  readStr(profilePath()),
		PPD:      ppdGet(),
		Governor: readStr("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"),
	}
	if hw != "" {
		s.Fan1 = readInt(filepath.Join(hw, "fan1_input"))
		s.Fan2 = readInt(filepath.Join(hw, "fan2_input"))
		s.Temp1 = readIntMillis(filepath.Join(hw, "temp1_input"))
		s.Temp2 = readIntMillis(filepath.Join(hw, "temp2_input"))
	}
	return s
}

func readCurve(hw string) ([]int, error) {
	c := make([]int, 0, 10)
	for i := 1; i <= 10; i++ {
		c = append(c, readInt(filepath.Join(hw, fmt.Sprintf("pwm1_auto_point%d_pwm", i))))
	}
	return c, nil
}

// writeCurve writes the 10 fan-curve points best-effort: a point the EC rejects
// (e.g. a value below its floor) is skipped instead of aborting the whole write.
func writeCurve(hw string, c []int) error {
	written := 0
	var firstErr error
	for i := 1; i <= 10 && i <= len(c); i++ {
		p := filepath.Join(hw, fmt.Sprintf("pwm1_auto_point%d_pwm", i))
		if err := os.WriteFile(p, []byte(strconv.Itoa(c[i-1])), 0644); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("write %s: %w", p, err)
			}
			continue
		}
		written++
	}
	if written == 0 {
		return firstErr
	}
	return nil
}

func setProfile(name string) error {
	if err := os.WriteFile(profilePath(), []byte(name), 0644); err != nil {
		return fmt.Errorf("set profile %s: %w", name, err)
	}
	return nil
}

func setGovernor(name string) {
	files, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/scaling_governor")
	for _, p := range files {
		_ = os.WriteFile(p, []byte(name), 0644)
	}
}

func setEPP(name string) {
	files, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/energy_performance_preference")
	for _, p := range files {
		_ = os.WriteFile(p, []byte(name), 0644)
	}
}

func ppdGet() string {
	out, err := exec.Command("powerprofilesctl", "get").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ppdSet(name string) {
	_ = exec.Command("powerprofilesctl", "set", name).Run()
}

func saveState(hw string) error {
	c, _ := readCurve(hw)
	st := savedState{Profile: readStr(profilePath()), PPD: ppdGet(), Curve: c}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, b, 0644)
}

func loadState() (savedState, error) {
	var st savedState
	b, err := os.ReadFile(statePath)
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(b, &st)
	return st, err
}

func restoreState(hw string) error {
	st, err := loadState()
	if err != nil {
		return err
	}
	var firstErr error

	// Restore in the same order the modes are applied (see setMode): power
	// profile first, then the platform profile. The platform profile is set
	// last so it stays authoritative over power-profiles-daemon.
	if st.PPD != "" {
		ppdSet(st.PPD)
	}
	if st.Profile != "" {
		if e := setProfile(st.Profile); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	// Writing the platform profile makes the firmware load that mode's own
	// default fan curve, and custom curve points only take effect in custom
	// mode. So a stored curve must be written after switching to custom, and
	// must not be written at all when returning to a firmware-managed mode
	// (otherwise a leftover software curve keeps the EC in custom).
	if st.Profile == "custom" && len(st.Curve) == 10 {
		if e := writeCurve(hw, st.Curve); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	if firstErr == nil {
		_ = os.Remove(statePath)
	}
	return firstErr
}

// quietOn applies the night/quiet profile: power-saver + gentle fan curve.
func quietOn(hw string) error {
	if err := saveState(hw); err != nil {
		return err
	}
	ppdSet("power-saver")
	setGovernor("powersave")
	setEPP("power")
	if err := setProfile("custom"); err != nil {
		return err
	}
	return writeCurve(hw, quietCurve)
}

// quietOff restores the pre-quiet state (or balanced if none saved).
func quietOff(hw string) error {
	if _, err := os.Stat(statePath); err == nil {
		return restoreState(hw)
	}
	ppdSet("balanced")
	setGovernor("powersave")
	_ = setProfile("balanced")
	return nil
}

// maxFan runs fans at max for sec seconds, then restores the previous state.
func maxFan(hw string, sec int, progress func(remaining int, st Status)) (err error) {
	if err := saveState(hw); err != nil {
		return err
	}
	defer func() {
		if rerr := restoreState(hw); rerr != nil && err == nil {
			err = rerr
		}
	}()
	all := make([]int, 10)
	for i := range all {
		all[i] = 255
	}
	_ = setProfile("custom")
	if err := writeCurve(hw, all); err != nil {
		return err
	}
	for rem := sec; rem > 0; rem-- {
		if progress != nil {
			progress(rem, readStatus(hw))
		}
		time.Sleep(time.Second)
	}
	return nil
}

func ensureRoot() {
	if os.Geteuid() == 0 {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cmd := exec.Command("sudo", append([]string{"-n", exe}, os.Args[1:]...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "root required (sudo -n failed). Re-run with sudo.")
		os.Exit(1)
	}
	os.Exit(0)
}
