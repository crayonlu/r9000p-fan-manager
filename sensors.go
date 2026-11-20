package main

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Sensor is one temperature reading from a hwmon source.
type Sensor struct {
	Source string
	Label  string
	Temp   int // °C
}

var sensorSourceName = map[string]string{
	"legion_hwmon": "Legion",
	"k10temp":      "CPU",
	"nvidia":       "GPU",
	"nvme":         "NVMe",
	"spd5118":      "Memory",
	"mt7921_phy0":  "WiFi",
}

var sensorSourceOrder = map[string]int{
	"legion_hwmon": 0,
	"k10temp":      1,
	"nvidia":       2,
	"nvme":         3,
	"spd5118":      4,
	"mt7921_phy0":  5,
}

func sensorTitle(s Sensor) string {
	if n, ok := sensorSourceName[s.Source]; ok {
		return n
	}
	return s.Source
}

// readSensors enumerates all hwmon temperature inputs plus the NVIDIA GPU temp.
func readSensors() []Sensor {
	var out []Sensor
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, d := range dirs {
		name := readStr(filepath.Join(d, "name"))
		files, _ := filepath.Glob(filepath.Join(d, "temp*_input"))
		for _, f := range files {
			raw := readInt(f)
			if raw <= 0 {
				continue
			}
			label := readStr(strings.TrimSuffix(f, "_input") + "_label")
			if label == "" {
				label = strings.TrimSuffix(filepath.Base(f), "_input")
			}
			out = append(out, Sensor{Source: name, Label: label, Temp: raw / 1000})
		}
	}
	if b, err := exec.Command("nvidia-smi", "--query-gpu=temperature.gpu", "--format=csv,noheader,nounits").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if t, e := strconv.Atoi(strings.TrimSpace(line)); e == nil && t > 0 {
				out = append(out, Sensor{Source: "nvidia", Label: "GPU", Temp: t})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi, ok1 := sensorSourceOrder[out[i].Source]
		if !ok1 {
			oi = 9
		}
		oj, ok2 := sensorSourceOrder[out[j].Source]
		if !ok2 {
			oj = 9
		}
		if oi != oj {
			return oi < oj
		}
		return out[i].Temp > out[j].Temp
	})
	return out
}
