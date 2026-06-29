package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// GRAPH — Btop-quality braille graph renderer.
// Smooth scrolling, filled RX+TX overlay, auto-scale, grid lines, axis labels.
// ═══════════════════════════════════════════════════════════════════════════════

// Braille dot patterns for 4 levels of density per column.
// Each terminal cell has 2×4 braille dots; we use the 4 vertical dots.
var brailleDots = []rune{' ', '⣀', '⣤', '⣶', '⣿'}

// blockChars for simple bar charts and gauges.
var blockChars = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// ── Graph cell flags ─────────────────────────────────────────────────────────

const (
	cellEmpty = 0
	cellRX    = 1
	cellTX    = 2
	cellBoth  = 3
)

// DrawGraph renders RX and TX history as a filled braille graph.
// rx, tx: time-series values in Mbps.
// w, h: graph dimensions in terminal cells.
func DrawGraph(rx, tx []float64, w, h int) string {
	if w < 10 {
		w = 10
	}
	if h < 3 {
		h = 3
	}

	// Determine scale
	maxV := 1.0
	for _, v := range rx {
		if v > maxV {
			maxV = v
		}
	}
	for _, v := range tx {
		if v > maxV {
			maxV = v
		}
	}
	if maxV < 1 {
		maxV = 1
	}
	maxV *= 1.15 // 15% headroom

	// Canvas: subH rows (4×h braille dots), w columns
	// Each cell tracks whether RX and/or TX fill it.
	subH := h * 4
	canvas := make([][]int, subH)
	for y := range subH {
		canvas[y] = make([]int, w)
	}

	// Fill RX area from bottom (green)
	fillArea(canvas, rx, maxV, w, subH, cellRX)
	// Fill TX area from bottom (red), overlaid
	fillArea(canvas, tx, maxV, w, subH, cellTX)

	// Render
	var sb strings.Builder
	for row := 0; row < h; row++ {
		for x := 0; x < w; x++ {
			// Count lit dots for this cell (4 dots per row)
			level := 0
			hasRX := false
			hasTX := false
			for dot := 0; dot < 4; dot++ {
				subY := row*4 + dot
				switch canvas[subY][x] {
				case cellRX:
					level++
					hasRX = true
				case cellTX:
					level++
					hasTX = true
				case cellBoth:
					level++
					hasRX = true
					hasTX = true
				}
			}

			ch := brailleDots[level]
			switch {
			case hasRX && hasTX:
				sb.WriteString(Orange.Render(string(ch)))
			case hasRX:
				sb.WriteString(Green.Render(string(ch)))
			case hasTX:
				sb.WriteString(Red.Render(string(ch)))
			default:
				sb.WriteString(Dim.Render(string(ch)))
			}
		}
		sb.WriteByte('\n')
	}

	// Grid line (bottom axis)
	sb.WriteString(Dim.Render(strings.Repeat("─", w)))
	sb.WriteByte('\n')

	// Axis labels: max value on left, 0 on right
	maxLabel := humanMbps(maxV)
	zeroLabel := "0"
	pad := w - runeWidth(maxLabel) - runeWidth(zeroLabel)
	if pad < 1 {
		pad = 1
	}
	sb.WriteString(Dim.Render(maxLabel))
	sb.WriteString(strings.Repeat(" ", pad))
	sb.WriteString(Dim.Render(zeroLabel))

	return sb.String()
}

// fillArea fills the canvas from bottom up to data values.
// flag is cellRX, cellTX, or cellBoth for blending.
func fillArea(canvas [][]int, data []float64, maxV float64, w, subH int, flag int) {
	if len(data) == 0 {
		return
	}
	step := float64(len(data)-1) / float64(max(1, w-1))
	for x := 0; x < w; x++ {
		idx := int(float64(x) * step)
		if idx >= len(data) {
			idx = len(data) - 1
		}
		val := data[idx]
		h := int(math.Round(val / maxV * float64(subH-1)))
		if h >= subH {
			h = subH - 1
		}
		if h < 0 {
			h = 0
		}
		for y := 0; y <= h; y++ {
			row := subH - 1 - y
			if canvas[row][x] == cellEmpty {
				canvas[row][x] = flag
			} else if canvas[row][x] != flag {
				canvas[row][x] = cellBoth
			}
		}
	}
}

