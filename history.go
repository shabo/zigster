package main

import (
	"math"
	"time"
)

// HistoryPoint is a single data point in the temperature history.
type HistoryPoint struct {
	Temp float64
	Time time.Time
}

// History stores a ring buffer of temperature readings for one sensor.
type History struct {
	Points []HistoryPoint
	Max    int // capacity
	Min    float64
	Peak   float64
}

// NewHistory creates a new history ring buffer with the given capacity.
func NewHistory(capacity int) *History {
	return &History{
		Points: make([]HistoryPoint, 0, capacity),
		Max:    capacity,
		Min:    math.MaxFloat64,
		Peak:   -math.MaxFloat64,
	}
}

// Push adds a new temperature reading to the history.
func (h *History) Push(temp float64, t time.Time) {
	p := HistoryPoint{Temp: temp, Time: t}
	if len(h.Points) >= h.Max {
		copy(h.Points, h.Points[1:])
		h.Points[len(h.Points)-1] = p
	} else {
		h.Points = append(h.Points, p)
	}

	if temp < h.Min {
		h.Min = temp
	}
	if temp > h.Peak {
		h.Peak = temp
	}
}

// Last returns the most recent temperature, or 0 if empty.
func (h *History) Last() float64 {
	if len(h.Points) == 0 {
		return 0
	}
	return h.Points[len(h.Points)-1].Temp
}

// Avg returns the average temperature across all stored points.
func (h *History) Avg() float64 {
	if len(h.Points) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range h.Points {
		sum += p.Temp
	}
	return sum / float64(len(h.Points))
}

// LastN returns the last n temperature values (for chart rendering).
func (h *History) LastN(n int) []float64 {
	if n <= 0 || len(h.Points) == 0 {
		return nil
	}
	start := len(h.Points) - n
	if start < 0 {
		start = 0
	}
	vals := make([]float64, 0, n)
	for _, p := range h.Points[start:] {
		vals = append(vals, p.Temp)
	}
	return vals
}

// LastNPoints returns the last n HistoryPoints (with timestamps).
func (h *History) LastNPoints(n int) []HistoryPoint {
	if n <= 0 || len(h.Points) == 0 {
		return nil
	}
	start := len(h.Points) - n
	if start < 0 {
		start = 0
	}
	out := make([]HistoryPoint, len(h.Points[start:]))
	copy(out, h.Points[start:])
	return out
}

// HistoryStore manages histories for all sensors.
type HistoryStore struct {
	Data     map[string]*History
	Capacity int
}

// NewHistoryStore creates a new store with the given per-sensor capacity.
func NewHistoryStore(capacity int) *HistoryStore {
	return &HistoryStore{
		Data:     make(map[string]*History),
		Capacity: capacity,
	}
}

// Record adds a reading for the given sensor key.
func (s *HistoryStore) Record(key string, temp float64, t time.Time) {
	h, ok := s.Data[key]
	if !ok {
		h = NewHistory(s.Capacity)
		s.Data[key] = h
	}
	h.Push(temp, t)
}

// Get returns the history for a sensor key, or nil.
func (s *HistoryStore) Get(key string) *History {
	return s.Data[key]
}
