package tui

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

/* ═══════════════════════════════════════════════════════════════════════════
   Layout Model — Dashboard with header, sidebar, graph, stats, and help bar.
   ═══════════════════════════════════════════════════════════════════════════ */

const MAX_HIST = 200

type tickMsg time.Time

type LayoutModel struct {
	W, H   int
	Mode   string // "all" | "docker"
	SortBy string // "total" | "rx" | "tx"
	Period string // "1m" | "5m" | "1h" | "24h"
	Tab    int    // 0=list 1=graph 2=connections
	Sel    int
	Scroll int

	Entries                    []entry
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

type entry struct {
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

type refreshData struct {
	Entries            []entry
	TotalRx, TotalTx   float64
	AvgRx, AvgTx       float64
	P95Rx, P95Tx       float64
	PeakRx, PeakTx     float64
	PpsRx, PpsTx       float64
	TotalRxB, TotalTxB uint64
	Drops, Errors      uint64
	ConnCount          int
}

func NewLayoutModel() LayoutModel {
	return LayoutModel{
		Mode: "all", SortBy: "total", Period: "5m",
		LastRefresh: time.Now(), StartedAt: time.Now(),
		RxHist:  make(map[string][]float64),
		TxHist:  make(map[string][]float64),
		PpsHist: make(map[string][]float64),
	}
}

func (m LayoutModel) Init() tea.Cmd {
	return tea.Batch(m.doRefresh(), tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }), tea.EnableMouseCellMotion)
}

func (m LayoutModel) doRefresh() tea.Cmd {
	return func() tea.Msg {
		es := collect(m.Mode)
		trx, ttx := 0.0, 0.0
		rxa, txa := []float64{}, []float64{}
		var ppsRx, ppsTx float64
		var rxB, txB uint64
		for _, e := range es {
			trx += e.RxMbps
			ttx += e.TxMbps
			rxa = append(rxa, e.RxMbps)
			txa = append(txa, e.TxMbps)
			ppsRx += e.Pps
			ppsTx += e.Pps
			rxB += e.RxBytes
			txB += e.TxBytes
		}
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
		return refreshData{
			Entries: es, TotalRx: trx, TotalTx: ttx,
			AvgRx: avg(rxa), AvgTx: avg(txa), P95Rx: pct(rxa, 95), P95Tx: pct(txa, 95),
			PeakRx: maxv(rxa), PeakTx: maxv(txa),
			PpsRx: ppsRx, PpsTx: ppsTx, TotalRxB: rxB, TotalTxB: txB,
		}
	}
}

/* ═══════════════════════════════════════════════════════════════════════════
   Update
   ═══════════════════════════════════════════════════════════════════════════ */

