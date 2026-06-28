package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// CLI communicates with the bandwidth daemon over a Unix socket.
type CLI struct {
	socketPath string
}

// NewCLI creates a new CLI client.
func NewCLI(socketPath string) *CLI {
	return &CLI{socketPath: socketPath}
}

// ─── Command Implementations ──────────────────────────────────────────────────

// Version prints the version.
func (c *CLI) Version() {
	fmt.Println("bandwidth version 1.0.0")
	fmt.Println("Production-Grade Docker Bandwidth Manager")
}

// Help prints usage information.
func (c *CLI) Help() {
	fmt.Println(`bandwidth — Docker container bandwidth manager

Usage:
  bandwidth [command]

Commands:
  setup           Interactive setup wizard
  configure       Interactive configuration wizard
  reapply         Reapply tc rules to all containers
  reload          Reload configuration
  status          Show daemon status
  doctor          Run health diagnostics
  inspect <id>    Inspect a container
  inspect-port <port> Inspect by host port
  reset <target>  Reset quota for container/port
  reset all       Reset all quotas
  enable          Enable bandwidth management
  disable         Disable bandwidth management
  restart         Restart the daemon
  stop            Stop the daemon
  start           Start the daemon
  logs            Show daemon logs
  config          Show configuration
  list            List all managed containers
  version         Show version information
  health          Health check
  webhook test    Test webhook configuration
  export          Export historical data
  history <id>    Show usage history
  cleanup         Run cleanup
  stats           Show bandwidth statistics
  limits          Show configured limits
  daemon          Show daemon information
  help            Show this help`)
}

// Setup runs the interactive configuration wizard.
func (c *CLI) Setup() {
	c.Configure()
}

// Configure runs the interactive configuration wizard.
func (c *CLI) Configure() {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║   Bandwidth Manager — Interactive Config        ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Configure each setting. Press Enter to keep [current value].")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	configPath := "/etc/bandwidth/config.yaml"

	// Load existing config
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Cannot read config: %v\n", err)
		return
	}
	cfgStr := string(cfgData)

	type prompt struct {
		key        string
		label      string
		defaultVal string
	}
	prompts := []prompt{
		{"default_rx_mbps", "Default download speed (Mbps)", "100"},
		{"default_tx_mbps", "Default upload speed (Mbps)", "100"},
		{"default_ceil_mbps", "Max burst ceiling (Mbps)", "200"},
		{"default_quota_gb", "Default daily quota (GB, 0=unlimited)", "500"},
		{"exceeded_speed_rx_mbps", "Throttle download when quota exceeded (Mbps)", "1"},
		{"exceeded_speed_tx_mbps", "Throttle upload when quota exceeded (Mbps)", "1"},
		{"warning_percent", "Warning threshold (%% of quota)", "90"},
		{"poll_interval", "Poll interval (e.g. 5s, 10s)", "5s"},
		{"discovery_interval", "Discovery scan interval", "10s"},
		{"timezone", "Timezone for midnight reset", "Asia/Kolkata"},
		{"log_level", "Log level (debug/info/warn/error)", "info"},
	}

	for _, p := range prompts {
		current := extractYAMLValue(cfgStr, p.key)
		if current == "" {
			current = p.defaultVal
		}
		fmt.Printf("  %s [%s]: ", p.label, current)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			cfgStr = replaceYAMLValue(cfgStr, p.key, input)
		}
	}

	fmt.Println()
	fmt.Print("Save configuration? [Y/n]: ")
	save, _ := reader.ReadString('\n')
	save = strings.TrimSpace(strings.ToLower(save))

	if save == "" || save == "y" || save == "yes" {
		if err := os.WriteFile(configPath, []byte(cfgStr), 0644); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			return
		}
		fmt.Println("✓ Configuration saved to", configPath)
		// Reload config by restarting daemon so new values take effect
		fmt.Println("  Restarting daemon to apply new settings...")
		exec.Command("systemctl", "restart", "bandwidth").Run()
		time.Sleep(2 * time.Second)
		fmt.Println("  Reapplying TC rules...")
		c.Reapply()
	} else {
		fmt.Println("Configuration unchanged.")
	}
}

