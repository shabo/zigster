package main

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SensorReading represents a single temperature reading from a sensor.
type SensorReading struct {
	Chip    string  // e.g. "coretemp-isa-0000"
	Adapter string  // e.g. "ISA adapter"
	Label   string  // e.g. "Core 0"
	Temp    float64 // current temperature in Celsius
	High    float64 // high threshold (0 if not available)
	Crit    float64 // critical threshold (0 if not available)
	HasHigh bool
	HasCrit bool
}

// Key returns a unique identifier for this sensor.
func (s SensorReading) Key() string {
	return s.Chip + "/" + s.Label
}

// ReadSensors dynamically discovers all available temperature sensors by
// combining: (1) `sensors -j` JSON output, (2) nvidia-smi, (3) drive temps.
// New sensors appearing at runtime are picked up automatically.
func ReadSensors() ([]SensorReading, error) {
	readings, err := readSensorsJSON()
	if err != nil {
		// Fallback to text parsing if JSON fails (older lm-sensors)
		readings, err = readSensorsText()
		if err != nil {
			return nil, err
		}
	}

	// Merge NVIDIA GPU temps
	readings = append(readings, ReadNvidiaGPU()...)

	// Merge drive temps (drivetemp hwmon + smartctl)
	readings = append(readings, ReadDriveTemps()...)

	return readings, nil
}

// ── JSON parser (primary) ────────────────────────────────────────────

// readSensorsJSON parses `sensors -j` for fully dynamic sensor discovery.
func readSensorsJSON() ([]SensorReading, error) {
	out, err := exec.Command("sensors", "-j").Output()
	if err != nil {
		return nil, err
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}

	var readings []SensorReading

	// Sort chip names for deterministic ordering
	chipNames := make([]string, 0, len(data))
	for k := range data {
		chipNames = append(chipNames, k)
	}
	sort.Strings(chipNames)

	for _, chipName := range chipNames {
		chipRaw := data[chipName]
		var chip map[string]json.RawMessage
		if err := json.Unmarshal(chipRaw, &chip); err != nil {
			continue
		}

		adapter := ""
		if raw, ok := chip["Adapter"]; ok {
			json.Unmarshal(raw, &adapter)
		}

		// Sort sensor labels for deterministic ordering
		labels := make([]string, 0, len(chip))
		for k := range chip {
			if k != "Adapter" {
				labels = append(labels, k)
			}
		}
		sort.Strings(labels)

		for _, label := range labels {
			sensorRaw := chip[label]

			var fields map[string]float64
			if err := json.Unmarshal(sensorRaw, &fields); err != nil {
				continue
			}

			// Find the temp_input field
			var temp float64
			var foundTemp bool
			for k, v := range fields {
				if strings.HasSuffix(k, "_input") && strings.Contains(k, "temp") {
					temp = v
					foundTemp = true
					break
				}
			}
			if !foundTemp || temp < -200 {
				continue
			}

			r := SensorReading{
				Chip:    chipName,
				Adapter: adapter,
				Label:   label,
				Temp:    temp,
			}

			for k, v := range fields {
				if strings.HasSuffix(k, "_max") && v > 0 && v < 1000 {
					r.High = v
					r.HasHigh = true
				}
				if strings.HasSuffix(k, "_crit") && v > 0 && v < 1000 {
					r.Crit = v
					r.HasCrit = true
				}
			}

			readings = append(readings, r)
		}
	}

	return readings, nil
}

// ── Text parser (fallback) ───────────────────────────────────────────

func readSensorsText() ([]SensorReading, error) {
	out, err := exec.Command("sensors").Output()
	if err != nil {
		return nil, err
	}
	return ParseSensorsText(string(out)), nil
}

var (
	adapterRe  = regexp.MustCompile(`^Adapter:\s+(.+)$`)
	namedValRe = regexp.MustCompile(`(\w+)\s*=\s*([+-]?\d+\.?\d*)°C`)
	tempValRe  = regexp.MustCompile(`[+-]?(\d+\.?\d*)°C`)
)

