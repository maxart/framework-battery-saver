// Package power reads live power-related metrics from sysfs and procfs and
// exposes them as point-in-time snapshots plus short rolling histories for
// sparklines. Nothing here requires root; the privileged toggle lives in the
// battery-saver.sh script that the UI shells out to.
package power

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cpuBasePath  = "/sys/devices/system/cpu"
	supplyPath   = "/sys/class/power_supply"
	raplBasePath = "/sys/class/powercap/intel-rapl:0"

	historyLength = 60
)

// Snapshot is a single coherent reading of the system's power state.
type Snapshot struct {
	PowerW      float64
	PowerSource string // "battery", "cpu pkg", or "n/a"

	FreqMHz    float64 // average current frequency across online cores
	CapFreqMHz float64 // scaling_max_freq (the active cap)
	MaxFreqMHz float64 // cpuinfo_max_freq (hardware ceiling)
	Governor   string

	BatteryPct    int
	BatteryStatus string // Charging, Discharging, Full, ...
	OnAC          bool

	BatteryETA      time.Duration // time to empty (discharging) or full (charging)
	BatteryETAValid bool

	Profile string // power-profiles-daemon profile, or "unknown"
	SaverOn bool

	BrightnessPct     int
	BrightnessPresent bool
	Wifi              RadioState
	Bluetooth         RadioState

	PowerHistory []float64
	FreqHistory  []float64
}

type Scraper struct {
	mu          sync.RWMutex
	powerHist   *ring
	freqHist    *ring
	last        Snapshot
	lastEnergy  uint64
	lastEnergyT time.Time
	haveEnergy  bool
}

func NewScraper() *Scraper {
	return &Scraper{
		powerHist: newRing(historyLength),
		freqHist:  newRing(historyLength),
	}
}

// Snapshot returns the most recent reading. Safe to call concurrently.
func (s *Scraper) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.last
}

// Scrape collects a fresh reading and appends to the rolling histories.
func (s *Scraper) Scrape(ctx context.Context) {
	snap := Snapshot{}

	snap.FreqMHz, snap.CapFreqMHz, snap.MaxFreqMHz = s.readFrequencies()
	snap.Governor = readString(filepath.Join(cpuBasePath, "cpu0/cpufreq/scaling_governor"))
	snap.BatteryPct, snap.BatteryStatus = s.readBattery()
	snap.OnAC = s.readOnAC()
	snap.BatteryETA, snap.BatteryETAValid = s.readBatteryETA(snap.BatteryStatus)
	snap.PowerW, snap.PowerSource = s.readPower(snap.BatteryStatus)
	snap.Profile = readProfile(ctx)
	// The state file is the authoritative marker: on/off create and remove it.
	// We don't infer from profile or frequency cap because amd_pstate reports a
	// dynamic cpuinfo_max_freq, making cap detection unreliable.
	snap.SaverOn = SaverActive()
	snap.BrightnessPct, snap.BrightnessPresent = readBacklight()
	snap.Wifi, snap.Bluetooth = readRadios()

	s.mu.Lock()
	s.powerHist.push(snap.PowerW)
	s.freqHist.push(snap.FreqMHz)
	snap.PowerHistory = s.powerHist.values()
	snap.FreqHistory = s.freqHist.values()
	s.last = snap
	s.mu.Unlock()
}

// Private

func (s *Scraper) readFrequencies() (avgMHz, capMHz, maxMHz float64) {
	matches, _ := filepath.Glob(filepath.Join(cpuBasePath, "cpu[0-9]*/cpufreq/scaling_cur_freq"))
	var sum, n float64
	for _, path := range matches {
		if khz, ok := readUint(path); ok {
			sum += float64(khz)
			n++
		}
	}
	if n > 0 {
		avgMHz = sum / n / 1000
	}
	if khz, ok := readUint(filepath.Join(cpuBasePath, "cpu0/cpufreq/scaling_max_freq")); ok {
		capMHz = float64(khz) / 1000
	}
	if khz, ok := readUint(filepath.Join(cpuBasePath, "cpu0/cpufreq/cpuinfo_max_freq")); ok {
		maxMHz = float64(khz) / 1000
	}
	return
}

func (s *Scraper) readBattery() (pct int, status string) {
	bat := firstSupply("BAT*")
	if bat == "" {
		return 0, "unknown"
	}
	pct = int(readInt(filepath.Join(bat, "capacity")))
	status = readString(filepath.Join(bat, "status"))
	if status == "" {
		status = "unknown"
	}
	return
}

