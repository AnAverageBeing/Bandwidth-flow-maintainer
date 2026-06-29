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
)

// ═══════════════════════════════════════════════════════════════════════════════
// DASHBOARD — Btop-quality bandwidth monitoring TUI.
// Header + sidebar (interfaces) + graph panel + stats + bottom bar.
// Keyboard-first navigation with full mouse support.
// ═══════════════════════════════════════════════════════════════════════════════

const maxHist = 200

// ── Messages ─────────────────────────────────────────────────────────────────

// Tick triggers periodic refresh.
type Tick time.Time

// RefreshResult carries collected data back from async refresh.
type RefreshResult struct {
	Entries            []Entry
	TotalRx, TotalTx   float64
	AvgRx, AvgTx       float64
	P95Rx, P95Tx       float64
	PeakRx, PeakTx     float64
	PpsRx, PpsTx       float64
	TotalRxB, TotalTxB uint64
	Drops, Errors      uint64
	ConnCount          int
}

// ── Entry ────────────────────────────────────────────────────────────────────

// Entry is a single row in the interface list.
type Entry struct {
	Name      string
	Type      string // "docker" | "iface"
	RxMbps    float64
	TxMbps    float64
	TotalMbps float64
	RxBytes   uint64
	TxBytes   uint64
	Pps       float64
	Status    string // "up" | "down"
}

// ── Dashboard Model ──────────────────────────────────────────────────────────

// DashboardModel is the main TUI model.
type DashboardModel struct {
	Width, Height int
	Mode          string // "all" | "docker"
	SortBy        string // "total" | "rx" | "tx"
	Period        string // "1m" | "5m" | "1h" | "24h"
	Selected      int
	Scroll        int
	Focus         int // 0=sidebar, 1=graph, 2=stats

	Entries                    []Entry
	TotalRx, TotalTx           float64
	AvgRx, AvgTx               float64
	P95Rx, P95Tx               float64
	PeakRx, PeakTx             float64
	PpsRx, PpsTx               float64
	TotalRxBytes, TotalTxBytes uint64
	Drops, Errors              uint64
	ConnCount                  int

	RxHist  map[string][]float64
	TxHist  map[string][]float64
	PpsHist map[string][]float64

	LastRefresh time.Time
	Quitting    bool
	ShowHelp    bool
	StartedAt   time.Time
}

// NewDashboard creates a new dashboard model.
func NewDashboard() DashboardModel {
	return DashboardModel{
		Mode: "all", SortBy: "total", Period: "5m",
		LastRefresh: time.Now(), StartedAt: time.Now(),
		RxHist:  make(map[string][]float64),
		TxHist:  make(map[string][]float64),
		PpsHist: make(map[string][]float64),
	}
}

// ── Init ─────────────────────────────────────────────────────────────────────

func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		m.collect(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return Tick(t) }),
		tea.EnableMouseCellMotion,
	)
}

// ── Update ───────────────────────────────────────────────────────────────────

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		if m.Width < 80 {
			m.Width = 80
		}
		if m.Height < 20 {
			m.Height = 20
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case Tick:
		m.LastRefresh = time.Now()
		return m, tea.Batch(
			m.collect(),
			tea.Tick(time.Second, func(t time.Time) tea.Msg { return Tick(t) }),
		)

	case RefreshResult:
		m.Entries = msg.Entries
		m.TotalRx, m.TotalTx = msg.TotalRx, msg.TotalTx
		m.AvgRx, m.AvgTx = msg.AvgRx, msg.AvgTx
		m.P95Rx, m.P95Tx = msg.P95Rx, msg.P95Tx
		m.PeakRx, m.PeakTx = msg.PeakRx, msg.PeakTx
		m.PpsRx, m.PpsTx = msg.PpsRx, msg.PpsTx
		m.TotalRxBytes, m.TotalTxBytes = msg.TotalRxB, msg.TotalTxB
		m.Drops, m.Errors = msg.Drops, msg.Errors
		m.ConnCount = msg.ConnCount

		// Update history per entry
		for _, e := range m.Entries {
			m.RxHist[e.Name] = trimHist(append(m.RxHist[e.Name], e.RxMbps))
			m.TxHist[e.Name] = trimHist(append(m.TxHist[e.Name], e.TxMbps))
			m.PpsHist[e.Name] = trimHist(append(m.PpsHist[e.Name], e.Pps))
		}
		if m.Selected >= len(m.Entries) {
			m.Selected = max(0, len(m.Entries)-1)
		}
		return m, nil
	}
	return m, nil
}

