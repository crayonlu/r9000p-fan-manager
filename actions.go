package main

import "time"

// Mode names surfaced to the UI.
const (
	modeQuiet       = "quiet"
	modeBalanced    = "balanced"
	modePerformance = "performance"
)

// setMode applies one of the three primary modes (idempotent).
func setMode(hw, mode string) error {
	switch mode {
	case modeQuiet:
		ppdSet("power-saver")
		setGovernor("powersave")
		setEPP("power")
		if err := setProfile("custom"); err != nil {
			return err
		}
		return writeCurve(hw, quietCurve)
	case modePerformance:
		ppdSet("performance")
		setGovernor("powersave")
		return setProfile("performance")
	default: // balanced
		ppdSet("balanced")
		setGovernor("powersave")
		return setProfile("balanced")
	}
}

// currentMode infers the active mode from the live state.
func currentMode(st Status) string {
	if st.Profile == "performance" || st.PPD == "performance" {
		return modePerformance
	}
	if st.Profile == "custom" && st.PPD == "power-saver" {
		return modeQuiet
	}
	return modeBalanced
}

// setCurvePreset applies a fan-curve preset (advanced).
func setCurvePreset(hw, preset string) error {
	switch preset {
	case "quiet":
		if err := setProfile("custom"); err != nil {
			return err
		}
		return writeCurve(hw, quietCurve)
	case "max":
		all := make([]int, 10)
		for i := range all {
			all[i] = 255
		}
		if err := setProfile("custom"); err != nil {
			return err
		}
		return writeCurve(hw, all)
	default: // EC default curve
		return setProfile("balanced")
	}
}

// maxFanCancelable spins fans to max for sec seconds, restoring the previous
// state afterwards. Closing cancel aborts early (still restores).
func maxFanCancelable(hw string, sec int, cancel <-chan struct{}) (err error) {
	if err := saveState(hw); err != nil {
		return err
	}
	defer func() {
		if rerr := restoreState(hw); rerr != nil && err == nil {
			err = rerr
		}
	}()
	if err := setProfile("custom"); err != nil {
		return err
	}
	all := make([]int, 10)
	for i := range all {
		all[i] = 255
	}
	if err := writeCurve(hw, all); err != nil {
		return err
	}
	for rem := sec; rem > 0; rem-- {
		select {
		case <-cancel:
			return nil
		case <-time.After(time.Second):
		}
	}
	return nil
}