func (m LayoutModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.W, m.H = msg.Width, msg.Height
		return m, nil

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.Sel = max(0, m.Sel-1)
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.Sel = min(len(m.Entries)-1, m.Sel+1)
			return m, nil
		}
		if msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
		// Sidebar: x < sidebarW, y >= headerH+2
		sideW := m.W * 30 / 100
		headerH := 4 // title + speed line
		if msg.Y >= headerH && msg.X < sideW {
			row := msg.Y - headerH + m.Scroll
			if row < len(m.Entries) {
				m.Sel = row
				m.Tab = 0
			}
		}
		if msg.Y >= headerH && msg.X >= sideW {
			m.Tab = 1
		}
		// Bottom bar
		if msg.Y >= m.H-1 {
			switch {
			case msg.X < 16: // Tab
				if m.Mode == "all" {
					m.Mode = "docker"
				} else {
					m.Mode = "all"
				}
				m.Sel = 0
				m.Scroll = 0
				return m, m.doRefresh()
			case msg.X >= 16 && msg.X < 28:
				switch m.SortBy {
				case "total":
					m.SortBy = "rx"
				case "rx":
					m.SortBy = "tx"
				case "tx":
					m.SortBy = "total"
				}
				return m, m.doRefresh()
			case msg.X >= 28 && msg.X < 36:
				m.Period = "1m"
				return m, m.doRefresh()
			case msg.X >= 36 && msg.X < 44:
				m.Period = "5m"
				return m, m.doRefresh()
			case msg.X >= 44 && msg.X < 52:
				m.Period = "1h"
				return m, m.doRefresh()
			case msg.X >= 52 && msg.X < 60:
				m.Period = "24h"
				return m, m.doRefresh()
			case msg.X >= 60:
				m.Quitting = true
				return m, tea.Quit
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.Quitting = true
			return m, tea.Quit
		case "?", "F1":
			m.ShowHelp = !m.ShowHelp
			return m, nil
		case "tab":
			m.Mode = map[string]string{"all": "docker", "docker": "all"}[m.Mode]
			m.Sel, m.Scroll = 0, 0
			return m, m.doRefresh()
		case "up", "k":
			m.Sel = max(0, m.Sel-1)
			return m, nil
		case "down", "j":
			m.Sel = min(len(m.Entries)-1, m.Sel+1)
			return m, nil
		case "left", "h":
			m.Tab = max(0, m.Tab-1)
			return m, nil
		case "right", "l":
			m.Tab = min(2, m.Tab+1)
			return m, nil
		case "g":
			m.Sel, m.Scroll = 0, 0
			return m, nil
		case "G":
			m.Sel = len(m.Entries) - 1
			return m, nil
		case "s":
			cyc := map[string]string{"total": "rx", "rx": "tx", "tx": "total"}
			m.SortBy = cyc[m.SortBy]
			return m, m.doRefresh()
		case "1":
			m.Period = "1m"
			return m, m.doRefresh()
		case "5":
			m.Period = "5m"
			return m, m.doRefresh()
		case "t":
			if m.Period == "1h" {
				m.Period = "24h"
			} else {
				m.Period = "1h"
			}
			return m, m.doRefresh()
		case "d":
			m.Period = "24h"
			return m, m.doRefresh()
		}

	case tickMsg:
		m.LastRefresh = time.Now()
		return m, tea.Batch(m.doRefresh(), tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }))

	case refreshData:
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
		if m.Sel >= len(m.Entries) {
			m.Sel = max(0, len(m.Entries)-1)
		}
		return m, nil
	}
	return m, nil
}

func trimHist(h []float64) []float64 {
	if len(h) > MAX_HIST {
		return h[len(h)-MAX_HIST:]
	}
	return h
}

/* ═══════════════════════════════════════════════════════════════════════════
   View
   ═══════════════════════════════════════════════════════════════════════════ */

func (m LayoutModel) View() string {
	if m.Quitting {
		return ""
	}
	w, h := m.W, m.H
	if w < 100 {
		w = 100
	}
	if h < 24 {
		h = 24
	}

	sideW := w * 30 / 100
	if sideW < 30 {
		sideW = 30
	}
	graphW := w - sideW - 1
	if graphW < 40 {
		graphW = 40
	}

	header := m.renderHeader(w)
	sidebar := m.renderSidebar(sideW, h-8)
	graph := m.renderGraphPanel(graphW, h-8)

	// Combine sidebar + graph side by side
	sLines := strings.Split(sidebar, "\n")
	gLines := strings.Split(graph, "\n")
	n := max(len(sLines), len(gLines))
	var body strings.Builder
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(sLines) {
			l = sLines[i]
		}
		if i < len(gLines) {
			r = gLines[i]
		}
		lp := l + strings.Repeat(" ", max(0, sideW-lipglossWidth(l)))
		body.WriteString(StyBg.Render(lp + " " + r + "\n"))
	}

	stats := m.renderStats(w)
	help := m.renderHelp(w)

	return header + "\n" + strings.TrimRight(body.String(), "\n") + "\n" + stats + "\n" + help
}

/* ═══════════════════════════════════════════════════════════════════════════
   Header — Title, selected interface, refresh, period, current speeds
   ═══════════════════════════════════════════════════════════════════════════ */

