package tui

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Color Palette — Dark theme with purple accent
// ═══════════════════════════════════════════════════════════════════════════════

var (
	clrBg       = lipgloss.Color("#0D1117")
	clrSurface  = lipgloss.Color("#161B22")
	clrBorder   = lipgloss.Color("#30363D")
	clrAccent   = lipgloss.Color("#7C3AED") // Purple accent
	clrText     = lipgloss.Color("#C9D1D9")
	clrMuted    = lipgloss.Color("#8B949E")
	clrDim      = lipgloss.Color("#484F58")
	clrWhite    = lipgloss.Color("#FFFFFF")
	clrGreen    = lipgloss.Color("#3FB950")
	clrRed      = lipgloss.Color("#F85149")
	clrYellow   = lipgloss.Color("#D29922")
	clrCyan     = lipgloss.Color("#58A6FF")
	clrOrange   = lipgloss.Color("#DB6D28")

	// Base styles
	base = lipgloss.NewStyle().Foreground(clrText)

	// Borders
	borderRounded = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(clrBorder).
			Padding(1, 2)

	borderThick = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(clrAccent).
			Padding(0, 1)

	// Text styles
	textBold   = lipgloss.NewStyle().Bold(true).Foreground(clrWhite)
	textMuted  = lipgloss.NewStyle().Foreground(clrMuted)
	textDim    = lipgloss.NewStyle().Foreground(clrDim)
	textGreen  = lipgloss.NewStyle().Foreground(clrGreen)
	textRed    = lipgloss.NewStyle().Foreground(clrRed)
	textYellow = lipgloss.NewStyle().Foreground(clrYellow)
	textCyan   = lipgloss.NewStyle().Foreground(clrCyan)
	textAccent = lipgloss.NewStyle().Foreground(clrAccent).Bold(true)

	// Panel title
	panelTitle = lipgloss.NewStyle().
			Foreground(clrWhite).
			Background(clrAccent).
			Bold(true).
			Padding(0, 2)

	// Selection
	selRow = lipgloss.NewStyle().
		Background(lipgloss.Color("#1F1A3A")).
		Foreground(clrWhite)

	// Button styles
	btnActive = lipgloss.NewStyle().
			Foreground(clrWhite).
			Background(clrAccent).
			Bold(true).
			Padding(0, 1)

	btnInactive = lipgloss.NewStyle().
			Foreground(clrMuted).
			Background(clrSurface).
			Padding(0, 1)

	// Gauge
	gaugeFull    = lipgloss.NewStyle().Foreground(clrGreen)
	gaugeHigh    = lipgloss.NewStyle().Foreground(clrYellow)
	gaugeCritical = lipgloss.NewStyle().Foreground(clrRed)
	gaugeEmpty   = lipgloss.NewStyle().Foreground(clrDim)

	// Table header
	tblHeader = lipgloss.NewStyle().
			Foreground(clrMuted).
			Bold(true).
			Background(clrSurface)

	// Sparkline colors
	sparkRX = lipgloss.NewStyle().Foreground(clrGreen)
	sparkTX = lipgloss.NewStyle().Foreground(clrRed)

	// Focus highlight
	cardFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(clrAccent).
			Padding(1, 2)

	cardNormal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(clrBorder).
			Padding(1, 2)
)

// ═══════════════════════════════════════════════════════════════════════════════
// Model
// ═══════════════════════════════════════════════════════════════════════════════

const maxHistory = 120

type tick time.Time

// TopModel is the polished bandwidth monitor TUI.
type TopModel struct {
	width, height int
	mode          string // "all" | "docker"
	sortBy        string // "total" | "rx" | "tx"
	period        string // "1m" | "5m" | "1h" | "24h"
	selected      int
	scrollOffset  int
	entries       []topEntry
	totalRX, totalTX     float64
	avgRX, avgTX         float64
	p95RX, p95TX         float64
	peakRX, peakTX       float64
	rxHistory    map[string][]float64
	txHistory    map[string][]float64
	lastRefresh  time.Time
	quitting     bool
	help         bool
	focused      string // "list" | "graph"
}

