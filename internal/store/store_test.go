package store

import (
	"testing"
	"time"

	"github.com/luki/sensors/internal/sensor"
)

func TestDiskStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()

	ds := &DiskStore{dir: dir}
	defer ds.Close()

	now := time.Date(2026, 2, 21, 14, 30, 0, 0, time.Local)
	readings := []sensor.Reading{
		{Chip: "coretemp-isa-0000", Label: "Core 0", Temp: 45.0, High: 101.0, Crit: 115.0, HasHigh: true, HasCrit: true},
		{Chip: "nvme-pci-0300", Label: "Composite", Temp: 36.9, High: 81.8, Crit: 84.8, HasHigh: true, HasCrit: true},
	}

	if err := ds.Write(readings, now); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ds.Close()

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
