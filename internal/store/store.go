// Package store handles persistent CSV storage of temperature readings
// with daily file rotation. Data is stored in ~/.sensors-data/.
package store

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/luki/sensors/internal/sensor"
)

const (
	dirName    = ".sensors-data"
	timeLayout = "2006-01-02T15:04:05"
	fileLayout = "2006-01-02"
)

// DiskStore handles persistent CSV storage of temperature readings.
// Files are stored as ~/.sensors-data/YYYY-MM-DD.csv with the format:
//
//	timestamp,chip,label,temp,high,crit
type DiskStore struct {
	dir     string
	current *os.File
	writer  *csv.Writer
	curDate string
}

// StoredReading is a single row from a CSV log file.
type StoredReading struct {
	Time  time.Time
	Chip  string
	Label string
	Temp  float64
	High  float64
	Crit  float64
}

// New creates a new disk store, creating the data directory if needed.
func New() (*DiskStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot find home dir: %w", err)
	}
	dir := filepath.Join(home, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create data dir: %w", err)
	}
	return &DiskStore{dir: dir}, nil
}

// Write appends a batch of sensor readings to today's CSV file.
func (d *DiskStore) Write(readings []sensor.Reading, t time.Time) error {
	dateStr := t.Format(fileLayout)

	if d.curDate != dateStr || d.current == nil {
		d.Close()
		path := filepath.Join(d.dir, dateStr+".csv")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		d.current = f
		d.writer = csv.NewWriter(f)
		d.curDate = dateStr

		info, _ := f.Stat()
		if info.Size() == 0 {
			d.writer.Write([]string{"time", "chip", "label", "temp", "high", "crit"})
		}
	}

	ts := t.Format(timeLayout)
	for _, r := range readings {
		d.writer.Write([]string{
			ts,
			r.Chip,
			r.Label,
			fmt.Sprintf("%.1f", r.Temp),
			fmt.Sprintf("%.1f", r.High),
			fmt.Sprintf("%.1f", r.Crit),
		})
	}
	d.writer.Flush()
	return d.writer.Error()
}

// Close flushes and closes the current file.
func (d *DiskStore) Close() {
	if d.writer != nil {
		d.writer.Flush()
	}
	if d.current != nil {
		d.current.Close()
		d.current = nil
	}
}

// ListDays returns available log dates (newest first).
func ListDays(dir string) ([]string, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, dirName)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var days []string
	for i := len(entries) - 1; i >= 0; i-- {
		name := entries[i].Name()
		if strings.HasSuffix(name, ".csv") {
			days = append(days, strings.TrimSuffix(name, ".csv"))
		}
	}
	return days, nil
}

// LoadDay reads all readings from a specific day's CSV file.
func LoadDay(day string) ([]StoredReading, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, dirName, day+".csv")
	return LoadFile(path)
}

// LoadFile reads all readings from a CSV file.
func LoadFile(path string) ([]StoredReading, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var readings []StoredReading
	for i, row := range records {
		if i == 0 && len(row) > 0 && row[0] == "time" {
			continue
		}
		if len(row) < 6 {
			continue
		}

		t, err := time.ParseInLocation(timeLayout, row[0], time.Local)
		if err != nil {
			continue
		}
		temp, _ := strconv.ParseFloat(row[3], 64)
		high, _ := strconv.ParseFloat(row[4], 64)
		crit, _ := strconv.ParseFloat(row[5], 64)

		readings = append(readings, StoredReading{
			Time:  t,
			Chip:  row[1],
			Label: row[2],
			Temp:  temp,
			High:  high,
			Crit:  crit,
		})
	}

	return readings, nil
}

// DataDir returns the path to the data directory.
func DataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, dirName)
}
