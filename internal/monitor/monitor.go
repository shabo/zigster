// Package monitor implements the live temperature monitoring TUI using
// BubbleTea with real-time sparkline charts and color-coded thresholds.
package monitor

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/luki/sensors/internal/chart"
	"github.com/luki/sensors/internal/history"
	"github.com/luki/sensors/internal/sensor"
	"github.com/luki/sensors/internal/store"
)

const (
	pollInterval = 1 * time.Second
	historySize  = 600 // 10 minutes at 1s interval
)

// ── Messages ─────────────────────────────────────────────────────────

type tickMsg time.Time

type sensorDataMsg struct {
	readings []sensor.Reading
	time     time.Time
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// ── Model ────────────────────────────────────────────────────────────

// Model is the BubbleTea model for the live monitor.
type Model struct {
	readings  []sensor.Reading
	history   *history.Store
	store     *store.DiskStore
	order     []string
	err       error
	width     int
	height    int
	scroll    int
	lastPoll  time.Time
	startTime time.Time
	paused    bool
}

// New creates the initial model for the live monitor.
func New() Model {
	ds, err := store.New()
	m := Model{
		history:   history.NewStore(historySize),
		store:     ds,
		startTime: time.Now(),
	}
	if err != nil {
		m.err = fmt.Errorf("disk store: %w", err)
	}
	return m
}

// ── Commands ─────────────────────────────────────────────────────────

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func pollSensors() tea.Msg {
	readings, err := sensor.ReadAll()
	if err != nil {
		return errMsg{err}
	}
	return sensorDataMsg{readings: readings, time: time.Now()}
}

// ── Init / Update ────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(pollSensors, tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.store != nil {
				m.store.Close()
			}
			return m, tea.Quit
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		case "home":
			m.scroll = 0
		case " ", "p":
			m.paused = !m.paused
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		if m.paused {
			return m, tickCmd()
		}
		return m, tea.Batch(pollSensors, tickCmd())

	case sensorDataMsg:
		m.readings = msg.readings
		m.lastPoll = msg.time
		for _, r := range msg.readings {
			m.history.Record(r.Key(), r.Temp, msg.time)
		}
		m.order = buildOrder(m.readings, m.order)

		if m.store != nil {
			if err := m.store.Write(msg.readings, msg.time); err != nil {
				m.err = fmt.Errorf("write: %w", err)
			}
		}

	case errMsg:
		m.err = msg.err
		return m, tickCmd()
	}

	return m, nil
}

func buildOrder(readings []sensor.Reading, existing []string) []string {
	seen := make(map[string]bool)
	for _, k := range existing {
		seen[k] = true
	}
	var newKeys []string
	for _, r := range readings {
		k := r.Key()
		if !seen[k] {
			newKeys = append(newKeys, k)
			seen[k] = true
		}
	}
	sort.Strings(newKeys)
	return append(existing, newKeys...)
}

// ── Color palette ────────────────────────────────────────────────────

var (
	colorTitleBg  = lipgloss.Color("17")
	colorTitleFg  = lipgloss.Color("51")
	colorBorder   = lipgloss.Color("62")
	colorChipName = lipgloss.Color("147")
	colorAdapter  = lipgloss.Color("243")
	colorLabel    = lipgloss.Color("252")
	colorDim      = lipgloss.Color("240")
	colorFooterBg = lipgloss.Color("235")
	colorOk       = lipgloss.Color("78")
	colorWarn     = lipgloss.Color("220")
	colorHigh     = lipgloss.Color("208")
	colorCrit     = lipgloss.Color("196")
	colorPaused   = lipgloss.Color("196")
)

// ── View ─────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "  Initializing..."
	}

	contentWidth := m.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	var sections []string

	sections = append(sections, m.renderTitleBar(contentWidth))

	if m.err != nil {
		errBox := lipgloss.NewStyle().
			Foreground(colorCrit).
			Bold(true).
			Width(contentWidth).
			Padding(0, 1).
			Render(fmt.Sprintf(" ERROR: %v", m.err))
		sections = append(sections, errBox)
	}

	if len(m.readings) == 0 {
		waiting := lipgloss.NewStyle().
			Foreground(colorDim).
			Width(contentWidth).
			Align(lipgloss.Center).
			Padding(2, 0).
			Render("Waiting for sensor data...")
		sections = append(sections, waiting)
	} else {
		panels := m.renderSensorPanels(contentWidth)
		sections = append(sections, panels...)
	}

	sections = append(sections, m.renderFooter(contentWidth))

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

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

func (m Model) renderTitleBar(width int) string {
	logo := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitleFg).
		Render("SENSORS MONITOR")

	var statusParts []string

	uptime := lipgloss.NewStyle().
		Foreground(colorDim).
		Render(fmt.Sprintf("up %s", fmtDuration(time.Since(m.startTime))))
	statusParts = append(statusParts, uptime)

	if !m.lastPoll.IsZero() {
		ts := lipgloss.NewStyle().
			Foreground(colorDim).
			Render(m.lastPoll.Format("15:04:05"))
		statusParts = append(statusParts, ts)
	}

	if m.paused {
		p := lipgloss.NewStyle().
			Foreground(colorPaused).
			Bold(true).
			Render("PAUSED")
		statusParts = append(statusParts, p)
	}

	if m.store != nil {
		rec := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Render("REC") +
			lipgloss.NewStyle().
				Foreground(colorDim).
				Render(" "+store.DataDir())
		statusParts = append(statusParts, rec)
	}

	sep := lipgloss.NewStyle().Foreground(colorDim).Render(" \u2502 ")
	right := strings.Join(statusParts, sep)

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

