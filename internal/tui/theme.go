package tui

import "github.com/charmbracelet/lipgloss"

/* ═══════════════════════════════════════════════════════════════════════════
   Theme — Dark professional palette, accent only where meaningful
   ═══════════════════════════════════════════════════════════════════════════ */

var (
	// Base
	T_Bg      = lipgloss.Color("#0a0e14")
	T_Surface = lipgloss.Color("#12171e")
	T_Border  = lipgloss.Color("#1e2837")
	T_Accent  = lipgloss.Color("#7c3aed")
	T_Accent2 = lipgloss.Color("#6366f1")
	T_Text    = lipgloss.Color("#cdd6f4")
	T_Subtext = lipgloss.Color("#6c7086")
	T_Muted   = lipgloss.Color("#45475a")
	T_Dim     = lipgloss.Color("#313244")

	// Semantic
	T_Green   = lipgloss.Color("#a6e3a1")
	T_Red     = lipgloss.Color("#f38ba8")
	T_Yellow  = lipgloss.Color("#f9e2af")
	T_Cyan    = lipgloss.Color("#89dceb")
	T_Blue    = lipgloss.Color("#89b4fa")
	T_Orange  = lipgloss.Color("#fab387")

	// ── Primitives ───────────────────────────────────────────────────────

	StyBg       = lipgloss.NewStyle().Background(T_Bg)
	StySurface  = lipgloss.NewStyle().Background(T_Surface)
	StyText     = lipgloss.NewStyle().Foreground(T_Text)
	StySubtext  = lipgloss.NewStyle().Foreground(T_Subtext)
	StyMuted    = lipgloss.NewStyle().Foreground(T_Muted)
	StyDim      = lipgloss.NewStyle().Foreground(T_Dim)

	StyBold     = lipgloss.NewStyle().Bold(true).Foreground(T_Text)
	StyAccent   = lipgloss.NewStyle().Bold(true).Foreground(T_Accent)
	StyGreen    = lipgloss.NewStyle().Foreground(T_Green)
	StyRed      = lipgloss.NewStyle().Foreground(T_Red)
	StyYellow   = lipgloss.NewStyle().Foreground(T_Yellow)
	StyCyan     = lipgloss.NewStyle().Foreground(T_Cyan)
	StyBlue     = lipgloss.NewStyle().Foreground(T_Blue)

	// ── Layout Helpers ───────────────────────────────────────────────────

	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(T_Border).
		Padding(0, 1)

	BoxFocused = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(T_Accent).
		Padding(0, 1)

	BoxThick = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(T_Border).
		Padding(0, 1)

	// ── Table ────────────────────────────────────────────────────────────

	TblHeader = lipgloss.NewStyle().
			Foreground(T_Subtext).
			Bold(true).
			Background(T_Surface)

	SelRow = lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1035")).
		Foreground(T_Text)

	SelRowFocused = lipgloss.NewStyle().
			Background(lipgloss.Color("#2d1f5e")).
			Foreground(T_Text)

	// ── Buttons ──────────────────────────────────────────────────────────

	BtnOn  = lipgloss.NewStyle().Foreground(T_Bg).Background(T_Accent).Bold(true).Padding(0, 1)
	BtnOff = lipgloss.NewStyle().Foreground(T_Subtext).Background(T_Surface).Padding(0, 1)
	BtnRed = lipgloss.NewStyle().Foreground(T_Text).Background(T_Red).Bold(true).Padding(0, 1)

	// ── Gauge/Spark ──────────────────────────────────────────────────────

	GaugeLow      = lipgloss.NewStyle().Foreground(T_Green)
	GaugeMid      = lipgloss.NewStyle().Foreground(T_Yellow)
	GaugeHigh     = lipgloss.NewStyle().Foreground(T_Red)
	GaugeEmpty    = lipgloss.NewStyle().Foreground(T_Dim)

	SparkRx = lipgloss.NewStyle().Foreground(T_Green)
	SparkTx = lipgloss.NewStyle().Foreground(T_Red)
)
