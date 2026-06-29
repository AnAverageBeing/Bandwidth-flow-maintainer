package tui

import "github.com/charmbracelet/lipgloss"

// ═══════════════════════════════════════════════════════════════════════════════
// THEME — Unified dark theme (Catppuccin Mocha inspired).
// All visual constants and pre-built styles live here.
// ═══════════════════════════════════════════════════════════════════════════════

// ── Color Palette ────────────────────────────────────────────────────────────

const (
	C_BG      = lipgloss.Color("#0a0e14")
	C_SURFACE = lipgloss.Color("#12171e")
	C_BORDER  = lipgloss.Color("#1e2837")
	C_ACCENT  = lipgloss.Color("#7c3aed")
	C_ACCENT2 = lipgloss.Color("#6366f1")
	C_TEXT    = lipgloss.Color("#cdd6f4")
	C_SUBTEXT = lipgloss.Color("#6c7086")
	C_MUTED   = lipgloss.Color("#45475a")
	C_DIM     = lipgloss.Color("#313244")
	C_WHITE   = lipgloss.Color("#ffffff")
	C_GREEN   = lipgloss.Color("#a6e3a1")
	C_RED     = lipgloss.Color("#f38ba8")
	C_YELLOW  = lipgloss.Color("#f9e2af")
	C_CYAN    = lipgloss.Color("#89dceb")
	C_BLUE    = lipgloss.Color("#89b4fa")
	C_ORANGE  = lipgloss.Color("#fab387")
	C_SEL_BG  = lipgloss.Color("#1a1035")
	C_SEL_BG2 = lipgloss.Color("#2d1f5e")
)

// ── Base Styles ──────────────────────────────────────────────────────────────

var (
	Bg      = lipgloss.NewStyle().Background(C_BG).Foreground(C_TEXT)
	Surface = lipgloss.NewStyle().Background(C_SURFACE).Foreground(C_TEXT)
	Text    = lipgloss.NewStyle().Foreground(C_TEXT)
	Subtext = lipgloss.NewStyle().Foreground(C_SUBTEXT)
	Muted   = lipgloss.NewStyle().Foreground(C_MUTED)
	Dim     = lipgloss.NewStyle().Foreground(C_DIM)
	Bold    = lipgloss.NewStyle().Bold(true).Foreground(C_WHITE)
	Accent  = lipgloss.NewStyle().Bold(true).Foreground(C_ACCENT)

	Green  = lipgloss.NewStyle().Foreground(C_GREEN)
	Red    = lipgloss.NewStyle().Foreground(C_RED)
	Yellow = lipgloss.NewStyle().Foreground(C_YELLOW)
	Cyan   = lipgloss.NewStyle().Foreground(C_CYAN)
	Blue   = lipgloss.NewStyle().Foreground(C_BLUE)
	Orange = lipgloss.NewStyle().Foreground(C_ORANGE)
)

// ── Panel / Box Styles ───────────────────────────────────────────────────────

var (
	Box        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(C_BORDER).Padding(0, 1)
	BoxFocused = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(C_ACCENT).Padding(0, 1)
	BoxThick   = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(C_BORDER).Padding(0, 1)
	BoxAccent  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(C_ACCENT).Padding(0, 1)
)

// ── Table Styles ─────────────────────────────────────────────────────────────

var (
	TblHeader = lipgloss.NewStyle().Foreground(C_SUBTEXT).Bold(true).Background(C_SURFACE)
	RowNormal = lipgloss.NewStyle().Foreground(C_TEXT)
	RowSelect = lipgloss.NewStyle().Background(C_SEL_BG).Foreground(C_TEXT)
	RowFocus  = lipgloss.NewStyle().Background(C_SEL_BG2).Foreground(C_WHITE)
)

// ── Button Styles ────────────────────────────────────────────────────────────

var (
	BtnOn  = lipgloss.NewStyle().Foreground(C_BG).Background(C_ACCENT).Bold(true).Padding(0, 1)
	BtnOff = lipgloss.NewStyle().Foreground(C_SUBTEXT).Background(C_SURFACE).Padding(0, 1)
	BtnRed = lipgloss.NewStyle().Foreground(C_TEXT).Background(C_RED).Bold(true).Padding(0, 1)
	BtnDim = lipgloss.NewStyle().Foreground(C_DIM).Background(C_SURFACE).Padding(0, 1)
)

// ── Gauge / Bar Styles ───────────────────────────────────────────────────────

var (
	GaugeLow   = lipgloss.NewStyle().Foreground(C_GREEN)
	GaugeMid   = lipgloss.NewStyle().Foreground(C_YELLOW)
	GaugeHigh  = lipgloss.NewStyle().Foreground(C_RED)
	GaugeEmpty = lipgloss.NewStyle().Foreground(C_DIM)
)

// ── Helper constructors ──────────────────────────────────────────────────────

// Title returns a styled title bar string with accent background.
func Title(s string) string {
	return lipgloss.NewStyle().
		Foreground(C_WHITE).
		Background(C_ACCENT).
		Bold(true).
		Padding(0, 2).
		Render(s)
}

// Label returns a dimmed label.
func Label(s string) string {
	return Dim.Render(s)
}

// Padded returns a style with padding applied.
func Padded(s string, l, r int) string {
	return lipgloss.NewStyle().Padding(0, r, 0, l).Render(s)
}
