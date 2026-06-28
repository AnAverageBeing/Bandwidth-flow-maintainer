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

// ─── Styles ───────────────────────────────────────────────────────────────────

var (
	tuiPurple   = lipgloss.Color("#7B68EE")
	tuiDkPurple = lipgloss.Color("#3A3A5A")
	tuiGreen    = lipgloss.Color("#00FF00")
	tuiRed      = lipgloss.Color("#FF4500")
	tuiYellow   = lipgloss.Color("#FFD700")
	tuiCyan     = lipgloss.Color("#00FFFF")
	tuiWhite    = lipgloss.Color("#FFFFFF")
	tuiMuted    = lipgloss.Color("#666666")

	topBar    = lipgloss.NewStyle().Bold(true).Foreground(tuiWhite).Background(tuiPurple).Padding(0, 1)
	topHeader = lipgloss.NewStyle().Bold(true).Foreground(tuiWhite).Background(tuiDkPurple)
	topSel    = lipgloss.NewStyle().Background(lipgloss.Color("#5A5A8A"))
	topGreen  = lipgloss.NewStyle().Foreground(tuiGreen)
	topRed    = lipgloss.NewStyle().Foreground(tuiRed)
	topYellow = lipgloss.NewStyle().Foreground(tuiYellow)
	topCyan   = lipgloss.NewStyle().Foreground(tuiCyan)
	topMuted  = lipgloss.NewStyle().Foreground(tuiMuted)
	topBtn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000")).Background(tuiPurple).Padding(0, 1)
	topBtnOff = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAA")).Background(tuiDkPurple).Padding(0, 1)
	topBtnRed = lipgloss.NewStyle().Bold(true).Foreground(tuiWhite).Background(tuiRed).Padding(0, 1)
	topPanel  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(tuiPurple).Padding(0, 1)
	topGraph  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(tuiPurple).Padding(1, 1)
)

type topTickMsg time.Time

// TopModel is the bandwidth top TUI with graphs.
type TopModel struct {
	width, height    int
	mode             string
	period           string
	sortBy           string
	selected         int
	entries          []topEntry
	totalRX, totalTX float64
	avgRX, avgTX     float64
	p95RX, p95TX     float64
	peakRX, peakTX   float64
	lastRefresh      time.Time
	quitting, help   bool

	// Graph history: up to 120 data points (2 min at 1s refresh, or 4 min at 2s)
	rxHistory  map[string][]float64
	txHistory  map[string][]float64
	maxHistory int
}

type topEntry struct {
	Name      string
	Type      string
	PID       int
	RXMbps    float64
	TXMbps    float64
	TotalMbps float64
}

type topRefreshMsg struct {
	entries          []topEntry
	totalRX, totalTX float64
	avgRX, avgTX     float64
	p95RX, p95TX     float64
	peakRX, peakTX   float64
}

func NewTopModel() TopModel {
	return TopModel{
		mode: "all", period: "5m", sortBy: "total", help: true,
		lastRefresh: time.Now(),
		rxHistory:   make(map[string][]float64),
		txHistory:   make(map[string][]float64),
		maxHistory:  80,
	}
}

func (m TopModel) Init() tea.Cmd {
	return tea.Batch(m.topRefreshCmd(), topTickCmd(), tea.EnableMouseCellMotion)
}

func topTickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg { return topTickMsg(t) })
}

func (m TopModel) topRefreshCmd() tea.Cmd {
	return func() tea.Msg {
		entries := topCollect(m.mode)
		totalRX, totalTX := 0.0, 0.0
		allRX, allTX := []float64{}, []float64{}
		for _, e := range entries {
			totalRX += e.RXMbps
			totalTX += e.TXMbps
			allRX = append(allRX, e.RXMbps)
			allTX = append(allTX, e.TXMbps)
		}
		sort.Slice(entries, func(i, j int) bool {
			switch m.sortBy {
			case "rx":
				return entries[i].RXMbps > entries[j].RXMbps
			case "tx":
				return entries[i].TXMbps > entries[j].TXMbps
			default:
				return entries[i].TotalMbps > entries[j].TotalMbps
			}
		})
		return topRefreshMsg{
			entries: entries, totalRX: totalRX, totalTX: totalTX,
			avgRX: topAvg(allRX), avgTX: topAvg(allTX),
			p95RX: topPercentile(allRX, 95), p95TX: topPercentile(allTX, 95),
			peakRX: topMax(allRX), peakTX: topMax(allTX),
		}
	}
}

