package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the main TUI application state for the bandwidth monitor.
type Model struct {
	width        int
	height       int
	containers   []TUIContainer
	daemonStatus string
	lastRefresh  time.Time
	selected     int
	searchQuery  string
	sortBy       string
	help         bool
	quitting     bool
}

// TUIContainer is a simplified container view for the TUI.
type TUIContainer struct {
	ID        string
	Name      string
	State     string
	RxMbps    float64
	TxMbps    float64
	LimitMbps float64
	UsedGB    float64
	QuotaGB   float64
	Webhook   bool
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7B68EE")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#4A4A6A"))

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#5A5A8A"))

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00"))

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000"))

	exceededStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4500"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#3A3A5A")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)
)

// NewModel creates the initial TUI model.
func NewModel() Model {
	return Model{
		help:        true,
		sortBy:      "name",
		lastRefresh: time.Now(),
	}
}

// Init is the BubbleTea initialization command.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
	)
}

// tickCmd sends a tick message every 2 seconds for auto-refresh.
func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type tickMsg time.Time

// Update handles messages and user input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			if m.selected < len(m.containers)-1 {
				m.selected++
			}
			return m, nil

		case "r":
			// Force refresh
			m.lastRefresh = time.Now()
			return m, tickCmd()

		case "s":
			// Cycle sort
			switch m.sortBy {
			case "name":
				m.sortBy = "rx"
			case "rx":
				m.sortBy = "tx"
			case "tx":
				m.sortBy = "usage"
			case "usage":
				m.sortBy = "name"
			}
			return m, nil

		case "/":
			// Search mode — simplified
			return m, nil
		}

	case tickMsg:
		// Refresh data
		m.lastRefresh = time.Now()
		return m, tickCmd()
	}

	return m, nil
}

// View renders the TUI.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var sb strings.Builder

	// Title
	sb.WriteString(titleStyle.Render("⚡ Bandwidth Manager TUI"))
	sb.WriteString("\n\n")

	// Status bar
	status := fmt.Sprintf("Containers: %d | Last refresh: %s | Sort: %s",
		len(m.containers), m.lastRefresh.Format("15:04:05"), m.sortBy)
	sb.WriteString(statusBarStyle.Render(status))
	sb.WriteString("\n\n")

	// Header
	header := fmt.Sprintf("  %-12s %-22s %-8s %8s %8s %8s %8s",
		"ID", "NAME", "STATE", "RX_Mbps", "TX_Mbps", "USED_GB", "QUOTA")
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")

	// Container rows
	maxRows := m.height - 10
	if maxRows < 1 {
		maxRows = 20
	}

	for i, c := range m.containers {
		if i >= maxRows {
			break
		}

		row := fmt.Sprintf("  %-12s %-22s %-8s %8.1f %8.1f %8.2f %8.1f",
			c.ID[:min(12, len(c.ID))],
			truncateStr(c.Name, 22),
			c.State,
			c.RxMbps,
			c.TxMbps,
			c.UsedGB,
			c.QuotaGB,
		)

		// Color by state and selection
		styled := lipgloss.NewStyle()
		switch c.State {
		case "running":
			styled = runningStyle
		case "stopped":
			styled = stoppedStyle
		case "exceeded":
			styled = exceededStyle
		}

		if i == m.selected {
			row = selectedStyle.Render(row)
		} else {
			row = styled.Render(row)
		}

		sb.WriteString(row)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Help footer
	if m.help {
		sb.WriteString(helpStyle.Render("↑↓: Navigate  s: Sort  r: Refresh  /: Search  ?: Toggle help  q: Quit"))
	} else {
		sb.WriteString(helpStyle.Render("?: Help"))
	}

	return sb.String()
}

// UpdateContainers refreshes the container list from the daemon.
func (m *Model) UpdateContainers(containers []TUIContainer) {
	m.containers = containers
	if m.selected >= len(m.containers) {
		m.selected = 0
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
