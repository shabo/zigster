// Package history provides a ring-buffer based temperature history
// tracker with per-sensor min/peak/avg statistics.
package history

import (
	"math"
	"time"
)

// Point is a single data point in the temperature history.
type Point struct {
	Temp float64
	Time time.Time
}

// Buffer stores a ring buffer of temperature readings for one sensor.
type Buffer struct {
	Points []Point
	Max    int // capacity
	Min    float64
	Peak   float64
}

// NewBuffer creates a new history ring buffer with the given capacity.
func NewBuffer(capacity int) *Buffer {
	return &Buffer{
		Points: make([]Point, 0, capacity),
		Max:    capacity,
		Min:    math.MaxFloat64,
		Peak:   -math.MaxFloat64,
	}
}

// Push adds a new temperature reading to the history.
func (b *Buffer) Push(temp float64, t time.Time) {
	p := Point{Temp: temp, Time: t}
	if len(b.Points) >= b.Max {
		copy(b.Points, b.Points[1:])
		b.Points[len(b.Points)-1] = p
	} else {
		b.Points = append(b.Points, p)
	}

	if temp < b.Min {
		b.Min = temp
	}
	if temp > b.Peak {
		b.Peak = temp
	}
}

// Last returns the most recent temperature, or 0 if empty.
func (b *Buffer) Last() float64 {
	if len(b.Points) == 0 {
		return 0
	}
	return b.Points[len(b.Points)-1].Temp
}

// Avg returns the average temperature across all stored points.
func (b *Buffer) Avg() float64 {
	if len(b.Points) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range b.Points {
		sum += p.Temp
	}
	return sum / float64(len(b.Points))
}

// LastN returns the last n temperature values (for chart rendering).
func (b *Buffer) LastN(n int) []float64 {
	if n <= 0 || len(b.Points) == 0 {
		return nil
	}
	start := len(b.Points) - n
	if start < 0 {
		start = 0
	}
	vals := make([]float64, 0, n)
	for _, p := range b.Points[start:] {
		vals = append(vals, p.Temp)
	}
	return vals
}

// LastNPoints returns the last n Points (with timestamps).
func (b *Buffer) LastNPoints(n int) []Point {
	if n <= 0 || len(b.Points) == 0 {
		return nil
	}
	start := len(b.Points) - n
	if start < 0 {
		start = 0
	}
	out := make([]Point, len(b.Points[start:]))
	copy(out, b.Points[start:])
	return out
}

// Store manages histories for all sensors.
type Store struct {
	Data     map[string]*Buffer
	Capacity int
}

// NewStore creates a new store with the given per-sensor capacity.
func NewStore(capacity int) *Store {
	return &Store{
		Data:     make(map[string]*Buffer),
		Capacity: capacity,
	}
}

// Record adds a reading for the given sensor key.
func (s *Store) Record(key string, temp float64, t time.Time) {
	b, ok := s.Data[key]
	if !ok {
		b = NewBuffer(s.Capacity)
		s.Data[key] = b
	}
	b.Push(temp, t)
}

// Get returns the history buffer for a sensor key, or nil.
func (s *Store) Get(key string) *Buffer {
	return s.Data[key]
}
