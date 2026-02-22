package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── History Viewer ───────────────────────────────────────────────────

// runHistoryViewer launches the historical data viewer TUI.
func runHistoryViewer() {
	days, err := ListDays("")
	if err != nil || len(days) == 0 {
		fmt.Fprintf(os.Stderr, "No history data found in %s\n", DataDir())
		os.Exit(1)
	}

	p := tea.NewProgram(
		initViewerModel(days),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ── Viewer Model ─────────────────────────────────────────────────────

type viewerModel struct {
	days     []string        // available dates
	dayIdx   int             // currently selected day
	readings []StoredReading // all readings for current day
	sensors  []string        // unique sensor keys (sorted)
	cursor   int             // time cursor position (index into time slots)
	scroll   int             // vertical scroll offset
	width    int
	height   int
	err      error

	// Derived from readings
	timeSlots  []time.Time            // unique timestamps (sorted)
	series     map[string][]dataPoint // sensor key -> sorted data points
	thresholds map[string][2]float64  // sensor key -> [high, crit]
}

type dataPoint struct {
	time time.Time
	temp float64
}

func initViewerModel(days []string) viewerModel {
	m := viewerModel{
		days:   days,
		dayIdx: 0,
	}
	m.loadDay()
	return m
}

func (m *viewerModel) loadDay() {
	day := m.days[m.dayIdx]
	readings, err := LoadDay(day)
	if err != nil {
		m.err = err
		return
	}
	m.readings = readings
	m.err = nil

	// Build time slots and series
	timeSet := make(map[int64]time.Time)
	seriesMap := make(map[string][]dataPoint)
	threshMap := make(map[string][2]float64)
	sensorSet := make(map[string]bool)

	for _, r := range readings {
		key := r.Chip + "/" + r.Label
		sensorSet[key] = true
		timeSet[r.Time.Unix()] = r.Time
		seriesMap[key] = append(seriesMap[key], dataPoint{time: r.Time, temp: r.Temp})

		if r.High > 0 || r.Crit > 0 {
			threshMap[key] = [2]float64{r.High, r.Crit}
		}
	}

	// Sort sensors
	var sensors []string
	for k := range sensorSet {
		sensors = append(sensors, k)
	}
	sort.Strings(sensors)
	m.sensors = sensors

	// Sort time slots
	var times []time.Time
	for _, t := range timeSet {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	m.timeSlots = times

	// Sort each series by time
	for k, pts := range seriesMap {
		sort.Slice(pts, func(i, j int) bool { return pts[i].time.Before(pts[j].time) })
		seriesMap[k] = pts
	}
	m.series = seriesMap
	m.thresholds = threshMap

	// Start cursor at the end
	if len(m.timeSlots) > 0 {
		m.cursor = len(m.timeSlots) - 1
	}
	m.scroll = 0
}

// ── Viewer Init / Update ─────────────────────────────────────────────

func (m viewerModel) Init() tea.Cmd {
	return nil
}

func (m viewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		// Time navigation
		case "left", "h":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right", "l":
			if m.cursor < len(m.timeSlots)-1 {
				m.cursor++
			}
		case "shift+left", "H":
			m.cursor -= 60
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "shift+right", "L":
			m.cursor += 60
			if m.cursor >= len(m.timeSlots) {
				m.cursor = len(m.timeSlots) - 1
			}
		case "home":
			m.cursor = 0
		case "end":
			if len(m.timeSlots) > 0 {
				m.cursor = len(m.timeSlots) - 1
			}

		// Day navigation
		case "[":
			if m.dayIdx < len(m.days)-1 {
				m.dayIdx++
				m.loadDay()
			}
		case "]":
			if m.dayIdx > 0 {
				m.dayIdx--
				m.loadDay()
			}

		// Scroll
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// ── Viewer View ──────────────────────────────────────────────────────

func (m viewerModel) View() string {
	if m.width == 0 {
		return "  Loading..."
	}

	contentWidth := m.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	var sections []string

	// Title bar
	sections = append(sections, m.renderViewerTitle(contentWidth))

	if m.err != nil {
		errBox := lipgloss.NewStyle().
			Foreground(colorCrit).
			Bold(true).
			Padding(0, 1).
			Render(fmt.Sprintf("ERROR: %v", m.err))
		sections = append(sections, errBox)
	}

	if len(m.timeSlots) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(colorDim).
			Padding(2, 0).
			Align(lipgloss.Center).
			Width(contentWidth).
			Render("No data for this day.")
		sections = append(sections, empty)
	} else {
		// Time cursor info
		sections = append(sections, m.renderCursorInfo(contentWidth))

		// Sensor panels
		panels := m.renderViewerPanels(contentWidth)
		sections = append(sections, panels...)
	}

	// Footer
	sections = append(sections, m.renderViewerFooter(contentWidth))

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Scroll
	lines := strings.Split(content, "\n")
	visibleLines := m.height
	if visibleLines < 5 {
		visibleLines = 5
	}
	maxScroll := len(lines) - visibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}

	start := m.scroll
	end := start + visibleLines
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start:end], "\n")
}

