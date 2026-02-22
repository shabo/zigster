package history

import (
	"testing"
	"time"
)

func TestHistory(t *testing.T) {
	h := NewBuffer(5)

	now := time.Now()
	for i := 0; i < 7; i++ {
		h.Push(float64(30+i), now.Add(time.Duration(i)*time.Second))
	}

	if len(h.Points) != 5 {
		t.Errorf("expected 5 points, got %d", len(h.Points))
	}

	if h.Last() != 36.0 {
		t.Errorf("Last(): got %f, want 36.0", h.Last())
	}

	if h.Min != 30.0 {
		t.Errorf("Min: got %f, want 30.0", h.Min)
	}

	if h.Peak != 36.0 {
		t.Errorf("Peak: got %f, want 36.0", h.Peak)
	}

	vals := h.LastN(3)
	if len(vals) != 3 {
		t.Errorf("LastN(3): got %d values, want 3", len(vals))
	}
}

func TestLastNPoints(t *testing.T) {
	h := NewBuffer(100)
	base := time.Date(2026, 2, 21, 14, 0, 0, 0, time.Local)

	for i := 0; i < 120; i++ {
		h.Push(float64(30+i%10), base.Add(time.Duration(i)*time.Second))
	}

	pts := h.LastNPoints(5)
	if len(pts) != 5 {
		t.Fatalf("LastNPoints(5): got %d, want 5", len(pts))
	}

	for _, p := range pts {
		if p.Time.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	}

	last := pts[len(pts)-1]
	if last.Time != base.Add(119*time.Second) {
		t.Errorf("last point time: got %v, want %v", last.Time, base.Add(119*time.Second))
	}
}