func (m LayoutModel) renderHeader(w int) string {
	// Title bar
	mode := "All Interfaces"
	if m.Mode == "docker" {
		mode = "Docker Only"
	}
	selName := "—"
	if m.Sel < len(m.Entries) && m.Sel >= 0 {
		selName = trunc(m.Entries[m.Sel].Name, 20)
	}
	title := fmt.Sprintf("⚡ Bandwidth Monitor › %s › %s", mode, selName)
	left := StyBg.Render(StyAccent.Render(" " + title + " "))
	right := StySubtext.Render(fmt.Sprintf("%s refresh │ %s period │ %s", m.LastRefresh.Format("15:04:05"), m.Period, time.Since(m.StartedAt).Round(time.Second)))
	sp := w - lipglossWidth(left) - lipglossWidth(right) - 2
	if sp < 1 {
		sp = 1
	}
	top := StyBg.Render(left + strings.Repeat(" ", sp) + right)

	// Speed line
	speedLine := fmt.Sprintf("  ↓%s  ↑%s    ⚡%s pps    🔗%d connections",
		humanMbps(m.TotalRx), humanMbps(m.TotalTx), humanShort(m.PpsRx+m.PpsTx), m.ConnCount)
	if m.Sel < len(m.Entries) && m.Sel >= 0 {
		e := m.Entries[m.Sel]
		spark := drawSparkSmall(m.RxHist[e.Name], min(w-60, 80))
		speedLine += "    " + spark
	}
	bottom := StyBg.Render(StyGreen.Render(speedLine))

	return top + "\n" + bottom
}

/* ═══════════════════════════════════════════════════════════════════════════
   Sidebar — Interface list with status, rx, tx, pps, sparkline
   ═══════════════════════════════════════════════════════════════════════════ */

func (m LayoutModel) renderSidebar(w, maxH int) string {
	visible := maxH - 2
	if visible < 3 {
		visible = 3
	}
	if m.Scroll > len(m.Entries)-visible {
		m.Scroll = max(0, len(m.Entries)-visible)
	}
	if m.Sel < m.Scroll {
		m.Scroll = m.Sel
	}
	if m.Sel >= m.Scroll+visible {
		m.Scroll = m.Sel - visible + 1
	}

	var sb strings.Builder
	// Header
	sb.WriteString(StyBg.Render(TblHeader.Render(fmt.Sprintf("  %-*s %6s %6s %6s", w-26, "Interface", "RX", "TX", "PPS"))))
	sb.WriteString("\n")

	for i := m.Scroll; i < min(len(m.Entries), m.Scroll+visible); i++ {
		e := m.Entries[i]
		status := "●"
		statusStyle := StyGreen
		if e.Status == "down" {
			status = "○"
			statusStyle = StyDim
		}

		name := trunc(e.Name, w-28)
		line := fmt.Sprintf("  %s %-*s %6s %6s %6s",
			statusStyle.Render(status), w-26, name,
			humanShort(e.RxMbps), humanShort(e.TxMbps), humanShort(e.Pps))

		if i == m.Sel {
			if m.Tab == 0 {
				line = SelRowFocused.Render(line)
			} else {
				line = SelRow.Render(line)
			}
		}
		sb.WriteString(StyBg.Render(line + "\n"))
	}

	// Fill remaining
	for i := len(m.Entries); i < visible; i++ {
		sb.WriteString(StyBg.Render("\n"))
	}
	return Box.Width(w).Render(sb.String())
}

/* ═══════════════════════════════════════════════════════════════════════════
   Graph Panel — Large graph + current stats
   ═══════════════════════════════════════════════════════════════════════════ */

func (m LayoutModel) renderGraphPanel(w, maxH int) string {
	if m.Sel < 0 || m.Sel >= len(m.Entries) {
		return BoxFocused.Width(w).Render(StyMuted.Render("\n  Select an interface\n  to view graph"))
	}

	e := m.Entries[m.Sel]
	rxD := m.RxHist[e.Name]
	txD := m.TxHist[e.Name]

	graphH := maxH - 6
	if graphH < 8 {
		graphH = 8
	}
	graphW := w - 6
	if graphW < 20 {
		graphW = 20
	}

	// Current stats
	header := fmt.Sprintf("  %s    ↓%s  ↑%s    %s pps    %s total    %s rx    %s tx",
		trunc(e.Name, 15), humanMbps(e.RxMbps), humanMbps(e.TxMbps),
		humanShort(e.Pps), humanBytes(float64(e.RxBytes+e.TxBytes)),
		humanBytes(float64(e.RxBytes)), humanBytes(float64(e.TxBytes)))

	g := drawGraph(rxD, txD, graphW, graphH)

	style := Box
	if m.Tab == 1 {
		style = BoxFocused
	}
	return style.Width(w).Render(header + "\n\n" + g)
}