// DrawSparkline renders a compact single-line sparkline.
func DrawSparkline(data []float64, w int, accent string) string {
	if len(data) == 0 || w < 2 {
		return Dim.Render(strings.Repeat("─", w))
	}

	maxV := 1.0
	for _, v := range data {
		if v > maxV {
			maxV = v
		}
	}
	if maxV < 0.001 {
		maxV = 1
	}

	s := lipgloss.NewStyle().Foreground(lipgloss.Color(accent))
	var sb strings.Builder
	step := float64(len(data)-1) / float64(max(1, w-1))
	for x := 0; x < w; x++ {
		idx := int(float64(x) * step)
		if idx >= len(data) {
			idx = len(data) - 1
		}
		val := data[idx] / maxV
		// Map to block chars 0-8
		b := int(math.Round(val * 8))
		if b < 0 {
			b = 0
		}
		if b > 8 {
			b = 8
		}
		if b == 0 && x > 0 {
			sb.WriteString(Dim.Render(string(blockChars[1])))
		} else {
			sb.WriteString(s.Render(string(blockChars[b])))
		}
	}
	return sb.String()
}

// DrawGauge renders a horizontal gauge bar.
func DrawGauge(val, maxV float64, w int) string {
	pct := val / maxV
	if pct > 1 {
		pct = 1
	}
	if pct < 0 {
		pct = 0
	}
	filled := int(math.Round(pct * float64(w)))
	if filled > w {
		filled = w
	}

	var sb strings.Builder
	for i := 0; i < w; i++ {
		if i < filled {
			frac := float64(i) / float64(w)
			switch {
			case frac > 0.85:
				sb.WriteString(GaugeHigh.Render("█"))
			case frac > 0.6:
				sb.WriteString(GaugeMid.Render("█"))
			default:
				sb.WriteString(GaugeLow.Render("█"))
			}
		} else {
			sb.WriteString(GaugeEmpty.Render("░"))
		}
	}
	return sb.String()
}

// runeWidth counts display width of a string, stripping ANSI escapes.
func runeWidth(s string) int {
	w := 0
	esc := false
	for _, r := range s {
		if r == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if r == 'm' {
				esc = false
			}
			continue
		}
		w++
	}
	return w
}

// ── Shared numeric helpers ───────────────────────────────────────────────────

func humanMbps(v float64) string {
	switch {
	case v >= 10000:
		return fmt.Sprintf("%.1fG", v/1000)
	case v >= 1000:
		return fmt.Sprintf("%.2fG", v/1000)
	case v >= 100:
		return fmt.Sprintf("%.0fM", v)
	case v >= 10:
		return fmt.Sprintf("%.1fM", v)
	case v >= 1:
		return fmt.Sprintf("%.2fM", v)
	case v >= 0.01:
		return fmt.Sprintf("%.0fK", v*1000)
	default:
		return "0"
	}
}

func humanMbpsShort(v float64) string {
	switch {
	case v >= 10000:
		return fmt.Sprintf("%.1fG", v/1000)
	case v >= 1000:
		return fmt.Sprintf("%.2fG", v/1000)
	case v >= 100:
		return fmt.Sprintf("%.0fM", v)
	case v >= 10:
		return fmt.Sprintf("%.1fM", v)
	case v >= 1:
		return fmt.Sprintf("%.2fM", v)
	case v >= 0.01:
		return fmt.Sprintf("%.0fK", v*1000)
	default:
		return "0"
	}
}

// humanBytes formats bytes as human-readable.
func humanBytes(v float64) string {
	switch {
	case v >= 1e12:
		return fmt.Sprintf("%.1fTB", v/1e12)
	case v >= 1e9:
		return fmt.Sprintf("%.1fGB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.1fMB", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fKB", v/1e3)
	default:
		return fmt.Sprintf("%.0fB", v)
	}
}

func avgV(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sorted := make([]float64, len(v))
	copy(sorted, v)
	// Simple insertion sort for small slices
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := int(float64(len(sorted)-1) * p / 100.0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func maxV(v []float64) float64 {
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

func truncName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:max(0, n-1)] + "…"
}