func (m viewerModel) renderViewerTitle(width int) string {
	logo := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitleFg).
		Render("SENSORS HISTORY")

	day := m.days[m.dayIdx]
	dayText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true).
		Render(day)

	nav := lipgloss.NewStyle().
		Foreground(colorDim).
		Render(fmt.Sprintf("  [ %d/%d ]", m.dayIdx+1, len(m.days)))

	dataInfo := ""
	if len(m.timeSlots) > 0 {
		first := m.timeSlots[0].Format("15:04:05")
		last := m.timeSlots[len(m.timeSlots)-1].Format("15:04:05")
		dataInfo = lipgloss.NewStyle().
			Foreground(colorDim).
			Render(fmt.Sprintf("  %s - %s  (%d readings, %d sensors)",
				first, last, len(m.readings), len(m.sensors)))
	}

	right := dayText + nav + dataInfo

	gap := width - lipgloss.Width(logo) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	filler := strings.Repeat(" ", gap)

	return lipgloss.NewStyle().
		Background(colorTitleBg).
		Width(width).
		Padding(0, 1).
		Render(logo + filler + right)
}

func (m viewerModel) renderCursorInfo(width int) string {
	if m.cursor < 0 || m.cursor >= len(m.timeSlots) {
		return ""
	}

	t := m.timeSlots[m.cursor]
	ts := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true).
		Render(t.Format("15:04:05"))

	pos := lipgloss.NewStyle().
		Foreground(colorDim).
		Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.timeSlots)))

	// Mini scrubber bar
	barWidth := width - 30
	if barWidth < 10 {
		barWidth = 10
	}
	scrubber := m.renderScrubber(barWidth)

	return lipgloss.NewStyle().
		Padding(0, 1).
		Render("  " + ts + pos + "  " + scrubber)
}

func (m viewerModel) renderScrubber(width int) string {
	if len(m.timeSlots) == 0 || width <= 0 {
		return ""
	}

	bar := make([]rune, width)
	for i := range bar {
		bar[i] = '─'
	}

	// Position cursor
	pos := 0
	if len(m.timeSlots) > 1 {
		pos = m.cursor * (width - 1) / (len(m.timeSlots) - 1)
	}
	if pos >= width {
		pos = width - 1
	}

	var sb strings.Builder
	dimS := lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
	curS := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	tickS := lipgloss.NewStyle().Foreground(lipgloss.Color("239"))

	for i := range bar {
		if i == pos {
			sb.WriteString(curS.Render("◆"))
		} else {
			// Mark hour boundaries
			slotIdx := 0
			if len(m.timeSlots) > 1 {
				slotIdx = i * (len(m.timeSlots) - 1) / (width - 1)
			}
			if slotIdx > 0 && slotIdx < len(m.timeSlots) {
				t := m.timeSlots[slotIdx]
				tPrev := m.timeSlots[slotIdx-1]
				if t.Hour() != tPrev.Hour() {
					sb.WriteString(tickS.Render("│"))
					continue
				}
			}
			sb.WriteString(dimS.Render("─"))
		}
	}

	return sb.String()
}