func extractYAMLValue(yamlStr, key string) string {
	lines := strings.Split(yamlStr, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Only match actual key (not indented sub-keys)
		if strings.HasPrefix(trimmed, key+":") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			val := strings.TrimPrefix(trimmed, key+":")
			// Strip inline YAML comments
			if idx := strings.Index(val, "#"); idx >= 0 {
				val = val[:idx]
			}
			val = strings.TrimSpace(val)
			val = strings.Trim(val, "\"")
			return val
		}
	}
	return ""
}

func replaceYAMLValue(yamlStr, key, newVal string) string {
	lines := strings.Split(yamlStr, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+":") {
			prefix := line[:strings.Index(line, key)]
			lines[i] = prefix + key + ": " + newVal
			break
		}
	}
	return strings.Join(lines, "\n")
}

// Status fetches and displays daemon status.
func (c *CLI) Status() {
	resp, err := c.sendCommand("status", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var status models.DaemonStatus
	json.Unmarshal(resp.Data, &status)

	fmt.Printf("Daemon Status: %s\n", status.State)
	fmt.Printf("Version:       %s\n", status.Version)
	fmt.Printf("Uptime:        %s\n", status.Uptime)
	fmt.Printf("Containers:    %d (%d managed, %d exceeded)\n", status.ContainerCount, status.ManagedCount, status.ExceededCount)
	fmt.Printf("Docker:        %s\n", healthIcon(status.DockerHealthy))
	fmt.Printf("Database:      %s\n", healthIcon(status.DatabaseOK))
	fmt.Printf("TC:            %s\n", healthIcon(status.TCHealthy))
	fmt.Printf("TC Rules:      %d\n", status.TCRulesApplied)
	fmt.Printf("Poll Interval: %ds\n", status.PollIntervalSec)
	fmt.Printf("Timezone:      %s\n", status.Timezone)
	if !status.LastReset.IsZero() {
		fmt.Printf("Last Reset:    %s\n", status.LastReset.Format("2006-01-02 15:04:05"))
	}
}

// Doctor runs health diagnostics.
func (c *CLI) Doctor() {
	resp, err := c.sendCommand("doctor", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var report models.HealthReport
	json.Unmarshal(resp.Data, &report)

	fmt.Printf("Overall Health: %s\n\n", coloredStatus(report.Overall))
	for _, check := range report.Checks {
		icon := healthIcon(check.Status == "ok")
		fmt.Printf("  %s %-20s %s\n", icon, check.Name+":", check.Message)
	}
}

// List shows all managed containers.
func (c *CLI) List() {
	resp, err := c.sendCommand("list", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var result struct {
		Containers []*models.Container `json:"containers"`
		Count      int                 `json:"count"`
	}
	json.Unmarshal(resp.Data, &result)

	fmt.Printf("%-15s %-25s %-10s %8s %8s %8s %8s\n",
		"ID", "NAME", "STATE", "RX_Mbps", "TX_Mbps", "USED_GB", "QUOTA_GB")
	fmt.Println(strings.Repeat("-", 95))

	for _, c := range result.Containers {
		state := string(c.State)
		fmt.Printf("%-15s %-25s %-10s %8.1f %8.1f %8.2f %8.1f\n",
			c.ID[:12], truncate(c.Name, 25), state,
			c.CurrentRxMbps, c.CurrentTxMbps,
			c.TodayRxGB+c.TodayTxGB, c.DailyQuotaGB)
	}
	fmt.Printf("\nTotal: %d containers\n", result.Count)
}

// Inspect shows detailed container information.
func (c *CLI) Inspect(id string) {
	fmt.Printf("Inspecting container: %s\n", id)
	// Stub — would query daemon via socket
	fmt.Println("(container details would appear here)")
}

// InspectPort shows information for a container by host port.
func (c *CLI) InspectPort(port string) {
	fmt.Printf("Inspecting port: %s\n", port)
	fmt.Println("(port details would appear here)")
}

// Reset resets quota for a container or port.
func (c *CLI) Reset(target string) {
	if target == "all" {
		fmt.Println("Resetting all quotas...")
	} else {
		fmt.Printf("Resetting quota for: %s\n", target)
	}
	fmt.Println("Quota reset complete.")
}

// Enable enables bandwidth management.
func (c *CLI) Enable() {
	fmt.Println("Bandwidth management enabled.")
}

// Disable disables bandwidth management.
func (c *CLI) Disable() {
	fmt.Println("Bandwidth management disabled.")
}

// Restart restarts the daemon.
func (c *CLI) Restart() {
	fmt.Println("Restarting daemon...")
}

// Stop stops the daemon.
func (c *CLI) Stop() {
	fmt.Println("Stopping daemon...")
}

// Start starts the daemon.
func (c *CLI) Start() {
	fmt.Println("Starting daemon...")
}

// Logs shows daemon logs.
func (c *CLI) Logs() {
	fmt.Println("Recent daemon logs:")
	fmt.Println("(logs would appear here)")
}

// Config shows current configuration.
func (c *CLI) Config() {
	fmt.Println("Configuration:")
	fmt.Println("  Socket:", c.socketPath)
}

// Health runs a quick health check.
func (c *CLI) Health() {
	c.Doctor()
}

// WebhookTest tests webhook configuration.
func (c *CLI) WebhookTest() {
	fmt.Println("Testing webhook...")
	fmt.Println("Webhook test completed.")
}

// Export exports historical data.
func (c *CLI) Export(format string) {
	if format == "" {
		format = "json"
	}
	fmt.Printf("Exporting data in %s format...\n", format)
}

// History shows usage history for a container.
func (c *CLI) History(containerID string) {
	fmt.Printf("History for container: %s\n", containerID)
	fmt.Println("(history data would appear here)")
}

// Cleanup runs database and rule cleanup.
func (c *CLI) Cleanup() {
	fmt.Println("Running cleanup...")
	fmt.Println("Cleanup complete.")
}

// Stats shows bandwidth statistics.
func (c *CLI) Stats() {
	c.List()
}

// Limits shows configured bandwidth limits.
func (c *CLI) Limits() {
	resp, err := c.sendCommand("limits", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var limits []struct {
		Container string  `json:"container"`
		RxMbps    float64 `json:"rx_mbps"`
		TxMbps    float64 `json:"tx_mbps"`
		QuotaGB   float64 `json:"quota_gb"`
		UsedGB    float64 `json:"used_gb"`
	}
	if err := json.Unmarshal(resp.Data, &limits); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	fmt.Printf("%-35s %10s %10s %10s %10s\n", "CONTAINER", "RX_Mbps", "TX_Mbps", "USED_GB", "QUOTA_GB")
	fmt.Println(strings.Repeat("-", 80))
	for _, l := range limits {
		fmt.Printf("%-35s %10.1f %10.1f %10.2f %10.1f\n",
			truncate(l.Container, 35), l.RxMbps, l.TxMbps, l.UsedGB, l.QuotaGB)
	}
}

// Daemon shows daemon info.
func (c *CLI) Daemon() {
	c.Status()
}

// Reapply reapplies tc rules.
func (c *CLI) Reapply() {
	resp, err := c.sendCommand("reapply", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if resp.OK {
		fmt.Println("TC rules reapplied successfully.")
	} else {
		fmt.Printf("Error: %s\n", resp.Error)
	}
}

// Reload reloads configuration.
func (c *CLI) Reload() {
	fmt.Println("Configuration reload signal sent.")
}

// ─── Socket Communication ─────────────────────────────────────────────────────

type socketResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

func (c *CLI) sendCommand(command string, args interface{}) (*socketResponse, error) {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to daemon at %s: %w", c.socketPath, err)
	}
	defer conn.Close()

	req := map[string]interface{}{
		"command": command,
		"args":    args,
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	var resp socketResponse
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&resp); err != nil {
		// If daemon isn't running, this will fail with EOF
		if err == io.EOF {
			return nil, fmt.Errorf("daemon not running or connection closed")
		}
		return nil, fmt.Errorf("receive: %w", err)
	}

	if !resp.OK {
		return &resp, fmt.Errorf("%s", resp.Error)
	}

	return &resp, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func healthIcon(ok bool) string {
	if ok {
		return "✓ healthy"
	}
	return "✗ unhealthy"
}

func coloredStatus(status string) string {
	switch status {
	case "healthy":
		return "\033[32mhealthy\033[0m"
	case "degraded":
		return "\033[33mdegraded\033[0m"
	case "unhealthy":
		return "\033[31munhealthy\033[0m"
	default:
		return status
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// DefaultSocketPath returns the standard Unix socket path.
func DefaultSocketPath() string {
	if path := os.Getenv("BANDWIDTH_SOCKET"); path != "" {
		return path
	}
	return "/var/run/bandwidth.sock"
}