/* ═══════════════════════════════════════════════════════════════════════════
   Stats Bar
   ═══════════════════════════════════════════════════════════════════════════ */

func (m LayoutModel) renderStats(w int) string {
	line := fmt.Sprintf(
		"Avg: ↓%s ↑%s │ 95th: ↓%s ↑%s │ Peak: ↓%s ↑%s │ Total: %s │ Drops: %d │ Errors: %d │ ⏱ %s",
		humanMbps(m.AvgRx), humanMbps(m.AvgTx),
		humanMbps(m.P95Rx), humanMbps(m.P95Tx),
		humanMbps(m.PeakRx), humanMbps(m.PeakTx),
		humanBytes(float64(m.TotalRxBytes+m.TotalTxBytes)),
		m.Drops, m.Errors,
		time.Since(m.StartedAt).Round(time.Second),
	)
	return BoxThick.Width(w - 2).Render(StyBg.Render(line))
}

/* ═══════════════════════════════════════════════════════════════════════════
   Help Bar
   ═══════════════════════════════════════════════════════════════════════════ */

func (m LayoutModel) renderHelp(w int) string {
	modeBtn := BtnOn.Render("[Tab]Filter")
	sortBtn := BtnOff.Render("[s]Sort")
	p1 := BtnOff.Render("[1]1m")
	p5 := BtnOff.Render("[5]5m")
	ph := BtnOff.Render("[t]1h")
	pd := BtnOff.Render("[d]24h")
	qBtn := BtnRed.Render("[q]Quit")

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

	btns := modeBtn + sortBtn + p1 + p5 + ph + pd + qBtn
	hint := StyDim.Render(" ↑↓j/k:Nav  ←→h/l:Panels  g/G:Top/Bot  s:Sort  Tab:Filter  q:Quit")
	return StyBg.Render(btns + "  " + hint)
}

/* ═══════════════════════════════════════════════════════════════════════════
   Data Collection
   ═══════════════════════════════════════════════════════════════════════════ */

func collect(mode string) []entry {
	var es []entry
	if mode == "all" || mode == "docker" {
		es = append(es, dockerEntries()...)
	}
	if mode == "all" {
		es = append(es, ifaceEntries()...)
	}
	return es
}

func dockerEntries() []entry {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.NetIO}}").CombinedOutput()
	if err != nil {
		return nil
	}
	var es []entry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := strings.Split(line, "\t")
		if len(p) < 2 {
			continue
		}
		rx, tx := parseNet(p[1])
		es = append(es, entry{Name: p[0], Type: "docker", RxMbps: rx, TxMbps: tx, TotalMbps: rx + tx, Status: "up"})
	}
	return es
}

func ifaceEntries() []entry {
	out, _ := exec.Command("ls", "/sys/class/net").CombinedOutput()
	var es []entry
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
		status := "up"
		oper, _ := exec.Command("cat", fmt.Sprintf("/sys/class/net/%s/operstate", iface)).CombinedOutput()
		if strings.TrimSpace(string(oper)) != "up" {
			status = "down"
		}
		if r == 0 && t == 0 && status != "down" {
			r, t = 0.001, 0.001
		} // minimal for display
		es = append(es, entry{Name: iface, Type: "iface", RxMbps: r, TxMbps: t, TotalMbps: r + t, Status: status, RxBytes: rx, TxBytes: tx, Pps: float64(rxp+txp) / (time.Since(time.Now().Add(-time.Second)).Seconds() + 1)})
	}
	return es
}

/* ═══════════════════════════════════════════════════════════════════════════
   CLI entry point
   ═══════════════════════════════════════════════════════════════════════════ */

func RunTop() error {
	p := tea.NewProgram(NewLayoutModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
