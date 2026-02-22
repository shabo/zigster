package sensor

import (
	"testing"
)

const testSensorOutput = `iwlwifi_1-virtual-0
Adapter: Virtual device
temp1:        +35.0°C  

nvme-pci-0300
Adapter: PCI adapter
Composite:    +36.9°C  (low  = -273.1°C, high = +81.8°C)
                       (crit = +84.8°C)
Sensor 1:     +36.9°C  (low  = -273.1°C, high = +65261.8°C)
Sensor 2:     +49.9°C  (low  = -273.1°C, high = +65261.8°C)

coretemp-isa-0000
Adapter: ISA adapter
Package id 0:  +48.0°C  (high = +101.0°C, crit = +115.0°C)
Core 0:        +46.0°C  (high = +101.0°C, crit = +115.0°C)
Core 1:        +45.0°C  (high = +101.0°C, crit = +115.0°C)
Core 2:        +46.0°C  (high = +101.0°C, crit = +115.0°C)
Core 3:        +48.0°C  (high = +101.0°C, crit = +115.0°C)
Core 4:        +46.0°C  (high = +101.0°C, crit = +115.0°C)
Core 5:        +47.0°C  (high = +101.0°C, crit = +115.0°C)
Core 6:        +44.0°C  (high = +101.0°C, crit = +115.0°C)
Core 7:        +45.0°C  (high = +101.0°C, crit = +115.0°C)

pch_cannonlake-virtual-0
Adapter: Virtual device
temp1:        +39.0°C  
`

func TestParseSensors(t *testing.T) {
	readings := ParseSensorsText(testSensorOutput)

	if len(readings) == 0 {
		t.Fatal("expected readings, got none")
	}

	if len(readings) < 10 {
		t.Errorf("expected at least 10 readings, got %d", len(readings))
	}

	var found bool
	for _, r := range readings {
		if r.Label == "Core 0" {
			found = true
			if r.Temp != 46.0 {
				t.Errorf("Core 0 temp: got %f, want 46.0", r.Temp)
			}
			if !r.HasHigh || r.High != 101.0 {
				t.Errorf("Core 0 high: got %f (has=%v), want 101.0", r.High, r.HasHigh)
			}
			if !r.HasCrit || r.Crit != 115.0 {
				t.Errorf("Core 0 crit: got %f (has=%v), want 115.0", r.Crit, r.HasCrit)
			}
			if r.Chip != "coretemp-isa-0000" {
				t.Errorf("Core 0 chip: got %q, want coretemp-isa-0000", r.Chip)
			}
			break
		}
	}
	if !found {
		t.Error("did not find Core 0 reading")
	}

	for _, r := range readings {
		if r.Label == "Composite" {
			if !r.HasHigh || r.High != 81.8 {
				t.Errorf("NVMe Composite high: got %f (has=%v), want 81.8", r.High, r.HasHigh)
			}
			if !r.HasCrit || r.Crit != 84.8 {
				t.Errorf("NVMe Composite crit: got %f (has=%v), want 84.8", r.Crit, r.HasCrit)
			}
			break
		}
	}

	for _, r := range readings {
		t.Logf("%-30s %-20s %6.1f°C  high=%.1f(has=%v) crit=%.1f(has=%v)",
			r.Chip, r.Label, r.Temp, r.High, r.HasHigh, r.Crit, r.HasCrit)
	}
}

func TestFriendlyName(t *testing.T) {
	tests := []struct {
		chip string
		want string
	}{
		{"coretemp-isa-0000", "CPU"},
		{"nvme-pci-0300", "NVMe SSD"},
		{"iwlwifi_1-virtual-0", "WiFi"},
		{"pch_cannonlake-virtual-0", "PCH (Chipset)"},
		{"amdgpu-pci-0600", "GPU (AMD)"},
		{"nvidia-gpu-0", "GPU (NVIDIA)"},
		{"smart-sda", "HDD/SSD"},
		{"drivetemp-hwmon4", "HDD/SSD"},
		{"some-unknown-chip", "Sensor"},
	}
	for _, tt := range tests {
		got := FriendlyName(tt.chip)
		if got != tt.want {
			t.Errorf("FriendlyName(%q) = %q, want %q", tt.chip, got, tt.want)
		}
	}
}