func (m Model) renderSensorPanels(totalWidth int) []string {
	type chipGroup struct {
		chip     string
		adapter  string
		readings []sensor.Reading
	}
	chipMap := make(map[string]*chipGroup)
	var chipOrder []string

	for _, r := range m.readings {
		g, ok := chipMap[r.Chip]
		if !ok {
			g = &chipGroup{chip: r.Chip, adapter: r.Adapter}
			chipMap[r.Chip] = g
			chipOrder = append(chipOrder, r.Chip)
		}
		g.readings = append(g.readings, r)
	}

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

	labelW := 14
	tempW := 7

	dimS := lipgloss.NewStyle().Foreground(colorDim)
	valS := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	frameL := lipgloss.NewStyle().Foreground(colorBorder).Render("\u2595")
	frameR := lipgloss.NewStyle().Foreground(colorBorder).Render("\u258F")

	var panels []string

	for _, chipName := range chipOrder {
		g := chipMap[chipName]

		var rows []string

		friendly := sensor.FriendlyName(g.chip)
		friendlyText := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorChipName).
			Render(friendly)
		chipID := lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Render(g.chip)
		adapterText := lipgloss.NewStyle().
			Foreground(colorAdapter).
			Render(g.adapter)
		rows = append(rows, friendlyText+"  "+chipID+"  "+adapterText)

		var lastPts []history.Point

		for _, r := range g.readings {
			hist := m.history.Get(r.Key())
			if hist == nil {
				continue
			}

			rangeMin := math.Max(0, hist.Min-5)
			rangeMax := hist.Peak + 5
			if r.HasCrit && r.Crit > rangeMax {
				rangeMax = r.Crit + 5
			}
			if r.HasHigh && r.High > rangeMax {
				rangeMax = r.High + 5
			}

			label := lipgloss.NewStyle().
				Foreground(colorLabel).
				Width(labelW).
				Render(truncate(r.Label, labelW))

			temp := lipgloss.NewStyle().
				Width(tempW).
				Align(lipgloss.Right).
				Render(chart.RenderTempValue(r.Temp, r.High, r.Crit, r.HasHigh, r.HasCrit))

			pts := hist.LastNPoints(chartWidth)
			lastPts = pts
			spark := chart.RenderSparklinePoints(pts, chartWidth, rangeMin, rangeMax, r.High, r.Crit, r.HasHigh, r.HasCrit)
			framedSpark := frameL + spark + frameR

			stats := dimS.Render(" avg") + valS.Render(fmt.Sprintf("%5.1f", hist.Avg())) +
				dimS.Render(" lo") + valS.Render(fmt.Sprintf("%5.1f", hist.Min)) +
				dimS.Render(" pk") + valS.Render(fmt.Sprintf("%5.1f", hist.Peak))

			var threshTags string
			if r.HasHigh {
				threshTags += dimS.Render(" H") + lipgloss.NewStyle().Foreground(colorWarn).Render(fmt.Sprintf("%.0f", r.High))
			}
			if r.HasCrit {
				threshTags += dimS.Render(" C") + lipgloss.NewStyle().Foreground(colorCrit).Render(fmt.Sprintf("%.0f", r.Crit))
			}

			row := label + " " + temp + " " + framedSpark + stats + threshTags
			rows = append(rows, row)
		}

		if lastPts != nil {
			timeline := chart.RenderTimeline(lastPts, chartWidth)
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

func (m Model) renderFooter(width int) string {
	okS := lipgloss.NewStyle().Foreground(colorOk).Render("\u2588\u2588")
	warnS := lipgloss.NewStyle().Foreground(colorWarn).Render("\u2588\u2588")
	highS := lipgloss.NewStyle().Foreground(colorHigh).Render("\u2588\u2588")
	critS := lipgloss.NewStyle().Foreground(colorCrit).Render("\u2588\u2588")
	tickS := lipgloss.NewStyle().Foreground(lipgloss.Color("239")).Render("\u2502")

	dimS := lipgloss.NewStyle().Foreground(colorDim)
	legend := okS + dimS.Render(" ok ") +
		warnS + dimS.Render(" warm ") +
		highS + dimS.Render(" high ") +
		critS + dimS.Render(" crit ") +
		tickS + dimS.Render(" 1min")

	keys := dimS.Render("q") + lipgloss.NewStyle().Foreground(colorLabel).Render(":quit") +
		dimS.Render("  j/k") + lipgloss.NewStyle().Foreground(colorLabel).Render(":scroll") +
		dimS.Render("  p") + lipgloss.NewStyle().Foreground(colorLabel).Render(":pause")

	gap := width - lipgloss.Width(legend) - lipgloss.Width(keys) - 4
	if gap < 1 {
		gap = 1
	}
	filler := strings.Repeat(" ", gap)

	return lipgloss.NewStyle().
		Background(colorFooterBg).
		Width(width).
		Padding(0, 1).
		Render(legend + filler + keys)
}

func truncate(s string, w int) string {
	if len(s) <= w {
		return s
	}
	if w <= 3 {
		return s[:w]
	}
	return s[:w-1] + "\u2026"
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}
