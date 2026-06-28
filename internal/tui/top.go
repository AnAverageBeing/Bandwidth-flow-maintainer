package tui

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	topBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7B68EE")).Padding(0, 1)
	topHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#4A4A6A"))
	topSelected = lipgloss.NewStyle().Background(lipgloss.Color("#5A5A8A"))
	topGreen    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	topYellow   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700"))
	topRed      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4500"))
	topCyan     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))
	topMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	topTitleBar = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7B68EE")).Padding(0, 1).Width(70)
	topBtn      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#7B68EE")).Padding(0, 1)
	topBtnMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC")).Background(lipgloss.Color("#3A3A5A")).Padding(0, 1)
	topBtnGreen = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#00FF00")).Padding(0, 1)
	topBtnRed   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#FF0000")).Padding(0, 1)
)

type topTickMsg time.Time

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
	return TopModel{mode: "all", period: "5m", sortBy: "total", help: true, lastRefresh: time.Now()}
}

func (m TopModel) Init() tea.Cmd {
	return tea.Batch(m.topRefreshCmd(), topTickCmd(), tea.EnableMouseCellMotion)
}

func topTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return topTickMsg(t) })
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
		// Bottom button bar (height-2)
		if msg.Y == m.height-2 || msg.Y == m.height-1 {
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
		// Click on entry rows (y >= 4)
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
		modeLabel = "🌐 All Apps"
	}
	sb.WriteString(topTitleBar.Render(fmt.Sprintf("⚡ Bandwidth Top — %s", modeLabel)))
	sb.WriteString("\n\n")

	// Total gauge
	total := m.totalRX + m.totalTX
	bar := topBuildGauge(total, 1000, 40)
	sb.WriteString(fmt.Sprintf("  Total: %s ↓  %s ↑  [%s]\n\n", humanMbps(m.totalRX), humanMbps(m.totalTX), bar))

	// Header
	sb.WriteString(topHeader.Render(fmt.Sprintf("  %-4s %-24s %-8s %10s %10s %10s", "#", "NAME", "TYPE", "RX", "TX", "TOTAL")))
	sb.WriteString("\n")

	// Entries
	maxRows := m.height - 14
	if maxRows < 5 {
		maxRows = 20
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
		row := fmt.Sprintf("  %-4d %-24s %-8s %10s %10s %10s",
			i+1, topTrunc(e.Name, 24), e.Type,
			humanMbps(e.RXMbps), humanMbps(e.TXMbps), humanMbps(e.TotalMbps))
		if i == m.selected {
			row = topSelected.Render(row)
		} else {
			row = c.Render(row)
		}
		sb.WriteString(row + "\n")
	}

	sb.WriteString("\n")

	// Stats panel
	stats := fmt.Sprintf("Stats [%s] │ Avg: %s↓ %s↑ │ 95th: %s↓ %s↑ │ Peak: %s↓ %s↑ │ %d entries",
		m.period,
		humanMbps(m.avgRX), humanMbps(m.avgTX),
		humanMbps(m.p95RX), humanMbps(m.p95TX),
		humanMbps(m.peakRX), humanMbps(m.peakTX),
		len(m.entries))
	sb.WriteString(topBorder.Render(stats))
	sb.WriteString("\n\n")

	// Button bar
	modeBtn := topBtn.Render(" [Tab] All/Docker ")
	sortBtn := topBtnMuted.Render(" [s] Sort ")
	p1Btn := topBtnMuted.Render(" [1] 1m ")
	p5Btn := topBtnMuted.Render(" [5] 5m ")
	phBtn := topBtnMuted.Render(" [h] 1h ")
	pdBtn := topBtnMuted.Render(" [d] 24h ")
	quitBtn := topBtnRed.Render(" [q] Quit ")

	// Highlight current period
	switch m.period {
	case "1m":
		p1Btn = topBtn.Render(" [1] 1m ")
	case "5m":
		p5Btn = topBtn.Render(" [5] 5m ")
	case "1h":
		phBtn = topBtn.Render(" [h] 1h ")
	case "24h":
		pdBtn = topBtn.Render(" [d] 24h ")
	}

	sb.WriteString(modeBtn + sortBtn + p1Btn + p5Btn + phBtn + pdBtn + quitBtn)
	if m.help {
		sb.WriteString("\n" + topMuted.Render("  Click buttons or use keys: ↑↓ Navigate  Tab Toggle  s Sort  1/5/h/d Period  q Quit"))
	}

	return sb.String()
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

func humanBytes(b float64) string {
	if b >= 1e9 {
		return fmt.Sprintf("%.2fGB", b/1e9)
	}
	if b >= 1e6 {
		return fmt.Sprintf("%.1fMB", b/1e6)
	}
	if b >= 1e3 {
		return fmt.Sprintf("%.0fKB", b/1e3)
	}
	return fmt.Sprintf("%.0fB", b)
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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
