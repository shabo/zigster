package chart

import (
	"strings"
	"testing"
	"time"

	"github.com/luki/sensors/internal/history"
)

func TestSparkline(t *testing.T) {
	values := []float64{30, 35, 40, 50, 60, 70, 80, 90, 100}
	result := RenderSparkline(values, 20, 20, 110, 80, 100, true, true)
	if len(result) == 0 {
		t.Error("sparkline should not be empty")
	}
	t.Logf("Sparkline: %s", result)
}

func TestSparklineMinuteTicks(t *testing.T) {
	base := time.Date(2026, 2, 21, 14, 0, 50, 0, time.Local)
	var pts []history.Point
	for i := 0; i < 20; i++ {
		pts = append(pts, history.Point{
			Temp: float64(40 + i%5),
			Time: base.Add(time.Duration(i) * time.Second),
		})
	}

	result := RenderSparklinePoints(pts, 20, 30, 55, 80, 100, true, true)
	if len(result) == 0 {
		t.Error("sparkline should not be empty")
	}
	if !strings.Contains(result, "\u2502") {
		t.Error("expected minute tick mark in sparkline")
	}
	t.Logf("Sparkline with ticks: %s", result)
}