func (m TopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
		bottomY := m.height - 2
		if msg.Y == bottomY {
			switch {
			case msg.X < 20:
				if m.mode == "all" {
					m.mode = "docker"
				} else {
					m.mode = "all"
				}
				m.selected = 0
				return m, m.topRefreshCmd()
			case msg.X >= 20 && msg.X < 35:
				switch m.sortBy {
				case "total":
					m.sortBy = "rx"
				case "rx":
					m.sortBy = "tx"
				case "tx":
					m.sortBy = "total"
				}
				return m, m.topRefreshCmd()
			case msg.X >= 35 && msg.X < 44:
				m.period = "1m"
				return m, m.topRefreshCmd()
			case msg.X >= 44 && msg.X < 53:
				m.period = "5m"
				return m, m.topRefreshCmd()
			case msg.X >= 53 && msg.X < 62:
				m.period = "1h"
				return m, m.topRefreshCmd()
			case msg.X >= 62 && msg.X < 71:
				m.period = "24h"
				return m, m.topRefreshCmd()
			case msg.X >= 71:
				m.quitting = true
				return m, tea.Quit
			}
		}
		rowY := msg.Y - 4
		if rowY >= 0 && rowY < len(m.entries) {
			m.selected = rowY
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "?":
			m.help = !m.help
			return m, nil
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.selected < len(m.entries)-1 {
				m.selected++
			}
			return m, nil
		case "left":
			if m.selected == 0 {
				return m, nil
			}
			// Show graph toggle? For now just move selection
			return m, nil
		case "right":
			return m, nil
		case "tab":
			if m.mode == "all" {
				m.mode = "docker"
			} else {
				m.mode = "all"
			}
			m.selected = 0
			return m, m.topRefreshCmd()
		case "s":
			switch m.sortBy {
			case "total":
				m.sortBy = "rx"
			case "rx":
				m.sortBy = "tx"
			case "tx":
				m.sortBy = "total"
			}
			return m, m.topRefreshCmd()
		case "1":
			m.period = "1m"
			return m, m.topRefreshCmd()
		case "5":
			m.period = "5m"
			return m, m.topRefreshCmd()
		case "h":
			m.period = "1h"
			return m, m.topRefreshCmd()
		case "d":
			m.period = "24h"
			return m, m.topRefreshCmd()
		}

	case topTickMsg:
		m.lastRefresh = time.Now()
		return m, tea.Batch(m.topRefreshCmd(), topTickCmd())

	case topRefreshMsg:
		m.entries = msg.entries
		m.totalRX, m.totalTX = msg.totalRX, msg.totalTX
		m.avgRX, m.avgTX = msg.avgRX, msg.avgTX
		m.p95RX, m.p95TX = msg.p95RX, msg.p95TX
		m.peakRX, m.peakTX = msg.peakRX, msg.peakTX

		// Update history
		for _, e := range m.entries {
			m.rxHistory[e.Name] = append(m.rxHistory[e.Name], e.RXMbps)
			m.txHistory[e.Name] = append(m.txHistory[e.Name], e.TXMbps)
			if len(m.rxHistory[e.Name]) > m.maxHistory {
				m.rxHistory[e.Name] = m.rxHistory[e.Name][len(m.rxHistory[e.Name])-m.maxHistory:]
			}
			if len(m.txHistory[e.Name]) > m.maxHistory {
				m.txHistory[e.Name] = m.txHistory[e.Name][len(m.txHistory[e.Name])-m.maxHistory:]
			}
		}
		return m, nil
	}
	return m, nil
}