// ── Mouse Handler ────────────────────────────────────────────────────────────

func (m DashboardModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	// Scroll wheel
	if msg.Button == tea.MouseButtonWheelUp {
		if m.Selected > 0 {
			m.Selected--
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.Selected < len(m.Entries)-1 {
			m.Selected++
		}
		return m, nil
	}

	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	x, y := msg.X, msg.Y
	sideW := m.Width * 35 / 100
	if sideW < 30 {
		sideW = 30
	}
	headerH := 3 // 2 lines header + separator
	bottomY := m.Height - 2

	// Click in sidebar area
	if x < sideW && y >= headerH && y < bottomY {
		row := y - headerH + m.Scroll
		if row >= 0 && row < len(m.Entries) {
			m.Selected = row
			m.Focus = 0
		}
		return m, nil
	}

	// Click in graph/stats area (right side)
	if x >= sideW && y >= headerH && y < bottomY {
		m.Focus = 1
		return m, nil
	}

	// Click bottom button bar
	if y >= bottomY {
		switch {
		case x < 16:
			// Tab: toggle mode
			if m.Mode == "all" {
				m.Mode = "docker"
			} else {
				m.Mode = "all"
			}
			m.Selected = 0
			m.Scroll = 0
			return m, m.collect()
		case x >= 16 && x < 28:
			// Sort
			switch m.SortBy {
			case "total":
				m.SortBy = "rx"
			case "rx":
				m.SortBy = "tx"
			case "tx":
				m.SortBy = "total"
			}
			return m, m.collect()
		case x >= 28 && x < 38:
			m.Period = "1m"
			return m, m.collect()
		case x >= 38 && x < 48:
			m.Period = "5m"
			return m, m.collect()
		case x >= 48 && x < 58:
			m.Period = "1h"
			return m, m.collect()
		case x >= 58 && x < 70:
			m.Period = "24h"
			return m, m.collect()
		case x >= 70 && x < 80:
			m.ShowHelp = !m.ShowHelp
			return m, nil
		case x >= 80:
			m.Quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// ── Key Handler ──────────────────────────────────────────────────────────────

func (m DashboardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.Quitting = true
		return m, tea.Quit

	case "?":
		m.ShowHelp = !m.ShowHelp
		return m, nil

	case "tab":
		if m.Mode == "all" {
			m.Mode = "docker"
		} else {
			m.Mode = "all"
		}
		m.Selected = 0
		m.Scroll = 0
		return m, m.collect()

	case "up", "k":
		if m.Selected > 0 {
			m.Selected--
		}
		return m, nil

	case "down", "j":
		if m.Selected < len(m.Entries)-1 {
			m.Selected++
		}
		return m, nil

	case "left", "h":
		if m.Focus > 0 {
			m.Focus--
		}
		return m, nil

	case "right", "l":
		if m.Focus < 2 {
			m.Focus++
		}
		return m, nil

	case "g":
		m.Selected = 0
		m.Scroll = 0
		return m, nil

	case "G":
		m.Selected = max(0, len(m.Entries)-1)
		return m, nil

	case "s":
		switch m.SortBy {
		case "total":
			m.SortBy = "rx"
		case "rx":
			m.SortBy = "tx"
		case "tx":
			m.SortBy = "total"
		}
		return m, m.collect()

	case "1":
		m.Period = "1m"
		return m, m.collect()

	case "5":
		m.Period = "5m"
		return m, m.collect()

	case "t":
		if m.Period == "1h" {
			m.Period = "24h"
		} else {
			m.Period = "1h"
		}
		return m, m.collect()

	case "d":
		m.Period = "24h"
		return m, m.collect()
	}

	return m, nil
}

// ── Collect Data ─────────────────────────────────────────────────────────────

func (m DashboardModel) collect() tea.Cmd {
	return func() tea.Msg {
		es := gatherEntries(m.Mode)
		totalRx, totalTx := 0.0, 0.0
		var rxA, txA []float64
		var totalPpsRx, totalPpsTx float64
		var totalRxB, totalTxB uint64
		var conns int

		for _, e := range es {
			totalRx += e.RxMbps
			totalTx += e.TxMbps
			rxA = append(rxA, e.RxMbps)
			txA = append(txA, e.TxMbps)
			totalPpsRx += e.Pps
			totalPpsTx += e.Pps
			totalRxB += e.RxBytes
			totalTxB += e.TxBytes
		}

		// Sort entries
		sort.Slice(es, func(i, j int) bool {
			switch m.SortBy {
			case "rx":
				return es[i].RxMbps > es[j].RxMbps
			case "tx":
				return es[i].TxMbps > es[j].TxMbps
			default:
				return es[i].TotalMbps > es[j].TotalMbps
			}
		})

		return RefreshResult{
			Entries:   es,
			TotalRx:   totalRx,
			TotalTx:   totalTx,
			AvgRx:     avgV(rxA),
			AvgTx:     avgV(txA),
			P95Rx:     percentile(rxA, 95),
			P95Tx:     percentile(txA, 95),
			PeakRx:    maxV(rxA),
			PeakTx:    maxV(txA),
			PpsRx:     totalPpsRx,
			PpsTx:     totalPpsTx,
			TotalRxB:  totalRxB,
			TotalTxB:  totalTxB,
			ConnCount: conns,
		}
	}
}

func trimHist(h []float64) []float64 {
	if len(h) > maxHist {
		return h[len(h)-maxHist:]
	}
	return h
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m DashboardModel) View() string {
	if m.Quitting {
		return ""
	}
	w, h := m.Width, m.Height
	if w < 80 {
		w = 80
	}
	if h < 20 {
		h = 20
	}

	sideW := w * 35 / 100
	if sideW < 28 {
		sideW = 28
	}
	if sideW > 40 {
		sideW = 40
	}
	rightW := w - sideW - 1
	if rightW < 40 {
		rightW = 40
	}

	header := m.renderHeader(w)
	sidebar := m.renderSidebar(sideW, h-7)
	right := m.renderRightPanel(rightW, h-7)

	// Combine sidebar + right panel side by side
	leftLines := strings.Split(sidebar, "\n")
	rightLines := strings.Split(right, "\n")
	rows := max(len(leftLines), len(rightLines))
	var body strings.Builder
	for i := 0; i < rows; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lw := runeWidth(l)
		pad := sideW - lw
		if pad < 0 {
			pad = 0
		}
		body.WriteString(Bg.Render(l + strings.Repeat(" ", pad) + " " + r))
		body.WriteByte('\n')
	}

	bottom := m.renderBottom(w)

	return header + "\n" +
		strings.TrimRight(body.String(), "\n") + "\n" +
		bottom
}

// ── Header ───────────────────────────────────────────────────────────────────

func (m DashboardModel) renderHeader(w int) string {
	modeLabel := "🌐 All Interfaces"
	if m.Mode == "docker" {
		modeLabel = "🐳 Docker Only"
	}

	// Title with accent background
	title := Title("⚡ Bandwidth Monitor")
	modePart := Subtext.Render("  " + modeLabel + "  ")
	timePart := Subtext.Render(m.LastRefresh.Format("15:04:05") + "  ")
	runtime := Subtext.Render("⏱ " + time.Since(m.StartedAt).Round(time.Second).String())

	mid := w - runeWidth(title) - runeWidth(modePart) - runeWidth(timePart) - runeWidth(runtime) - 4
	if mid < 1 {
		mid = 1
	}

	line1 := title + strings.Repeat(" ", mid) + runtime + timePart

	// Speed line
	selName := "—"
	if m.Selected >= 0 && m.Selected < len(m.Entries) {
		selName = truncName(m.Entries[m.Selected].Name, 20)
	}

	speedLine := fmt.Sprintf("  ↓ %s  ↑ %s  ⚡ %s pps  🔗 %d conn  ▌ %s",
		Green.Render(humanMbps(m.TotalRx)),
		Red.Render(humanMbps(m.TotalTx)),
		humanMbpsShort(m.PpsRx+m.PpsTx),
		m.ConnCount,
		Dim.Render(selName),
	)

	// Small sparkline if selected entry has history
	if m.Selected >= 0 && m.Selected < len(m.Entries) {
		e := m.Entries[m.Selected]
		sparkW := max(10, w-runeWidth(speedLine)-4)
		spark := DrawSparkline(m.RxHist[e.Name], sparkW, "#a6e3a1")
		speedLine += "  " + spark
	}

	line2 := Bg.Render(Green.Render(speedLine))

	// Separator
	sep := Dim.Render(strings.Repeat("─", w))

	return line1 + "\n" + line2 + "\n" + sep
}

// ── Sidebar ──────────────────────────────────────────────────────────────────

func (m DashboardModel) renderSidebar(w, maxH int) string {
	visible := maxH - 1
	if visible < 5 {
		visible = 5
	}

	// Scroll management
	if m.Scroll > len(m.Entries)-visible {
		m.Scroll = max(0, len(m.Entries)-visible)
	}
	if m.Selected < m.Scroll {
		m.Scroll = m.Selected
	}
	if m.Selected >= m.Scroll+visible {
		m.Scroll = m.Selected - visible + 1
	}

	var sb strings.Builder

	// Column header
	hdr := TblHeader.Render(fmt.Sprintf("  %-*s %7s %7s %7s", w-26, "Interface", "RX", "TX", "Total"))
	sb.WriteString(Bg.Render(hdr))
	sb.WriteByte('\n')

	for i := m.Scroll; i < min(len(m.Entries), m.Scroll+visible); i++ {
		e := m.Entries[i]

		// Status indicator
		status := "●"
		statusStyle := Green
		if e.Status == "down" {
			status = "○"
			statusStyle = Dim
		}

		// Type badge
		typeBadge := "[I]"
		if e.Type == "docker" {
			typeBadge = "[D]"
		}

		name := truncName(e.Name, w-30)
		row := fmt.Sprintf("  %s%s %-*s %7s %7s %7s",
			statusStyle.Render(status),
			Dim.Render(typeBadge),
			w-30, name,
			humanMbpsShort(e.RxMbps),
			humanMbpsShort(e.TxMbps),
			humanMbpsShort(e.TotalMbps),
		)

		// Color by throughput
		rowStyle := RowNormal
		switch {
		case e.TotalMbps > 500:
			rowStyle = GaugeHigh.Copy().UnsetBackground()
		case e.TotalMbps > 100:
			rowStyle = GaugeMid.Copy().UnsetBackground()
		case e.TotalMbps > 10:
			rowStyle = GaugeLow.Copy().UnsetBackground()
		}

		if i == m.Selected {
			if m.Focus == 0 {
				row = RowFocus.Render(row)
			} else {
				row = RowSelect.Render(row)
			}
		} else {
			row = rowStyle.Render(row)
		}

		sb.WriteString(Bg.Render(row))
		sb.WriteByte('\n')
	}

	// Fill remaining lines
	remaining := visible - min(len(m.Entries), m.Scroll+visible) + m.Scroll - m.Scroll
	_ = remaining
	for i := len(m.Entries) - m.Scroll; i < visible; i++ {
		if i > 0 {
			sb.WriteString(Bg.Render(strings.Repeat(" ", w)))
			sb.WriteByte('\n')
		}
	}

	// Border around sidebar
	style := Box
	if m.Focus == 0 {
		style = BoxFocused
	}
	return style.Width(w).Render(sb.String())
}

// ── Right Panel ──────────────────────────────────────────────────────────────

func (m DashboardModel) renderRightPanel(w, maxH int) string {
	graphH := maxH * 65 / 100
	if graphH < 6 {
		graphH = 6
	}
	statsH := maxH - graphH - 1
	if statsH < 3 {
		statsH = 3
	}

	graph := m.renderGraphSection(w, graphH)
	stats := m.renderStatsSection(w, statsH)

	return graph + "\n" + stats
}

// ── Graph Section ────────────────────────────────────────────────────────────

func (m DashboardModel) renderGraphSection(w, maxH int) string {
	if m.Selected < 0 || m.Selected >= len(m.Entries) {
		content := Muted.Render("\n  Select an interface from the list\n  to view live bandwidth graph")
		style := Box.Width(w)
		return style.Render(content)
	}

	e := m.Entries[m.Selected]
	rxD := m.RxHist[e.Name]
	txD := m.TxHist[e.Name]

	graphH := maxH - 4
	if graphH < 4 {
		graphH = 4
	}
	graphW := w - 6
	if graphW < 20 {
		graphW = 20
	}

	// Title with current values
	title := fmt.Sprintf(" Live Graph: %s  │  ↓ %s  ↑ %s  │  %s pps  │  %s total ",
		truncName(e.Name, 25),
		Green.Render(humanMbps(e.RxMbps)),
		Red.Render(humanMbps(e.TxMbps)),
		humanMbpsShort(e.Pps),
		humanBytes(float64(e.RxBytes+e.TxBytes)),
	)
	titleLine := Accent.Render(title)

	// Graph content
	graphContent := DrawGraph(rxD, txD, graphW, graphH)

	// Legend
	legend := fmt.Sprintf("  %s RX (inbound)    %s TX (outbound)    %s Both",
		Green.Render("──"),
		Red.Render("──"),
		Orange.Render("──"),
	)

	content := titleLine + "\n\n" + graphContent + "\n" + legend

	style := Box
	if m.Focus == 1 {
		style = BoxFocused
	}
	return style.Width(w).Render(content)
}

// ── Stats Section ────────────────────────────────────────────────────────────

func (m DashboardModel) renderStatsSection(w, maxH int) string {
	periodLabel := fmt.Sprintf("Period: %s", m.Period)
	avgLine := fmt.Sprintf("Avg  ↓ %s  ↑ %s",
		Green.Render(humanMbps(m.AvgRx)),
		Red.Render(humanMbps(m.AvgTx)),
	)
	p95Line := fmt.Sprintf("95th ↓ %s  ↑ %s",
		Green.Render(humanMbps(m.P95Rx)),
		Red.Render(humanMbps(m.P95Tx)),
	)
	peakLine := fmt.Sprintf("Peak ↓ %s  ↑ %s",
		Green.Render(humanMbps(m.PeakRx)),
		Red.Render(humanMbps(m.PeakTx)),
	)
	totalLine := fmt.Sprintf("Total: %s transferred  │  Drops: %d  Errors: %d  ⏱ %s",
		humanBytes(float64(m.TotalRxBytes+m.TotalTxBytes)),
		m.Drops, m.Errors,
		time.Since(m.StartedAt).Round(time.Second),
	)

	entriesLine := fmt.Sprintf("Entries: %d  │  Sort: %s  │  %s",
		len(m.Entries), m.SortBy, periodLabel,
	)

	content := fmt.Sprintf("  %s\n  %s  %s\n  %s  %s\n  %s",
		entriesLine, avgLine, p95Line, peakLine, totalLine,
		Dim.Render(""),
	)

	style := Box
	if m.Focus == 2 {
		style = BoxFocused
	}
	return style.Width(w).Render(content)
}

// ── Bottom Bar ───────────────────────────────────────────────────────────────

func (m DashboardModel) renderBottom(w int) string {
	// Button bar
	modeBtn := BtnOn.Render("[Tab] " + m.Mode)
	sortBtn := BtnOff.Render("[s]Sort")
	p1, p5, ph, pd := BtnOff.Render("[1]1m"), BtnOff.Render("[5]5m"), BtnOff.Render("[t]1h"), BtnOff.Render("[d]24h")
	helpBtn := BtnOff.Render("[?]Help")
	quitBtn := BtnRed.Render("[q]Quit")

	switch m.Period {
	case "1m":
		p1 = BtnOn.Render("[1]1m")
	case "5m":
		p5 = BtnOn.Render("[5]5m")
	case "1h":
		ph = BtnOn.Render("[t]1h")
	case "24h":
		pd = BtnOn.Render("[d]24h")
	}

	buttonBar := Bg.Render(modeBtn + sortBtn + p1 + p5 + ph + pd + helpBtn + quitBtn)

	// Help bar
	helpLine := Dim.Render(" ↑↓/jk:Nav  ←→/hl:Panels  g/G:Top/Bot  s:Sort  Tab:Mode  1/5/t/d:Period  ?:Help  q:Quit")

	return buttonBar + "\n" + Bg.Render(helpLine)
}

// ── Data Collection ──────────────────────────────────────────────────────────

func gatherEntries(mode string) []Entry {
	var es []Entry
	if mode == "all" || mode == "docker" {
		es = append(es, dockerEntries()...)
	}
	if mode == "all" {
		es = append(es, ifaceEntries()...)
	}
	return es
}

func dockerEntries() []Entry {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.NetIO}}").CombinedOutput()
	if err != nil {
		return nil
	}
	var es []Entry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		rx, tx := parseNetIO(parts[1])
		es = append(es, Entry{
			Name: parts[0], Type: "docker",
			RxMbps: rx, TxMbps: tx, TotalMbps: rx + tx,
			Status: "up",
		})
	}
	return es
}

