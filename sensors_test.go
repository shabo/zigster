package main

import (
	"strings"
	"testing"
	"time"
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

	// Check we got the expected number (wifi + nvme composite/s1/s2 + package + 8 cores + pch = 13)
	if len(readings) < 10 {
		t.Errorf("expected at least 10 readings, got %d", len(readings))
	}

	// Find a core reading and verify thresholds
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

	// Check NVMe composite has high threshold
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

	// Print all readings for debugging
	for _, r := range readings {
		t.Logf("%-30s %-20s %6.1f°C  high=%.1f(has=%v) crit=%.1f(has=%v)",
			r.Chip, r.Label, r.Temp, r.High, r.HasHigh, r.Crit, r.HasCrit)
	}
}

func TestHistory(t *testing.T) {
	h := NewHistory(5)

	now := time.Now()
	for i := 0; i < 7; i++ {
		h.Push(float64(30+i), now.Add(time.Duration(i)*time.Second))
	}

	// Capacity is 5, so we should have the last 5
	if len(h.Points) != 5 {
		t.Errorf("expected 5 points, got %d", len(h.Points))
	}

	// Last should be 36 (30+6)
	if h.Last() != 36.0 {
		t.Errorf("Last(): got %f, want 36.0", h.Last())
	}

	// Min should be 30 (seen in the full history)
	if h.Min != 30.0 {
		t.Errorf("Min: got %f, want 30.0", h.Min)
	}

	// Peak should be 36
	if h.Peak != 36.0 {
		t.Errorf("Peak: got %f, want 36.0", h.Peak)
	}

	// LastN
	vals := h.LastN(3)
	if len(vals) != 3 {
		t.Errorf("LastN(3): got %d values, want 3", len(vals))
	}
}

func TestSparkline(t *testing.T) {
	values := []float64{30, 35, 40, 50, 60, 70, 80, 90, 100}
	result := RenderSparkline(values, 20, 20, 110, 80, 100, true, true)
	if len(result) == 0 {
		t.Error("sparkline should not be empty")
	}
	t.Logf("Sparkline: %s", result)
}

func TestLastNPoints(t *testing.T) {
	h := NewHistory(100)
	base := time.Date(2026, 2, 21, 14, 0, 0, 0, time.Local)

	for i := 0; i < 120; i++ {
		h.Push(float64(30+i%10), base.Add(time.Duration(i)*time.Second))
	}

	pts := h.LastNPoints(5)
	if len(pts) != 5 {
		t.Fatalf("LastNPoints(5): got %d, want 5", len(pts))
	}

	// Should have timestamps
	for _, p := range pts {
		if p.Time.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	}

	// Last point should be the most recent (we pushed 120 into cap 100, so last is index 119)
	last := pts[len(pts)-1]
	if last.Time != base.Add(119*time.Second) {
		t.Errorf("last point time: got %v, want %v", last.Time, base.Add(119*time.Second))
	}
}

func TestSparklineMinuteTicks(t *testing.T) {
	// Create points that cross a minute boundary
	base := time.Date(2026, 2, 21, 14, 0, 50, 0, time.Local)
	var pts []HistoryPoint
	for i := 0; i < 20; i++ {
		pts = append(pts, HistoryPoint{
			Temp: float64(40 + i%5),
			Time: base.Add(time.Duration(i) * time.Second),
		})
	}

	result := RenderSparklinePoints(pts, 20, 30, 55, 80, 100, true, true)
	if len(result) == 0 {
		t.Error("sparkline should not be empty")
	}
	// Should contain a tick mark (│) at the minute boundary (second 10 = 14:01:00)
	if !strings.Contains(result, "│") {
		t.Error("expected minute tick mark in sparkline")
	}
	t.Logf("Sparkline with ticks: %s", result)
}

func TestDiskStoreRoundTrip(t *testing.T) {
	// Create a temp dir for testing
	dir := t.TempDir()

	store := &DiskStore{dir: dir}
	defer store.Close()

	now := time.Date(2026, 2, 21, 14, 30, 0, 0, time.Local)
	readings := []SensorReading{
		{Chip: "coretemp-isa-0000", Label: "Core 0", Temp: 45.0, High: 101.0, Crit: 115.0, HasHigh: true, HasCrit: true},
		{Chip: "nvme-pci-0300", Label: "Composite", Temp: 36.9, High: 81.8, Crit: 84.8, HasHigh: true, HasCrit: true},
	}

	if err := store.Write(readings, now); err != nil {
		t.Fatalf("Write: %v", err)
	}
	store.Close()

	// Read back
	loaded, err := LoadFile(dir + "/2026-02-21.csv")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 readings, got %d", len(loaded))
	}

	if loaded[0].Chip != "coretemp-isa-0000" || loaded[0].Temp != 45.0 {
		t.Errorf("first reading: got %+v", loaded[0])
	}
	if loaded[1].Chip != "nvme-pci-0300" || loaded[1].Temp != 36.9 {
		t.Errorf("second reading: got %+v", loaded[1])
	}
}

func TestChipFriendlyName(t *testing.T) {
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
		got := ChipFriendlyName(tt.chip)
		if got != tt.want {
			t.Errorf("ChipFriendlyName(%q) = %q, want %q", tt.chip, got, tt.want)
		}
	}
}