func (m TopModel) View() string {
	if m.quitting {
		return "\n"
	}
	var sb strings.Builder

	// Title
	modeLabel := "🐳 Docker Only"
	if m.mode == "all" {
		modeLabel = "🌐 All Interfaces + Docker"
	}
	sb.WriteString(topBar.Render(fmt.Sprintf("⚡ Bandwidth Top — %s  [%d entries]", modeLabel, len(m.entries))))
	sb.WriteString("\n\n")

	// Total
	total := m.totalRX + m.totalTX
	gauge := topBuildGauge(total, float64(len(m.entries))*200+1, 30)
	sb.WriteString(fmt.Sprintf(" Total: %s↓  %s↑  [%s]\n\n", humanMbps(m.totalRX), humanMbps(m.totalTX), gauge))

	// Split view: list on left, graph on right
	listWidth := 45
	if m.width > 100 {
		listWidth = 50
	}
	if m.width > 130 {
		listWidth = 55
	}
	graphWidth := m.width - listWidth - 3
	if graphWidth < 30 {
		graphWidth = 30
	}

	// Left panel: list
	hdr := fmt.Sprintf("%-4s %-*s %5s %5s %5s", "#", listWidth-22, "NAME", "RX", "TX", "TOT")
	leftContent := topHeader.Render(" "+hdr) + "\n"

	maxRows := m.height - 14
	if maxRows < 3 {
		maxRows = 10
	}
	for i, e := range m.entries {
		if i >= maxRows {
			break
		}
		c := topCyan
		if e.TotalMbps > 100 {
			c = topYellow
		}
		if e.TotalMbps > 500 {
			c = topRed
		}
		nameW := listWidth - 22
		row := fmt.Sprintf(" %-4d %-*s %5s %5s %5s",
			i+1, nameW, topTrunc(e.Name, nameW), humanMbpsShort(e.RXMbps), humanMbpsShort(e.TXMbps), humanMbpsShort(e.TotalMbps))
		if i == m.selected {
			row = topSel.Render(row)
		} else {
			row = c.Render(row)
		}
		leftContent += row + "\n"
	}

	// Fill remaining rows
	for i := len(m.entries); i < maxRows; i++ {
		leftContent += "\n"
	}

	// Right panel: graph for selected entry
	rightContent := ""
	if m.selected >= 0 && m.selected < len(m.entries) {
		sel := m.entries[m.selected]
		rxData := m.rxHistory[sel.Name]
		txData := m.txHistory[sel.Name]

		graphH := maxRows - 2
		if graphH < 5 {
			graphH = 5
		}

		rightContent = topGraph.Width(graphWidth).Render(
			topBar.Render(fmt.Sprintf(" %s ", topTrunc(sel.Name, graphWidth-4))) + "\n" +
				fmt.Sprintf(" ↓%s  ↑%s  Total: %s\n", humanMbps(sel.RXMbps), humanMbps(sel.TXMbps), humanMbps(sel.TotalMbps)) +
				drawSparkGraph(rxData, txData, graphWidth-4, graphH) + "\n" +
				topGreen.Render("── RX (inbound)") + "  " + topRed.Render("── TX (outbound)"),
		)
	} else {
		rightContent = topGraph.Width(graphWidth).Render(topMuted.Render("Select an entry to see graph"))
	}

	// Render left + right side by side
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")
	for i := 0; i < maxRows+2; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		// Pad left to listWidth
		lp := l + strings.Repeat(" ", max(0, listWidth-lipglossWidth(l)))
		sb.WriteString(lp + " " + r + "\n")
	}

	// Stats
	sb.WriteString("\n")
	stats := fmt.Sprintf("Stats [%s] │ Avg: %s↓ %s↑ │ 95th: %s↓ %s↑ │ Peak: %s↓ %s↑",
		m.period,
		humanMbps(m.avgRX), humanMbps(m.avgTX),
		humanMbps(m.p95RX), humanMbps(m.p95TX),
		humanMbps(m.peakRX), humanMbps(m.peakTX))
	sb.WriteString(topPanel.Render(stats))
	sb.WriteString("\n\n")

	// Button bar
	modeBtn := topBtn.Render("[Tab] All/Docker")
	sortBtn := topBtnOff.Render("[s] Sort")
	p1 := topBtnOff.Render("[1]1m")
	p5 := topBtnOff.Render("[5]5m")
	ph := topBtnOff.Render("[h]1h")
	pd := topBtnOff.Render("[d]24h")
	qBtn := topBtnRed.Render("[q] Quit")
	switch m.period {
	case "1m":
		p1 = topBtn.Render("[1]1m")
	case "5m":
		p5 = topBtn.Render("[5]5m")
	case "1h":
		ph = topBtn.Render("[h]1h")
	case "24h":
		pd = topBtn.Render("[d]24h")
	}
	sb.WriteString(modeBtn + sortBtn + p1 + p5 + ph + pd + qBtn)
	sb.WriteString("\n" + topMuted.Render("↑↓ choose  Tab toggle  s sort  1/5/h/d period  q quit  ? help"))

	return sb.String()
}