// ParseSensorsText parses the human-readable `sensors` output.
func ParseSensorsText(output string) []SensorReading {
	var readings []SensorReading
	var currentChip, currentAdapter string

	lines := strings.Split(output, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")

		if strings.TrimSpace(line) == "" {
			continue
		}

		if m := adapterRe.FindStringSubmatch(line); m != nil {
			currentAdapter = m[1]
			continue
		}

		if strings.Contains(line, "°C") {
			idx := strings.Index(line, ":")
			if idx < 0 {
				continue
			}
			label := strings.TrimSpace(line[:idx])

			// Extract the first temperature value after the colon
			rest := line[idx+1:]
			m := tempValRe.FindStringSubmatch(rest)
			if m == nil {
				continue
			}
			temp, err := strconv.ParseFloat(m[1], 64)
			if err != nil || temp < -200 {
				continue
			}
			// Check for negative sign
			fullMatch := tempValRe.FindString(rest)
			if strings.HasPrefix(strings.TrimSpace(fullMatch), "-") {
				temp = -temp
			}

			r := SensorReading{
				Chip:    currentChip,
				Adapter: currentAdapter,
				Label:   label,
				Temp:    temp,
			}

			if high := extractNamedVal(line, "high"); high > 0 && high < 1000 {
				r.High = high
				r.HasHigh = true
			}
			if crit := extractNamedVal(line, "crit"); crit > 0 && crit < 1000 {
				r.Crit = crit
				r.HasCrit = true
			}
			if i+1 < len(lines) {
				next := strings.TrimRight(lines[i+1], "\r")
				if strings.Contains(next, "crit") && !strings.Contains(next, ":") {
					if crit := extractNamedVal(next, "crit"); crit > 0 && crit < 1000 {
						r.Crit = crit
						r.HasCrit = true
					}
				}
			}

			readings = append(readings, r)
			continue
		}

		// Chip header — non-indented line without °C
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			currentChip = strings.TrimSpace(line)
		}
	}

	return readings
}

func extractNamedVal(line, name string) float64 {
	matches := namedValRe.FindAllStringSubmatch(line, -1)
	for _, m := range matches {
		if m[1] == name {
			v, err := strconv.ParseFloat(m[2], 64)
			if err == nil && v > -200 {
				return v
			}
		}
	}
	return 0
}

// ── Chip identity ────────────────────────────────────────────────────

var chipIdentityMap = []struct {
	prefix string
	name   string
}{
	{"coretemp", "CPU"},
	{"k10temp", "CPU"},
	{"zenpower", "CPU"},
	{"amdgpu", "GPU (AMD)"},
	{"radeon", "GPU (AMD)"},
	{"nouveau", "GPU (NVIDIA)"},
	{"nvidia-gpu", "GPU (NVIDIA)"},
	{"nvidia", "GPU (NVIDIA)"},
	{"intel_gpu", "GPU (Intel)"},
	{"i915", "GPU (Intel)"},
	{"nvme", "NVMe SSD"},
	{"drivetemp", "HDD/SSD"},
	{"smart-", "HDD/SSD"},
	{"iwlwifi", "WiFi"},
	{"ath", "WiFi"},
	{"mt7", "WiFi"},
	{"rtw", "WiFi"},
	{"pch", "PCH (Chipset)"},
	{"acpi", "ACPI Thermal"},
	{"it87", "Motherboard"},
	{"nct", "Motherboard"},
	{"w83", "Motherboard"},
	{"f71", "Motherboard"},
	{"asus", "Motherboard"},
	{"thinkpad", "Laptop EC"},
	{"dell", "Laptop EC"},
	{"hp", "Laptop EC"},
	{"bat", "Battery"},
}

// ChipFriendlyName returns a human-readable component name for a chip ID.
func ChipFriendlyName(chip string) string {
	lower := strings.ToLower(chip)
	for _, entry := range chipIdentityMap {
		if strings.HasPrefix(lower, entry.prefix) {
			return entry.name
		}
	}
	return "Sensor"
}
