package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// tempColor returns the appropriate color for a temperature value given thresholds.
func tempColor(v, high, crit float64, hasHigh, hasCrit bool) lipgloss.Color {
	switch {
	case hasCrit && v >= crit:
		return lipgloss.Color("196") // red
	case hasHigh && v >= high:
		return lipgloss.Color("208") // orange
	case hasHigh && v >= high*0.85:
		return lipgloss.Color("220") // yellow
	default:
		return lipgloss.Color("78") // soft green
	}
}

// RenderSparkline renders a sparkline chart with color-coded blocks.
// Kept for backward compatibility (no timestamp ticks).
func RenderSparkline(values []float64, width int, rangeMin, rangeMax float64, high, crit float64, hasHigh, hasCrit bool) string {
	if width <= 0 {
		return ""
	}
	// Convert to points without timestamps (no tick marks)
	pts := make([]HistoryPoint, len(values))
	for i, v := range values {
		pts[i] = HistoryPoint{Temp: v}
	}
	return RenderSparklinePoints(pts, width, rangeMin, rangeMax, high, crit, hasHigh, hasCrit)
}

// RenderSparklinePoints renders a sparkline with minute tick marks on the
// timeline. A subtle '|' is drawn at each minute boundary.
func RenderSparklinePoints(points []HistoryPoint, width int, rangeMin, rangeMax float64, high, crit float64, hasHigh, hasCrit bool) string {
	if width <= 0 {
		return ""
	}

	if len(points) == 0 {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
		return dim.Render(strings.Repeat("╌", width))
	}

	if len(points) > width {
		points = points[len(points)-width:]
	}

	padLen := width - len(points)
	span := rangeMax - rangeMin
	if span <= 0 {
		span = 1
	}

	var sb strings.Builder

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	for i := 0; i < padLen; i++ {
		sb.WriteString(dim.Render("╌"))
	}

	tickStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("239"))

	for i, p := range points {
		norm := (p.Temp - rangeMin) / span
		norm = math.Max(0, math.Min(1, norm))

		idx := int(norm * 7)
		if idx > 7 {
			idx = 7
		}

		// Check if this point is at a minute boundary:
		// The second of this point's time is 0, OR the previous point was
		// in a different minute.
		isMinuteTick := false
		if !p.Time.IsZero() {
			if p.Time.Second() == 0 {
				isMinuteTick = true
			} else if i > 0 && !points[i-1].Time.IsZero() {
				if p.Time.Minute() != points[i-1].Time.Minute() {
					isMinuteTick = true
				}
			}
		}

		if isMinuteTick {
			// Draw a tick mark character — use a thin pipe that's subtle
			sb.WriteString(tickStyle.Render("│"))
		} else {
			ch := string(sparkBlocks[idx])
			color := tempColor(p.Temp, high, crit, hasHigh, hasCrit)
			style := lipgloss.NewStyle().Foreground(color)
			if hasCrit && p.Temp >= crit {
				style = style.Bold(true)
			}
			sb.WriteString(style.Render(ch))
		}
	}

	return sb.String()
}

// RenderTimeline renders the time labels under the sparkline, showing
// HH:MM at each minute tick position.
func RenderTimeline(points []HistoryPoint, width int) string {
	if len(points) == 0 || width <= 0 {
		return ""
	}

	if len(points) > width {
		points = points[len(points)-width:]
	}

	padLen := width - len(points)

	// Build a rune buffer for the timeline
	line := make([]rune, width)
	for i := range line {
		line[i] = ' '
	}

	tickStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("239"))

	// Collect tick positions and their labels
	type tick struct {
		pos   int
		label string
	}
	var ticks []tick

	for i, p := range points {
		if p.Time.IsZero() {
			continue
		}
		isMinuteTick := false
		if p.Time.Second() == 0 {
			isMinuteTick = true
		} else if i > 0 && !points[i-1].Time.IsZero() {
			if p.Time.Minute() != points[i-1].Time.Minute() {
				isMinuteTick = true
			}
		}
		if isMinuteTick {
			pos := padLen + i
			label := p.Time.Format("15:04")
			ticks = append(ticks, tick{pos: pos, label: label})
		}
	}

	// Place labels, skipping if they'd overlap (need at least 6 chars apart)
	lastEnd := -1
	for _, t := range ticks {
		start := t.pos - 2 // center the 5-char label
		if start < 0 {
			start = 0
		}
		end := start + len(t.label)
		if end > width {
			continue
		}
		if start <= lastEnd+1 {
			continue // too close to previous label
		}
		for j, ch := range t.label {
			line[start+j] = ch
		}
		lastEnd = end
	}

	// Render with dim style
	result := string(line)
	return tickStyle.Render(result)
}

// RenderThresholdScale renders a scale bar showing current position vs thresholds.
func RenderThresholdScale(current, rangeMin, rangeMax, high, crit float64, hasHigh, hasCrit bool, width int) string {
	if width <= 0 {
		return ""
	}

	span := rangeMax - rangeMin
	if span <= 0 {
		span = 1
	}

	bar := make([]rune, width)
	for i := range bar {
		bar[i] = '·'
	}

	if hasHigh && high > rangeMin {
		pos := int(float64(width-1) * (high - rangeMin) / span)
		if pos >= 0 && pos < width {
			bar[pos] = '▪'
		}
	}
	if hasCrit && crit > rangeMin {
		pos := int(float64(width-1) * (crit - rangeMin) / span)
		if pos >= 0 && pos < width {
			bar[pos] = '▪'
		}
	}

	curPos := int(float64(width-1) * (current - rangeMin) / span)
	if curPos < 0 {
		curPos = 0
	}
	if curPos >= width {
		curPos = width - 1
	}

	var sb strings.Builder
	for i, ch := range bar {
		if i == curPos {
			color := tempColor(current, high, crit, hasHigh, hasCrit)
			style := lipgloss.NewStyle().Foreground(color).Bold(true)
			sb.WriteString(style.Render("◆"))
		} else if ch == '▪' {
			highPos := -1
			critPos := -1
			if hasHigh && high > rangeMin {
				highPos = int(float64(width-1) * (high - rangeMin) / span)
			}
			if hasCrit && crit > rangeMin {
				critPos = int(float64(width-1) * (crit - rangeMin) / span)
			}
			if i == critPos {
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("▪"))
			} else if i == highPos {
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("▪"))
			} else {
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("▪"))
			}
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("236")).Render(string(ch)))
		}
	}

	return sb.String()
}

// RenderTempValue renders the temperature value with color coding.
func RenderTempValue(temp, high, crit float64, hasHigh, hasCrit bool) string {
	s := fmt.Sprintf("%5.1f°C", temp)
	color := tempColor(temp, high, crit, hasHigh, hasCrit)
	style := lipgloss.NewStyle().Foreground(color)
	if hasCrit && temp >= crit {
		style = style.Bold(true)
	}
	return style.Render(s)
}