// readBatteryETA estimates time-to-empty (discharging) or time-to-full
// (charging) from the gauge's energy/power or charge/current pairs. Returns
// false when the battery is idle (Full) or the rate is unknown/implausible.
func (s *Scraper) readBatteryETA(status string) (time.Duration, bool) {
	bat := firstSupply("BAT*")
	if bat == "" {
		return 0, false
	}

	// Prefer energy (µWh) / power (µW); fall back to charge (µAh) / current (µA).
	// Both pairs divide to hours.
	var remaining, rate float64
	if power, ok := readUint(filepath.Join(bat, "power_now")); ok && power > 0 {
		rate = float64(power)
		switch status {
		case "Discharging":
			if e, ok := readUint(filepath.Join(bat, "energy_now")); ok {
				remaining = float64(e)
			}
		case "Charging":
			full, okF := readUint(filepath.Join(bat, "energy_full"))
			now, okN := readUint(filepath.Join(bat, "energy_now"))
			if okF && okN && full > now {
				remaining = float64(full - now)
			}
		}
	} else if current, ok := readUint(filepath.Join(bat, "current_now")); ok && current > 0 {
		rate = float64(current)
		switch status {
		case "Discharging":
			if c, ok := readUint(filepath.Join(bat, "charge_now")); ok {
				remaining = float64(c)
			}
		case "Charging":
			full, okF := readUint(filepath.Join(bat, "charge_full"))
			now, okN := readUint(filepath.Join(bat, "charge_now"))
			if okF && okN && full > now {
				remaining = float64(full - now)
			}
		}
	}

	if remaining <= 0 || rate <= 0 {
		return 0, false
	}
	hours := remaining / rate
	if hours <= 0 || hours > 48 { // implausible: treat as unknown
		return 0, false
	}
	return time.Duration(hours * float64(time.Hour)), true
}

func (s *Scraper) readOnAC() bool {
	matches, _ := filepath.Glob(filepath.Join(supplyPath, "*/online"))
	for _, path := range matches {
		if strings.Contains(path, "BAT") {
			continue
		}
		if readString(path) == "1" {
			return true
		}
	}
	return false
}

// readPower prefers the battery's instantaneous discharge power (full-system,
// root-free). When not discharging it tries the RAPL package counter, which is
// usually root-only and so degrades to "n/a".
func (s *Scraper) readPower(status string) (float64, string) {
	if status == "Discharging" {
		if w, ok := s.batteryWatts(); ok {
			return w, "battery"
		}
	}
	if w, ok := s.raplWatts(); ok {
		return w, "cpu pkg"
	}
	if w, ok := s.batteryWatts(); ok {
		return w, "battery"
	}
	return 0, "n/a"
}

func (s *Scraper) batteryWatts() (float64, bool) {
	bat := firstSupply("BAT*")
	if bat == "" {
		return 0, false
	}
	if uw, ok := readUint(filepath.Join(bat, "power_now")); ok && uw > 0 {
		return float64(uw) / 1e6, true
	}
	cur, okC := readUint(filepath.Join(bat, "current_now"))  // µA
	volt, okV := readUint(filepath.Join(bat, "voltage_now")) // µV
	if okC && okV && cur > 0 && volt > 0 {
		return float64(cur) * float64(volt) / 1e12, true
	}
	return 0, false
}

func (s *Scraper) raplWatts() (float64, bool) {
	uj, ok := readUint(filepath.Join(raplBasePath, "energy_uj"))
	if !ok {
		return 0, false
	}
	now := time.Now()
	defer func() {
		s.lastEnergy = uj
		s.lastEnergyT = now
		s.haveEnergy = true
	}()
	if !s.haveEnergy {
		return 0, false
	}
	dt := now.Sub(s.lastEnergyT).Seconds()
	if dt <= 0 || uj < s.lastEnergy { // counter wrap or no elapsed time
		return 0, false
	}
	return float64(uj-s.lastEnergy) / 1e6 / dt, true
}

// Helpers

func firstSupply(pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(supplyPath, pattern))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func readString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readUint(path string) (uint64, bool) {
	v, err := strconv.ParseUint(readString(path), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readInt(path string) int64 {
	v, err := strconv.ParseInt(readString(path), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