// ─── Sparkline Graph ──────────────────────────────────────────────────────────

func drawSparkGraph(rx, tx []float64, width, height int) string {
	if len(rx) == 0 && len(tx) == 0 {
		return topMuted.Render("collecting data...")
	}
	// Find max for scaling
	maxVal := 0.0
	for _, v := range rx {
		if v > maxVal {
			maxVal = v
		}
	}
	for _, v := range tx {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal < 0.1 {
		maxVal = 1
	}
	maxVal *= 1.1 // 10% headroom

	// Combine data into a single slice for rendering
	combined := make([]float64, max(len(rx), len(tx)))
	copy(combined, rx)

	// Build grid
	grid := make([][]rune, height)
	for y := 0; y < height; y++ {
		grid[y] = make([]rune, width)
		for x := 0; x < width; x++ {
			grid[y][x] = ' '
		}
	}

	// Plot RX (green ░ characters)
	plotLine(grid, rx, maxVal, width, height, '░', tuiGreen)
	// Plot TX (red ░ characters)
	plotLine(grid, tx, maxVal, width, height, '▒', tuiRed)

	// Render
	var sb strings.Builder
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			ch := grid[y][x]
			if ch == '░' {
				sb.WriteString(topGreen.Render(string('▄')))
			} else if ch == '▒' {
				sb.WriteString(topRed.Render(string('▄')))
			} else if ch == '█' {
				// Overlap: show yellow
				sb.WriteString(topYellow.Render(string('▄')))
			} else {
				sb.WriteRune(' ')
			}
		}
		sb.WriteRune('\n')
	}

	// Scale labels
	_ = combined
	sb.WriteString(topMuted.Render(fmt.Sprintf(" %s", humanMbps(maxVal))))
	sb.WriteString(strings.Repeat(" ", width-15))
	sb.WriteString(topMuted.Render(fmt.Sprintf("0 ")))

	return sb.String()
}

func plotLine(grid [][]rune, data []float64, maxVal float64, width, height int, ch rune, color lipgloss.Color) {
	if len(data) == 0 {
		return
	}
	step := float64(len(data)) / float64(width-1)
	for x := 0; x < width; x++ {
		idx := int(float64(x) * step)
		if idx >= len(data) {
			idx = len(data) - 1
		}
		val := data[idx]
		y := height - 1 - int(math.Round(val/maxVal*float64(height-1)))
		if y < 0 {
			y = 0
		}
		if y >= height {
			y = height - 1
		}
		existing := grid[y][x]
		if existing == ' ' {
			grid[y][x] = ch
		} else if existing != ch {
			grid[y][x] = '█' // overlap marker
		}
	}
}

// ─── Human Readable ───────────────────────────────────────────────────────────