func (m viewerModel) renderViewerPanels(totalWidth int) []string {
	if m.cursor < 0 || m.cursor >= len(m.timeSlots) {
		return nil
	}

	cursorTime := m.timeSlots[m.cursor]

	innerWidth := totalWidth - 4
	if innerWidth < 30 {
		innerWidth = 30
	}

	chartWidth := innerWidth - 60
	if chartWidth < 15 {
		chartWidth = 15
	}
	if chartWidth > 140 {
		chartWidth = 140
	}

	labelW := 16
	tempW := 8

	// Group sensors by chip
	type chipGroup struct {
		chip    string
		sensors []string
	}
	chipMap := make(map[string]*chipGroup)
	var chipOrder []string

	for _, key := range m.sensors {
		parts := strings.SplitN(key, "/", 2)
		chip := parts[0]
		g, ok := chipMap[chip]
		if !ok {
			g = &chipGroup{chip: chip}
			chipMap[chip] = g
			chipOrder = append(chipOrder, chip)
		}
		g.sensors = append(g.sensors, key)
	}

	var panels []string

	for _, chipName := range chipOrder {
		g := chipMap[chipName]

		var rows []string

		friendly := ChipFriendlyName(g.chip)
		friendlyText := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorChipName).
			Render(friendly)
		chipID := lipgloss.NewStyle().
			Foreground(colorDim).
			Render(g.chip)
		rows = append(rows, friendlyText+"  "+chipID)

		colLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Width(labelW).Render("sensor")
		colVal := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Width(tempW).Align(lipgloss.Right).Render("value")
		colHistPad := strings.Repeat(" ", chartWidth/2-3)
		colHist := lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(colHistPad + "history")
		rows = append(rows, colLabel+" "+colVal+"  "+colHist)

		sep := lipgloss.NewStyle().
			Foreground(lipgloss.Color("237")).
			Render(strings.Repeat("─", innerWidth))
		rows = append(rows, sep)

		for _, key := range g.sensors {
			pts, ok := m.series[key]
			if !ok || len(pts) == 0 {
				continue
			}

			parts := strings.SplitN(key, "/", 2)
			sensorLabel := parts[0]
			if len(parts) > 1 {
				sensorLabel = parts[1]
			}

			thresh := m.thresholds[key]
			high, crit := thresh[0], thresh[1]
			hasHigh := high > 0
			hasCrit := crit > 0

			// Find temp at cursor time
			curTemp := findTempAtTime(pts, cursorTime)

			// Compute range
			minV, maxV := math.MaxFloat64, -math.MaxFloat64
			for _, p := range pts {
				if p.temp < minV {
					minV = p.temp
				}
				if p.temp > maxV {
					maxV = p.temp
				}
			}
			rangeMin := math.Max(0, minV-5)
			rangeMax := maxV + 5
			if hasCrit && crit > rangeMax {
				rangeMax = crit + 5
			}
			if hasHigh && high > rangeMax {
				rangeMax = high + 5
			}

			// Build sparkline — show the window around the cursor
			sparkPts := buildSparkWindow(pts, m.cursor, chartWidth, m.timeSlots)

			label := lipgloss.NewStyle().
				Foreground(colorLabel).
				Bold(true).
				Width(labelW).
				Render(truncate(sensorLabel, labelW))

			temp := lipgloss.NewStyle().
				Width(tempW).
				Align(lipgloss.Right).
				Render(RenderTempValue(curTemp, high, crit, hasHigh, hasCrit))

			spark := RenderSparklinePoints(sparkPts, chartWidth, rangeMin, rangeMax, high, crit, hasHigh, hasCrit)

			frameL := lipgloss.NewStyle().Foreground(colorBorder).Render("▕")
			frameR := lipgloss.NewStyle().Foreground(colorBorder).Render("▏")
			framedSpark := frameL + spark + frameR

			// Stats for full day
			avg := 0.0
			for _, p := range pts {
				avg += p.temp
			}
			avg /= float64(len(pts))

			dimS := lipgloss.NewStyle().Foreground(colorDim)
			valS := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
			stats := dimS.Render("avg") + valS.Render(fmt.Sprintf("%5.1f", avg)) +
				dimS.Render(" lo") + valS.Render(fmt.Sprintf("%5.1f", minV)) +
				dimS.Render(" pk") + valS.Render(fmt.Sprintf("%5.1f", maxV))

			var threshTags string
			if hasHigh {
				threshTags += " " + lipgloss.NewStyle().Foreground(colorWarn).Render(fmt.Sprintf("H:%.0f°", high))
			}
			if hasCrit {
				threshTags += " " + lipgloss.NewStyle().Foreground(colorCrit).Render(fmt.Sprintf("C:%.0f°", crit))
			}

			row := label + " " + temp + " " + framedSpark + " " + stats + threshTags
			rows = append(rows, row)

			// Timeline
			timeline := RenderTimeline(sparkPts, chartWidth)
			if strings.TrimSpace(timeline) != "" {
				pad := strings.Repeat(" ", labelW+tempW+2)
				rows = append(rows, pad+" "+timeline)
			}
		}

		panelContent := lipgloss.JoinVertical(lipgloss.Left, rows...)
		panel := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			Width(totalWidth).
			Render(panelContent)

		panels = append(panels, panel)
	}

	return panels
}

