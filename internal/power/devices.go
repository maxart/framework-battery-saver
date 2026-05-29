package power

import (
	"path/filepath"
)

const (
	backlightPath = "/sys/class/backlight"
	rfkillPath    = "/sys/class/rfkill"
)

// RadioState describes a wireless radio (Wi-Fi or Bluetooth) as seen via
// rfkill. Off is true when the radio is soft- or hard-blocked; HardBlock means
// a physical/firmware switch is off and software can't re-enable it.
type RadioState struct {
	Present   bool
	Off       bool
	HardBlock bool
}

// readBacklight returns the first backlight's level as a percentage. Reading is
// root-free; changing it is done via brightnessctl elsewhere.
func readBacklight() (pct int, present bool) {
	dirs, _ := filepath.Glob(filepath.Join(backlightPath, "*"))
	for _, d := range dirs {
		cur, ok1 := readUint(filepath.Join(d, "brightness"))
		max, ok2 := readUint(filepath.Join(d, "max_brightness"))
		if ok1 && ok2 && max > 0 {
			return int((cur*100 + max/2) / max), true
		}
	}
	return 0, false
}

// readRadios reports Wi-Fi and Bluetooth state from sysfs (root-free). The
// rfkill "type" is "wlan" for Wi-Fi and "bluetooth" for Bluetooth.
func readRadios() (wifi, bluetooth RadioState) {
	dirs, _ := filepath.Glob(filepath.Join(rfkillPath, "rfkill*"))
	for _, d := range dirs {
		soft := readString(filepath.Join(d, "soft")) == "1"
		hard := readString(filepath.Join(d, "hard")) == "1"
		st := RadioState{Present: true, Off: soft || hard, HardBlock: hard}
		switch readString(filepath.Join(d, "type")) {
		case "wlan":
			wifi = st
		case "bluetooth":
			bluetooth = st
		}
	}
	return
}
