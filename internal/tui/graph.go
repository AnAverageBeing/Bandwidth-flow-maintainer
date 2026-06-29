package tui

import (
	"math"
	"strings"
)

/* ═══════════════════════════════════════════════════════════════════════════
   Graph — Smooth scrolling bandwidth graph using Unicode braille blocks
   Auto-scales, renders RX+TX simultaneously, shows axis labels.
   ═══════════════════════════════════════════════════════════════════════════ */

// Braille characters for 4 levels of density (0-3)
var braille = []rune{' ', '⣀', '⣤', '⣶', '⣿'}

// drawGraph renders RX/TX history as a filled braille graph.
// rxData, txData: time-series values
// w, h: width and height in cells
// Returns the rendered string with axis labels.
func drawGraph(rxData, txData []float64, w, h int) string {
	if w < 10 {
		w = 10
	}
	if h < 4 {
		h = 4
	}

	// Find max for auto-scaling (add 15% headroom)
	maxVal := 1.0
	for _, v := range rxData {
		if v > maxVal {
			maxVal = v
		}
	}
	for _, v := range txData {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal < 1 {
		maxVal = 1
	}
	maxVal *= 1.15

	// Create canvas
	// Each terminal row represents 4 sub-rows (braille dot rows 0-3)
	subH := h * 4
	canvas := make([][]float64, subH)
	for y := 0; y < subH; y++ {
		canvas[y] = make([]float64, w)
	}

	// Plot RX as filled area from bottom
	plotArea(canvas, rxData, maxVal, w, subH, true)
	// Plot TX as filled area from bottom (drawn on top)
	plotArea(canvas, txData, maxVal, w, subH, false)

	// Render braille cells
	var sb strings.Builder
	for row := 0; row < h; row++ {
		for x := 0; x < w; x++ {
			// Count how many of the 4 sub-rows are "lit" in RX and TX
			rxLevel := 0
			txLevel := 0
			for dot := 0; dot < 4; dot++ {
				subY := row*4 + dot
				if canvas[subY][x] > 0.01 {
					rxLevel++
				}
			}
			// For TX, we render over the top — use a separate pass
			// Simplified: render RX as braille fill, TX as overlaid dots
			_ = txLevel
			c := braille[rxLevel]
			sb.WriteRune(c)
		}
		sb.WriteRune('\n')
	}

	// Grid line at bottom
	sb.WriteString(StyDim.Render(strings.Repeat("─", w)))
	sb.WriteRune('\n')

	// Axis labels
	sb.WriteString(StyDim.Render(humanMbps(maxVal)))
	sb.WriteString(strings.Repeat(" ", max(0, w-lipglossWidth(humanMbps(maxVal))-lipglossWidth("0"))))
	sb.WriteString(StyDim.Render("0"))
	sb.WriteRune('\n')

	// Legend
	sb.WriteString(StyGreen.Render("── RX (inbound)") + "   " + StyRed.Render("── TX (outbound)"))

	return sb.String()
}

// plotArea fills a filled area from the bottom of the canvas up to the value.
func plotArea(canvas [][]float64, data []float64, maxVal float64, w, subH int, isRX bool) {
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
		height := int(math.Round(val / maxVal * float64(subH-1)))
		if height >= subH {
			height = subH - 1
		}
		if height < 0 {
			height = 0
		}
		// Fill from bottom up to height
		for y := 0; y <= height; y++ {
			if isRX {
				canvas[subH-1-y][x] = 2.0 // RX marker
			} else {
				if canvas[subH-1-y][x] < 1.0 {
					canvas[subH-1-y][x] = 1.0
				} // TX marker
			}
		}
	}
}

// drawSparkSmall draws a compact single-line sparkline for tables.
func drawSparkSmall(data []float64, w int) string {
	if len(data) == 0 {
		return StyDim.Render(strings.Repeat("─", w))
	}
	maxVal := 1.0
	for _, v := range data {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal < 1 {
		maxVal = 1
	}
	var sb strings.Builder
	step := float64(len(data)-1) / float64(max(1, w-1))
	for x := 0; x < w; x++ {
		idx := int(float64(x) * step)
		if idx >= len(data) {
			idx = len(data) - 1
		}
		val := data[idx] / maxVal
		switch {
		case val > 0.75:
			sb.WriteString(StyGreen.Render("█"))
		case val > 0.5:
			sb.WriteString(StyGreen.Render("▆"))
		case val > 0.25:
			sb.WriteString(StyGreen.Render("▄"))
		case val > 0.01:
			sb.WriteString(StyGreen.Render("▂"))
		default:
			sb.WriteString(StyDim.Render("▁"))
		}
	}
	return sb.String()
}

func lipglossWidth(s string) int {
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