func humanMbps(mbps float64) string {
	if mbps >= 1000 {
		return fmt.Sprintf("%.2fG", mbps/1000)
	}
	if mbps >= 1 {
		return fmt.Sprintf("%.1fM", mbps)
	}
	if mbps >= 0.001 {
		return fmt.Sprintf("%.0fK", mbps*1000)
	}
	return "0"
}

func humanMbpsShort(mbps float64) string {
	if mbps >= 1000 {
		return fmt.Sprintf("%.1fG", mbps/1000)
	}
	if mbps >= 10 {
		return fmt.Sprintf("%.0fM", mbps)
	}
	if mbps >= 1 {
		return fmt.Sprintf("%.1fM", mbps)
	}
	if mbps >= 0.01 {
		return fmt.Sprintf("%.0fK", mbps*1000)
	}
	return "0"
}

// ─── Data Collection ──────────────────────────────────────────────────────────

func topCollect(mode string) []topEntry {
	var entries []topEntry
	if mode == "all" || mode == "docker" {
		entries = append(entries, topDockerStats()...)
	}
	if mode == "all" {
		entries = append(entries, topIfaceStats()...)
	}
	return entries
}

func topDockerStats() []topEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.NetIO}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	var entries []topEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		rx, tx := topParseNetIO(parts[1])
		entries = append(entries, topEntry{Name: parts[0], Type: "docker", RXMbps: rx, TXMbps: tx, TotalMbps: rx + tx})
	}
	return entries
}

func topIfaceStats() []topEntry {
	out, _ := exec.Command("ls", "/sys/class/net").CombinedOutput()
	var entries []topEntry
	for _, iface := range strings.Fields(string(out)) {
		if iface == "lo" {
			continue
		}
		rx, _ := topReadUint(fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", iface))
		tx, _ := topReadUint(fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", iface))
		rxM := float64(rx) / 1e6
		txM := float64(tx) / 1e6
		if rxM == 0 && txM == 0 {
			continue
		}
		entries = append(entries, topEntry{Name: iface, Type: "iface", RXMbps: rxM, TXMbps: txM, TotalMbps: rxM + txM})
	}
	return entries
}

func topParseNetIO(s string) (float64, float64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 {
		return 0, 0
	}
	return topParseSize(strings.TrimSpace(parts[0])), topParseSize(strings.TrimSpace(parts[1]))
}

func topParseSize(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "0B" || s == "" {
		return 0
	}
	var v float64
	var u string
	fmt.Sscanf(s, "%f%s", &v, &u)
	switch strings.ToUpper(u) {
	case "B":
		return v / 1e6
	case "KB", "KIB":
		return v / 1000
	case "MB", "MIB":
		return v
	case "GB", "GIB":
		return v * 1000
	case "TB":
		return v * 1e6
	default:
		return v / 1e6
	}
}

func topReadUint(path string) (uint64, error) {
	out, err := exec.Command("cat", path).CombinedOutput()
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}

func topAvg(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func topPercentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	idx := int(float64(len(v)-1) * p / 100.0)
	if idx >= len(v) {
		idx = len(v) - 1
	}
	return v[idx]
}

func topMax(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func topBuildGauge(val, max float64, w int) string {
	r := val / max
	if r > 1 {
		r = 1
	}
	f := int(r * float64(w))
	var b strings.Builder
	for i := 0; i < w; i++ {
		if i < f {
			if r > 0.8 {
				b.WriteString(topRed.Render("█"))
			} else if r > 0.5 {
				b.WriteString(topYellow.Render("█"))
			} else {
				b.WriteString(topGreen.Render("█"))
			}
		} else {
			b.WriteString(topMuted.Render("░"))
		}
	}
	return b.String()
}

func topTrunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func lipglossWidth(s string) int {
	// Approximate width by stripping ANSI codes
	clean := lipgloss.NewStyle().Render(s)
	// Simple: count runes ignoring ANSI
	count := 0
	inEscape := false
	for _, r := range clean {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}