func (m viewerModel) renderViewerFooter(width int) string {
	dimS := lipgloss.NewStyle().Foreground(colorDim)
	keyS := lipgloss.NewStyle().Foreground(colorLabel)

	keys := dimS.Render("q") + keyS.Render(":quit") +
		dimS.Render("  h/l") + keyS.Render(":scrub") +
		dimS.Render("  H/L") + keyS.Render(":skip 1m") +
		dimS.Render("  home/end") + keyS.Render(":jump") +
		dimS.Render("  [/]") + keyS.Render(":day") +
		dimS.Render("  j/k") + keyS.Render(":scroll")

	return lipgloss.NewStyle().
		Background(colorFooterBg).
		Width(width).
		Padding(0, 1).
		Render(keys)
}

// ── Helpers ──────────────────────────────────────────────────────────

// findTempAtTime finds the temperature closest to the given time.
func findTempAtTime(pts []dataPoint, t time.Time) float64 {
	// Binary search for closest
	best := pts[0].temp
	bestDiff := absDuration(pts[0].time.Sub(t))
	for _, p := range pts {
		diff := absDuration(p.time.Sub(t))
		if diff < bestDiff {
			bestDiff = diff
			best = p.temp
		}
		if p.time.After(t) && diff > bestDiff {
			break // past our point, getting further
		}
	}
	return best
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// buildSparkWindow creates a window of HistoryPoints around the cursor
// position for the sparkline to render.
func buildSparkWindow(pts []dataPoint, cursorIdx int, width int, timeSlots []time.Time) []HistoryPoint {
	if len(pts) == 0 || len(timeSlots) == 0 {
		return nil
	}

	// Determine the time window: cursor at the right edge, showing `width` seconds back
	cursorTime := timeSlots[cursorIdx]

	// Build a time->temp lookup for fast access
	tempMap := make(map[int64]float64)
	for _, p := range pts {
		tempMap[p.time.Unix()] = p.temp
	}

	// Walk backwards from cursor to fill the window
	var result []HistoryPoint
	for i := width - 1; i >= 0; i-- {
		slotIdx := cursorIdx - i
		if slotIdx < 0 {
			continue
		}
		if slotIdx >= len(timeSlots) {
			continue
		}
		t := timeSlots[slotIdx]
		if temp, ok := tempMap[t.Unix()]; ok {
			result = append(result, HistoryPoint{Temp: temp, Time: t})
		}
	}

	// If cursor points to a valid time, make sure we include it
	if temp, ok := tempMap[cursorTime.Unix()]; ok {
		if len(result) == 0 || result[len(result)-1].Time != cursorTime {
			result = append(result, HistoryPoint{Temp: temp, Time: cursorTime})
		}
	}

	return result
}
