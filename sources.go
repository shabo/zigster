package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ReadNvidiaGPU reads GPU temperatures via nvidia-smi.
// Returns nil (no error) if nvidia-smi is not available — we just skip it.
func ReadNvidiaGPU() []SensorReading {
	// Quick check if nvidia-smi exists
	path, err := exec.LookPath("nvidia-smi")
	if err != nil || path == "" {
		return nil
	}

	// Get basic temp per GPU
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=index,name,temperature.gpu",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return nil
	}

	// Get thresholds from the verbose query
	thresholds := parseNvidiaThresholds()

	var readings []SensorReading
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ", ", 3)
		if len(parts) < 3 {
			continue
		}

		idx := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		temp, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err != nil {
			continue
		}

		chipName := fmt.Sprintf("nvidia-gpu-%s", idx)

		r := SensorReading{
			Chip:    chipName,
			Adapter: name,
			Label:   "GPU Temp",
			Temp:    temp,
		}

		// Apply thresholds if we got them
		if t, ok := thresholds["slowdown"]; ok {
			r.High = t
			r.HasHigh = true
		}
		if t, ok := thresholds["shutdown"]; ok {
			r.Crit = t
			r.HasCrit = true
		}

		readings = append(readings, r)
	}

	return readings
}

var nvidiaTempRe = regexp.MustCompile(`GPU (\w[\w .]+\w)\s*:\s*(\d+)\s*C`)

// parseNvidiaThresholds parses nvidia-smi -q -d TEMPERATURE for threshold values.
func parseNvidiaThresholds() map[string]float64 {
	out, err := exec.Command("nvidia-smi", "-q", "-d", "TEMPERATURE").Output()
	if err != nil {
		return nil
	}

	result := make(map[string]float64)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "GPU Shutdown Temp") {
			if v := extractNvidiaTemp(line); v > 0 {
				result["shutdown"] = v
			}
		} else if strings.HasPrefix(line, "GPU Slowdown Temp") {
			if v := extractNvidiaTemp(line); v > 0 {
				result["slowdown"] = v
			}
		} else if strings.HasPrefix(line, "GPU Max Operating Temp") {
			if v := extractNvidiaTemp(line); v > 0 {
				result["max_operating"] = v
			}
		}
	}
	return result
}

var nvidiaTempValRe = regexp.MustCompile(`:\s*(\d+)\s*C`)

func extractNvidiaTemp(line string) float64 {
	m := nvidiaTempValRe.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	return v
}

// ReadDriveTemps reads HDD/SSD temperatures via the drivetemp kernel module
// (sysfs hwmon) or falls back to smartctl for SATA drives not exposed via hwmon.
func ReadDriveTemps() []SensorReading {
	var readings []SensorReading

	// First: check if drivetemp module exposes anything via hwmon
	readings = append(readings, readDrivetempHwmon()...)

	// Second: try smartctl for any /dev/sd* drives not already covered
	readings = append(readings, readSmartctlDrives()...)

	return readings
}

// readDrivetempHwmon checks /sys/class/hwmon for drivetemp entries.
func readDrivetempHwmon() []SensorReading {
	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/name")
	var readings []SensorReading

	for _, namePath := range matches {
		dir := filepath.Dir(namePath)
		nameBytes, err := readFileContent(namePath)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(nameBytes))
		if name != "drivetemp" {
			continue
		}

		tempPath := filepath.Join(dir, "temp1_input")
		tempBytes, err := readFileContent(tempPath)
		if err != nil {
			continue
		}
		tempMilliC, err := strconv.ParseFloat(strings.TrimSpace(string(tempBytes)), 64)
		if err != nil {
			continue
		}
		temp := tempMilliC / 1000.0

		readings = append(readings, SensorReading{
			Chip:    "drivetemp-" + filepath.Base(dir),
			Adapter: "SATA drive",
			Label:   "Drive Temp",
			Temp:    temp,
		})
	}
	return readings
}

// readSmartctlDrives tries smartctl on /dev/sd* SATA drives.
func readSmartctlDrives() []SensorReading {
	// Check if smartctl is available
	path, err := exec.LookPath("smartctl")
	if err != nil || path == "" {
		return nil
	}

	drives, _ := filepath.Glob("/dev/sd?")
	var readings []SensorReading

	for _, dev := range drives {
		// Try to get temp via smartctl (needs root, may fail)
		out, err := exec.Command("sudo", "-n", "smartctl", "-A", dev).Output()
		if err != nil {
			// Try without sudo
			out, err = exec.Command("smartctl", "-A", dev).Output()
			if err != nil {
				continue
			}
		}

		temp, ok := parseSmartTemp(string(out))
		if !ok {
			continue
		}

		// Get model name
		model := getSmartModel(dev)
		if model == "" {
			model = "SATA drive"
		}

		devName := filepath.Base(dev)
		readings = append(readings, SensorReading{
			Chip:    "smart-" + devName,
			Adapter: model,
			Label:   "Drive Temp",
			Temp:    temp,
			High:    55, // typical HDD warning
			HasHigh: true,
			Crit:    60, // typical HDD critical
			HasCrit: true,
		})
	}

	return readings
}

var smartTempRe = regexp.MustCompile(`(?:194\s+Temperature_Celsius|190\s+Airflow_Temperature_Cel)\s+\S+\s+(\d+)`)

func parseSmartTemp(output string) (float64, bool) {
	// Prefer attribute 194 (Temperature_Celsius)
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "194") && strings.Contains(line, "Temperature") {
			m := smartTempRe.FindStringSubmatch(line)
			if m != nil {
				v, err := strconv.ParseFloat(m[1], 64)
				if err == nil {
					return v, true
				}
			}
		}
	}
	// Fallback to 190 (Airflow)
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "190") && strings.Contains(line, "Temperature") {
			m := smartTempRe.FindStringSubmatch(line)
			if m != nil {
				v, err := strconv.ParseFloat(m[1], 64)
				if err == nil {
					return v, true
				}
			}
		}
	}
	return 0, false
}

func getSmartModel(dev string) string {
	out, err := exec.Command("sudo", "-n", "smartctl", "-i", dev).Output()
	if err != nil {
		out, err = exec.Command("smartctl", "-i", dev).Output()
		if err != nil {
			return ""
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Device Model:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Device Model:"))
		}
		if strings.HasPrefix(line, "Model Number:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Model Number:"))
		}
	}
	return ""
}

// readFileContent reads the contents of a small sysfs file.
func readFileContent(path string) ([]byte, error) {
	out, err := exec.Command("cat", path).Output()
	return out, err
}