func ifaceEntries() []Entry {
	out, _ := exec.Command("ls", "/sys/class/net").CombinedOutput()
	var es []Entry
	for _, iface := range strings.Fields(string(out)) {
		if iface == "lo" {
			continue
		}
		rx, _ := readUint64(fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", iface))
		tx, _ := readUint64(fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", iface))
		rxp, _ := readUint64(fmt.Sprintf("/sys/class/net/%s/statistics/rx_packets", iface))
		txp, _ := readUint64(fmt.Sprintf("/sys/class/net/%s/statistics/tx_packets", iface))

		r := float64(rx) / 1e6
		t := float64(tx) / 1e6
		if r == 0 && t == 0 {
			continue
		}

		status := "up"
		oper, _ := exec.Command("cat", fmt.Sprintf("/sys/class/net/%s/operstate", iface)).CombinedOutput()
		if strings.TrimSpace(string(oper)) != "up" {
			status = "down"
		}

		pps := float64(rxp+txp) / 5.0 // rough estimate per second over 5s
		es = append(es, Entry{
			Name: iface, Type: "iface",
			RxMbps: r, TxMbps: t, TotalMbps: r + t,
			RxBytes: rx, TxBytes: tx, Pps: pps,
			Status: status,
		})
	}
	return es
}

func parseNetIO(s string) (float64, float64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 {
		return 0, 0
	}
	return parseSize(strings.TrimSpace(parts[0])), parseSize(strings.TrimSpace(parts[1]))
}

func parseSize(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" {
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

func readUint64(path string) (uint64, error) {
	out, err := exec.Command("cat", path).CombinedOutput()
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}

// ── Entry Point ──────────────────────────────────────────────────────────────

// RunTop launches the bandwidth monitoring TUI.
func RunTop() error {
	p := tea.NewProgram(
		NewDashboard(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