type topEntry struct {
	Name      string
	Type      string
	RXMbps    float64
	TXMbps    float64
	TotalMbps float64
}

type refreshMsg struct {
	entries              []topEntry
	totalRX, totalTX     float64
	avgRX, avgTX         float64
	p95RX, p95TX         float64
	peakRX, peakTX       float64
}

func NewTopModel() TopModel {
	return TopModel{
		mode: "all", sortBy: "total", period: "5m",
		help: true, focused: "list",
		lastRefresh: time.Now(),
		rxHistory:   make(map[string][]float64),
		txHistory:   make(map[string][]float64),
	}
}

func (m TopModel) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tea.Tick(time.Second, func(t time.Time) tea.Msg { return tick(t) }), tea.EnableMouseCellMotion)
}

func (m TopModel) refresh() tea.Cmd {
	return func() tea.Msg {
		es := collectEntries(m.mode)
		trx, ttx := 0.0, 0.0
		rxa, txa := []float64{}, []float64{}
		for _, e := range es {
			trx += e.RXMbps; ttx += e.TXMbps
			rxa = append(rxa, e.RXMbps); txa = append(txa, e.TXMbps)
		}
		sort.Slice(es, func(i, j int) bool {
			switch m.sortBy {
			case "rx": return es[i].RXMbps > es[j].RXMbps
			case "tx": return es[i].TXMbps > es[j].TXMbps
			default: return es[i].TotalMbps > es[j].TotalMbps
			}
		})
		return refreshMsg{es, trx, ttx, avg(rxa), avg(txa), pct(rxa, 95), pct(txa, 95), maxv(rxa), maxv(txa)}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Update
// ═══════════════════════════════════════════════════════════════════════════════

func (m TopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress { return m, nil }
		if msg.Button == tea.MouseButtonWheelUp && m.selected > 0 { m.selected--; return m, nil }
		if msg.Button == tea.MouseButtonWheelDown && m.selected < len(m.entries)-1 { m.selected++; return m, nil }
		if msg.Button != tea.MouseButtonLeft { return m, nil }
		// Click handling: determine which region
		listW := m.width * 45 / 100
		if msg.X < listW && msg.Y >= 4 {
			row := msg.Y - 4 + m.scrollOffset
			if row < len(m.entries) { m.selected = row; m.focused = "list" }
		} else if msg.X >= listW && msg.Y >= 4 {
			m.focused = "graph"
		}
		// Bottom bar clicks
		if msg.Y >= m.height-2 {
			switch {
			case msg.X < 18: // Tab
				if m.mode == "all" { m.mode = "docker" } else { m.mode = "all" }
				m.selected = 0; m.scrollOffset = 0; return m, m.refresh()
			case msg.X >= 18 && msg.X < 30: // Sort
				switch m.sortBy { case "total": m.sortBy = "rx"; case "rx": m.sortBy = "tx"; case "tx": m.sortBy = "total" }
				return m, m.refresh()
			case msg.X >= 30 && msg.X < 38: m.period = "1m"; return m, m.refresh()
			case msg.X >= 38 && msg.X < 46: m.period = "5m"; return m, m.refresh()
			case msg.X >= 46 && msg.X < 54: m.period = "1h"; return m, m.refresh()
			case msg.X >= 54 && msg.X < 63: m.period = "24h"; return m, m.refresh()
			case msg.X >= 63: m.quitting = true; return m, tea.Quit
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c": m.quitting = true; return m, tea.Quit
		case "?": m.help = !m.help; return m, nil
		case "tab":
			m.focused = "list"
			if m.mode == "all" { m.mode = "docker" } else { m.mode = "all" }
			m.selected = 0; m.scrollOffset = 0; return m, m.refresh()
		case "up", "k":
			if m.selected > 0 { m.selected-- }
			if m.selected < m.scrollOffset { m.scrollOffset = m.selected }
			return m, nil
		case "down", "j":
			if m.selected < len(m.entries)-1 { m.selected++ }
			visible := m.height - 10
			if visible < 1 { visible = 1 }
			if m.selected >= m.scrollOffset+visible { m.scrollOffset = m.selected - visible + 1 }
			return m, nil
		case "left", "h": m.focused = "list"; return m, nil
		case "right", "l": m.focused = "graph"; return m, nil
		case "s":
			switch m.sortBy { case "total": m.sortBy = "rx"; case "rx": m.sortBy = "tx"; case "tx": m.sortBy = "total" }
			return m, m.refresh()
		case "1": m.period = "1m"; return m, m.refresh()
		case "5": m.period = "5m"; return m, m.refresh()
		case "t":
			if m.period == "1h" { m.period = "24h" } else { m.period = "1h" }
			return m, m.refresh()
		case "d": m.period = "24h"; return m, m.refresh()
		case "g": m.selected = 0; m.scrollOffset = 0; return m, nil
		case "G": m.selected = len(m.entries) - 1; m.scrollOffset = max(0, len(m.entries)-(m.height-10)); return m, nil
		}

	case tick:
		m.lastRefresh = time.Now()
		return m, tea.Batch(m.refresh(), tea.Tick(time.Second, func(t time.Time) tea.Msg { return tick(t) }))

	case refreshMsg:
		m.entries = msg.entries
		m.totalRX, m.totalTX = msg.totalRX, msg.totalTX
		m.avgRX, m.avgTX = msg.avgRX, msg.avgTX
		m.p95RX, m.p95TX = msg.p95RX, msg.p95TX
		m.peakRX, m.peakTX = msg.peakRX, msg.peakTX
		// Update history
		for _, e := range m.entries {
			m.rxHistory[e.Name] = append(m.rxHistory[e.Name], e.RXMbps)
			m.txHistory[e.Name] = append(m.txHistory[e.Name], e.TXMbps)
			if len(m.rxHistory[e.Name]) > maxHistory { m.rxHistory[e.Name] = m.rxHistory[e.Name][len(m.rxHistory[e.Name])-maxHistory:] }
			if len(m.txHistory[e.Name]) > maxHistory { m.txHistory[e.Name] = m.txHistory[e.Name][len(m.txHistory[e.Name])-maxHistory:] }
		}
		if m.selected >= len(m.entries) { m.selected = max(0, len(m.entries)-1) }
		return m, nil
	}
	return m, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// View
// ═══════════════════════════════════════════════════════════════════════════════

func (m TopModel) View() string {
	if m.quitting { return "" }
	if m.width < 80 { m.width = 80 }

	w, h := m.width, m.height
	listW := w * 45 / 100
	if listW < 35 { listW = 35 }
	graphW := w - listW - 1
	if graphW < 30 { graphW = 30 }

	// ── Header ──
	header := m.renderHeader(w)

	// ── Content: left list + right graph ──
	listContent := m.renderList(listW, h-10)
	graphContent := m.renderGraph(graphW, h-10)

	// Combine left + right panels
	leftLines := strings.Split(listContent, "\n")
	rightLines := strings.Split(graphContent, "\n")
	rows := max(len(leftLines), len(rightLines))
	var mid strings.Builder
	for i := 0; i < rows; i++ {
		l := ""
		if i < len(leftLines) { l = leftLines[i] }
		r := ""
		if i < len(rightLines) { r = rightLines[i] }
		mid.WriteString(fmt.Sprintf("%-*s %s\n", listW, l, r))
	}

	// ── Stats bar ──
	stats := m.renderStats(w)

	// ── Bottom bar ──
	bottom := m.renderBottom(w)

	return header + "\n" + strings.TrimRight(mid.String(), "\n") + "\n" + stats + "\n" + bottom
}

func (m TopModel) renderHeader(w int) string {
	mode := "🐳 Docker Only"
	if m.mode == "all" { mode = "🌐 All Interfaces" }
	title := fmt.Sprintf("⚡ Bandwidth Monitor — %s", mode)
	left := lipgloss.NewStyle().Foreground(clrWhite).Background(clrAccent).Bold(true).Padding(0, 2).Render(title)
	right := textMuted.Render(fmt.Sprintf("%d entries │ %s", len(m.entries), m.lastRefresh.Format("15:04:05")))
	spacer := w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if spacer < 1 { spacer = 1 }
	return left + strings.Repeat(" ", spacer) + right
}

func (m TopModel) renderList(w, maxH int) string {
	var sb strings.Builder

	// Total gauge
	total := m.totalRX + m.totalTX
	gaugeW := w - 4
	if gaugeW < 10 { gaugeW = 10 }
	sb.WriteString(fmt.Sprintf(" %s↓ %s↑  %s\n", humanMbps(m.totalRX), humanMbps(m.totalTX), gauge(total, float64(max(1, len(m.entries)))*200, gaugeW)))

	// Table header
	sb.WriteString(tblHeader.Render(fmt.Sprintf(" %-4s %-*s %6s %6s %6s", "#", w-30, "NAME", "RX", "TX", "TOT")))
	sb.WriteString("\n")

	// Rows
	visible := maxH - 4
	if visible < 3 { visible = 3 }
	for i := m.scrollOffset; i < min(len(m.entries), m.scrollOffset+visible); i++ {
		e := m.entries[i]
		c := textCyan
		if e.TotalMbps > 100 { c = textYellow }
		if e.TotalMbps > 500 { c = textRed }
		nameW := w - 30
		row := fmt.Sprintf(" %-4d %-*s %6s %6s %6s", i+1, nameW, trunc(e.Name, nameW), humanShort(e.RXMbps), humanShort(e.TXMbps), humanShort(e.TotalMbps))
		if i == m.selected {
			row = selRow.Render(row)
			if m.focused == "list" {
				row = lipgloss.NewStyle().Background(lipgloss.Color("#2D1F5E")).Foreground(clrWhite).Render(row)
			}
		} else {
			row = c.Render(row)
		}
		sb.WriteString(row + "\n")
	}

	// Fill remaining space
	for i := len(m.entries); i < visible; i++ {
		sb.WriteString("\n")
	}

	return borderRounded.Width(w).Render(sb.String())
}

func (m TopModel) renderGraph(w, maxH int) string {
	var sb strings.Builder

	if m.selected < 0 || m.selected >= len(m.entries) {
		return borderRounded.Width(w).Render(textMuted.Render("\n  Select an entry\n  to view graph"))
	}

	e := m.entries[m.selected]
	rxD := m.rxHistory[e.Name]
	txD := m.txHistory[e.Name]

	header := panelTitle.Render(fmt.Sprintf(" %s ", trunc(e.Name, w-4)))
	sb.WriteString(header + "\n")

	sb.WriteString(fmt.Sprintf(" ↓%s  ↑%s  Total: %s\n\n", humanMbps(e.RXMbps), humanMbps(e.TXMbps), humanMbps(e.TotalMbps)))

	graphH := maxH - 6
	if graphH < 5 { graphH = 5 }
	graphW := w - 6
	if graphW < 10 { graphW = 10 }
	sb.WriteString(drawSpark(rxD, txD, graphW, graphH))
	sb.WriteString("\n")
	sb.WriteString(textGreen.Render("── Inbound") + "  " + textRed.Render("── Outbound"))

	style := cardNormal
	if m.focused == "graph" { style = cardFocused }
	return style.Width(w).Render(sb.String())
}

func (m TopModel) renderStats(w int) string {
	stats := fmt.Sprintf("Period [%s] │ Avg ↓%s ↑%s │ 95th ↓%s ↑%s │ Peak ↓%s ↑%s",
		m.period, humanMbps(m.avgRX), humanMbps(m.avgTX),
		humanMbps(m.p95RX), humanMbps(m.p95TX),
		humanMbps(m.peakRX), humanMbps(m.peakTX))
	return borderThick.Width(w - 2).Render(stats)
}

func (m TopModel) renderBottom(w int) string {
	modeBtn := btnActive.Render("[Tab]Filter")
	sortBtn := btnActive.Render("[s]Sort")
	p1 := btnInactive.Render("[1]1m")
	p5 := btnInactive.Render("[5]5m")
	ph := btnInactive.Render("[t]1h")
	pd := btnInactive.Render("[d]24h")
	qBtn := lipgloss.NewStyle().Foreground(clrWhite).Background(clrRed).Bold(true).Padding(0, 1).Render("[q]Quit")

	switch m.period {
	case "1m": p1 = btnActive.Render("[1]1m")
	case "5m": p5 = btnActive.Render("[5]5m")
	case "1h": ph = btnActive.Render("[t]1h")
	case "24h": pd = btnActive.Render("[d]24h")
	}

	buttons := modeBtn + sortBtn + p1 + p5 + ph + pd + qBtn
	help := textDim.Render("↑↓j/k:Navigate  Tab:Filter  s:Sort  1/5/t/d:Period  g/G:Top/Bottom  ?:Help")
	return fmt.Sprintf("%s\n%s", buttons, help)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Sparkline Graph
// ═══════════════════════════════════════════════════════════════════════════════

func drawSpark(rx, tx []float64, w, h int) string {
	if len(rx) == 0 && len(tx) == 0 {
		return textMuted.Render("collecting data...")
	}

	// Find scale
	maxVal := 1.0
	for _, v := range rx { if v > maxVal { maxVal = v } }
	for _, v := range tx { if v > maxVal { maxVal = v } }
	maxVal *= 1.15

	// Build pixel grid
	grid := make([][]rune, h)
	for y := 0; y < h; y++ {
		grid[y] = make([]rune, w)
		for x := 0; x < w; x++ { grid[y][x] = ' ' }
	}

	plot := func(data []float64, ch rune) {
		if len(data) == 0 { return }
		step := float64(len(data)-1) / float64(max(1, w-1))
		for x := 0; x < w; x++ {
			idx := int(float64(x) * step)
			if idx >= len(data) { idx = len(data) - 1 }
			y := h - 1 - int(math.Round(data[idx]/maxVal*float64(h-1)))
			if y < 0 { y = 0 }
			if y >= h { y = h - 1 }
			if grid[y][x] == ' ' { grid[y][x] = ch } else { grid[y][x] = '█' }
		}
	}

	plot(rx, '░')
	plot(tx, '▒')

	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			switch grid[y][x] {
			case '░': sb.WriteString(sparkRX.Render("▄"))
			case '▒': sb.WriteString(sparkTX.Render("▄"))
			case '█': sb.WriteString(textYellow.Render("▄"))
			default: sb.WriteRune(' ')
			}
		}
		sb.WriteRune('\n')
	}

	// Scale labels
	sb.WriteString(textDim.Render(humanMbps(maxVal)))
	sb.WriteString(strings.Repeat(" ", max(0, w-10)))
	sb.WriteString(textDim.Render("0"))

	return sb.String()
}

// ═══════════════════════════════════════════════════════════════════════════════
// Gauge
// ═══════════════════════════════════════════════════════════════════════════════

func gauge(val, max float64, w int) string {
	r := val / max
	if r > 1 { r = 1 }
	filled := int(r * float64(w))
	var sb strings.Builder
	for i := 0; i < w; i++ {
		if i < filled {
			pct := float64(i) / float64(w)
			switch {
			case pct > 0.85: sb.WriteString(gaugeCritical.Render("█"))
			case pct > 0.6: sb.WriteString(gaugeHigh.Render("█"))
			default: sb.WriteString(gaugeFull.Render("█"))
			}
		} else {
			sb.WriteString(gaugeEmpty.Render("░"))
		}
	}
	return sb.String()
}

// ═══════════════════════════════════════════════════════════════════════════════
// Data Collection
// ═══════════════════════════════════════════════════════════════════════════════

func collectEntries(mode string) []topEntry {
	var es []topEntry
	if mode == "all" || mode == "docker" { es = append(es, dockerStats()...) }
	if mode == "all" { es = append(es, ifaceStats()...) }
	return es
}

func dockerStats() []topEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.NetIO}}").CombinedOutput()
	if err != nil { return nil }
	var es []topEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		p := strings.Split(line, "\t")
		if len(p) < 2 { continue }
		rx, tx := parseNet(p[1])
		es = append(es, topEntry{Name: p[0], Type: "docker", RXMbps: rx, TXMbps: tx, TotalMbps: rx + tx})
	}
	return es
}

