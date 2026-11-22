package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// withFakeSysfs points the package path variables at a throwaway directory tree
// and restores them when the test finishes.
func withFakeSysfs(t *testing.T) (hw, profile string) {
	t.Helper()
	dir := t.TempDir()
	hw = filepath.Join(dir, "hwmon0")
	if err := os.MkdirAll(hw, 0o755); err != nil {
		t.Fatal(err)
	}
	profile = filepath.Join(dir, "platform_profile")
	if err := os.WriteFile(profile, []byte("balanced"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldClass, oldACPI, oldState := profileClassPath, profileACPIpath, statePath
	profileClassPath = profile
	profileACPIpath = filepath.Join(dir, "acpi_profile") // does not exist
	statePath = filepath.Join(dir, "state.json")
	t.Cleanup(func() {
		profileClassPath, profileACPIpath, statePath = oldClass, oldACPI, oldState
	})
	return hw, profile
}

func setPWM(t *testing.T, hw string, v int) {
	t.Helper()
	for i := 1; i <= 10; i++ {
		p := filepath.Join(hw, "pwm1_auto_point"+strconv.Itoa(i)+"_pwm")
		if err := os.WriteFile(p, []byte(strconv.Itoa(v)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readPWM(t *testing.T, hw string) []int {
	t.Helper()
	out := make([]int, 10)
	for i := 1; i <= 10; i++ {
		p := filepath.Join(hw, "pwm1_auto_point"+strconv.Itoa(i)+"_pwm")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[i-1], _ = strconv.Atoi(string(b))
	}
	return out
}

func writeState(t *testing.T, st savedState) {
	t.Helper()
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readProfile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Returning to a firmware-managed mode must restore the profile and must not
// write a leftover software fan curve, which would keep the EC in custom.
func TestRestoreNonCustomSkipsCurve(t *testing.T) {
	hw, profile := withFakeSysfs(t)
	setPWM(t, hw, 255) // as left behind by a max-fan run
	stored := make([]int, 10)
	for i := range stored {
		stored[i] = 120
	}
	writeState(t, savedState{Profile: "balanced", Curve: stored})

	if err := restoreState(hw); err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, profile); got != "balanced" {
		t.Fatalf("profile = %q, want balanced", got)
	}
	for i, v := range readPWM(t, hw) {
		if v != 255 {
			t.Fatalf("point %d = %d, curve should not be written for non-custom mode", i+1, v)
		}
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file should be removed after successful restore")
	}
}

// Returning to custom must restore the profile and then write the stored curve
// back (writing it before the profile switch would let the firmware wipe it).
func TestRestoreCustomWritesStoredCurve(t *testing.T) {
	hw, profile := withFakeSysfs(t)
	setPWM(t, hw, 255)
	writeState(t, savedState{Profile: "custom", Curve: quietCurve})

	if err := restoreState(hw); err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, profile); got != "custom" {
		t.Fatalf("profile = %q, want custom", got)
	}
	got := readPWM(t, hw)
	for i, want := range quietCurve {
		if got[i] != want {
			t.Fatalf("curve point %d = %d, want %d", i+1, got[i], want)
		}
	}
}

// End to end: max fan from balanced returns to balanced.
func TestMaxFanRestoresPreviousProfile(t *testing.T) {
	hw, profile := withFakeSysfs(t)
	setPWM(t, hw, 120)

	cancel := make(chan struct{})
	if err := maxFanCancelable(hw, 0, cancel); err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, profile); got != "balanced" {
		t.Fatalf("profile after max fan = %q, want balanced", got)
	}
}