func ifaceStats() []topEntry {
	out, _ := exec.Command("ls", "/sys/class/net").CombinedOutput()
	var es []topEntry
	for _, iface := range strings.Fields(string(out)) {
		if iface == "lo" { continue }
		rx, _ := readUint64(fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", iface))
		tx, _ := readUint64(fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", iface))
		r := float64(rx) / 1e6; t := float64(tx) / 1e6
		if r == 0 && t == 0 { continue }
		es = append(es, topEntry{Name: iface, Type: "iface", RXMbps: r, TXMbps: t, TotalMbps: r + t})
	}
	return es
}

func parseNet(s string) (float64, float64) {
	p := strings.SplitN(s, "/", 2)
	if len(p) < 2 { return 0, 0 }
	return parseSize(strings.TrimSpace(p[0])), parseSize(strings.TrimSpace(p[1]))
}

func parseSize(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" { return 0 }
	var v float64; var u string; fmt.Sscanf(s, "%f%s", &v, &u)
	switch strings.ToUpper(u) {
	case "B": return v / 1e6
	case "KB", "KIB": return v / 1000
	case "MB", "MIB": return v
	case "GB", "GIB": return v * 1000
	case "TB": return v * 1e6
	default: return v / 1e6
	}
}

func readUint64(path string) (uint64, error) {
	out, err := exec.Command("cat", path).CombinedOutput()
	if err != nil { return 0, err }
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════════

func humanMbps(v float64) string {
	switch {
	case v >= 10000: return fmt.Sprintf("%.1fG", v/1000)
	case v >= 1000: return fmt.Sprintf("%.2fG", v/1000)
	case v >= 10: return fmt.Sprintf("%.0fM", v)
	case v >= 1: return fmt.Sprintf("%.1fM", v)
	case v >= 0.01: return fmt.Sprintf("%.0fK", v*1000)
	default: return "0"
	}
}

func humanShort(v float64) string {
	switch {
	case v >= 1000: return fmt.Sprintf("%.1fG", v/1000)
	case v >= 100: return fmt.Sprintf("%.0fM", v)
	case v >= 10: return fmt.Sprintf("%.1fM", v)
	case v >= 1: return fmt.Sprintf("%.2fM", v)
	case v >= 0.01: return fmt.Sprintf("%.0fK", v*1000)
	default: return "0"
	}
}

func avg(v []float64) float64 {
	if len(v) == 0 { return 0 }
	s := 0.0; for _, x := range v { s += x }
	return s / float64(len(v))
}

func pct(v []float64, p float64) float64 {
	if len(v) == 0 { return 0 }
	c := make([]float64, len(v)); copy(c, v)
	sort.Float64s(c)
	idx := int(float64(len(c)-1) * p / 100.0)
	if idx >= len(c) { idx = len(c) - 1 }
	return c[idx]
}

func maxv(v []float64) float64 {
	if len(v) == 0 { return 0 }
	m := v[0]; for _, x := range v[1:] { if x > m { m = x } }
	return m
}

func trunc(s string, max int) string {
	if len(s) <= max { return s }
	return s[:max-1] + "…"
}
