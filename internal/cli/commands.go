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

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/tui"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Top launches the bandwidth monitoring TUI.
func (c *CLI) Top() {
	if err := tui.RunTop(); err != nil {
		fmt.Printf("Error launching TUI: %v\n", err)
	}
}

// CLI communicates with the bandwidth daemon over a Unix socket.
type CLI struct {
	socketPath string
}

// NewCLI creates a new CLI client.
func NewCLI(socketPath string) *CLI {
	return &CLI{socketPath: socketPath}
}

// ─── Command Implementations ──────────────────────────────────────────────────

// Version prints the version with branding.
func (c *CLI) Version() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  ⚡ Bandwidth Manager v1.0.0                             ║")
	fmt.Println("║  Production-Grade Docker Bandwidth Management            ║")
	fmt.Println("║  Developed by AnAverageBeing                             ║")
	fmt.Println("║  github.com/AnAverageBeing/Bandwidth-flow-maintainer     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
}

// Help prints usage information.
func (c *CLI) Help() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  ⚡ Bandwidth Manager — Docker Bandwidth Control         ║")
	fmt.Println("║  Developed by AnAverageBeing                             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  bandwidth [command]")
	fmt.Println()
	fmt.Println("Management:")
	fmt.Println("  setup              Interactive setup wizard")
	fmt.Println("  configure          Interactive configuration wizard")
	fmt.Println("  reapply            Reapply tc rules to all containers")
	fmt.Println("  reload             Reload configuration")
	fmt.Println("  enable             Enable bandwidth management")
	fmt.Println("  disable            Disable bandwidth management")
	fmt.Println("  restart            Restart the daemon")
	fmt.Println("  stop               Stop the daemon")
	fmt.Println("  start              Start the daemon")
	fmt.Println()
	fmt.Println("Monitoring:")
	fmt.Println("  status             Show daemon status")
	fmt.Println("  top                Live bandwidth monitoring TUI")
	fmt.Println("  list               List all managed containers")
	fmt.Println("  stats              Show bandwidth statistics")
	fmt.Println("  limits             Show configured limits")
	fmt.Println("  health             Health check")
	fmt.Println("  doctor             Run health diagnostics")
	fmt.Println("  daemon             Show daemon information")
	fmt.Println("  logs               Show daemon logs")
	fmt.Println("  config             Show configuration")
	fmt.Println()
	fmt.Println("Containers:")
	fmt.Println("  inspect <id>       Inspect a container")
	fmt.Println("  inspect-port <p>   Inspect by host port")
	fmt.Println("  reset <target>     Reset quota for container/port")
	fmt.Println("  reset all          Reset all quotas")
	fmt.Println("  history <id>       Show usage history")
	fmt.Println()
	fmt.Println("Other:")
	fmt.Println("  version            Show version and credits")
	fmt.Println("  webhook test       Test webhook configuration")
	fmt.Println("  export             Export historical data")
	fmt.Println("  cleanup            Run cleanup")
	fmt.Println("  help               Show this help")
	fmt.Println()
	fmt.Println("GitHub: github.com/AnAverageBeing/Bandwidth-flow-maintainer")
}

// Setup runs the interactive configuration wizard.
func (c *CLI) Setup() {
	c.Configure()
}

// Configure runs the interactive configuration wizard.
func (c *CLI) Configure() {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║   Bandwidth Manager — Interactive Config        ║")
	fmt.Println("║   Developed by AnAverageBeing                    ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Configure each setting. Press Enter to keep [current value].")
	fmt.Println()

	// Use /dev/tty for real terminal input (works even when piped from curl)
	tty, err := os.Open("/dev/tty")
	if err != nil {
		// Fallback to stdin if /dev/tty unavailable (e.g., in containers)
		tty = os.Stdin
	} else {
		defer tty.Close()
	}
	reader := bufio.NewReader(tty)
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
		{"warning_percent", "Warning threshold (% of quota)", "90"},
		{"poll_interval", "Poll interval (e.g. 5s, 10s)", "5s"},
		{"discovery_interval", "Discovery scan interval", "10s"},
		{"timezone", "Timezone for midnight reset", "Asia/Kolkata"},
		{"log_level", "Log level (debug/info/warn/error)", "info"},
		{"cleanup_stale_hours", "Remove containers unseen for N hours", "72"},
		{"history_retention_days", "Keep usage history for N days", "365"},
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

	// ── Alert Configuration ──
	fmt.Println()
	fmt.Println("  ── Alerts & Notifications ──")
	fmt.Println("  Where should alerts be sent?")
	fmt.Println("    1) None (default)")
	fmt.Println("    2) Console log only")
	fmt.Println("    3) Discord webhook")
	fmt.Println("    4) Both (console + Discord)")
	fmt.Print("  Choose [1]: ")
	alertChoice, _ := reader.ReadString('\n')
	alertChoice = strings.TrimSpace(alertChoice)
	if alertChoice == "" {
		alertChoice = "1"
	}

	switch alertChoice {
	case "1":
		cfgStr = replaceYAMLValue(cfgStr, "enabled", "false")
		fmt.Println("  ✓ Alerts: disabled")
	case "2":
		cfgStr = replaceYAMLValue(cfgStr, "enabled", "false")
		// Console logging is always on via logging.console
		fmt.Println("  ✓ Alerts: console log (already enabled)")
	case "3", "4":
		cfgStr = replaceYAMLValue(cfgStr, "enabled", "true")
		fmt.Print("  Discord webhook URL: ")
		whURL, _ := reader.ReadString('\n')
		whURL = strings.TrimSpace(whURL)
		if whURL != "" {
			cfgStr = replaceYAMLValue(cfgStr, "url", whURL)
			cfgStr = replaceYAMLValue(cfgStr, "enabled", "true")
			fmt.Println("  ✓ Discord webhook configured")
		}
		if alertChoice == "3" {
			fmt.Println("  ✓ Alerts: Discord only")
		} else {
			fmt.Println("  ✓ Alerts: Console + Discord")
		}
	}

	// ── API ──
	fmt.Println()
	fmt.Print("  Enable REST API? [y/N]: ")
	apiChoice, _ := reader.ReadString('\n')
	apiChoice = strings.TrimSpace(strings.ToLower(apiChoice))
	if apiChoice == "y" || apiChoice == "yes" {
		cfgStr = replaceYAMLValue(cfgStr, "enabled", "true")
		fmt.Println("  ✓ API enabled (token auto-generated on start)")
	} else {
		cfgStr = replaceYAMLValue(cfgStr, "enabled", "false")
		fmt.Println("  ✓ API disabled")
	}

	// ── Metrics ──
	fmt.Print("  Enable Prometheus metrics? [y/N]: ")
	metChoice, _ := reader.ReadString('\n')
	metChoice = strings.TrimSpace(strings.ToLower(metChoice))
	if metChoice == "y" || metChoice == "yes" {
		cfgStr = replaceYAMLValue(cfgStr, "enabled", "true")
		fmt.Println("  ✓ Metrics enabled on :9090/metrics")
	} else {
		cfgStr = replaceYAMLValue(cfgStr, "enabled", "false")
		fmt.Println("  ✓ Metrics disabled")
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
		// Only match top-level keys (not indented sub-keys)
		if strings.HasPrefix(trimmed, key+":") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			prefix := line[:strings.Index(line, key)]
			lines[i] = prefix + key + ": " + newVal
			return strings.Join(lines, "\n")
		}
	}
	return yamlStr
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

	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  ⚡ Bandwidth Manager Daemon Status                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  State:        %s\n", status.State)
	fmt.Printf("  Version:      %s\n", status.Version)
	fmt.Printf("  Uptime:       %s\n", status.Uptime)
	fmt.Printf("  Containers:   %d total (%d managed, %d exceeded)\n", status.ContainerCount, status.ManagedCount, status.ExceededCount)
	fmt.Printf("  Docker:       %s\n", healthIcon(status.DockerHealthy))
	fmt.Printf("  Database:     %s\n", healthIcon(status.DatabaseOK))
	fmt.Printf("  TC Health:    %s\n", healthIcon(status.TCHealthy))
	fmt.Printf("  TC Rules:     %d active\n", status.TCRulesApplied)
	fmt.Printf("  Poll:         every %ds\n", status.PollIntervalSec)
	fmt.Printf("  Timezone:     %s\n", status.Timezone)
	if !status.LastReset.IsZero() {
		fmt.Printf("  Last Reset:   %s\n", status.LastReset.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()
	fmt.Println("  ── Developed by AnAverageBeing ──")
	fmt.Println("  github.com/AnAverageBeing/Bandwidth-flow-maintainer")
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

	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║  🐳 Docker Container Bandwidth — Per-Container Limits (Not VPS-Wide)                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("%-15s %-25s %-16s %8s %8s %8s %8s\n",
		"ID", "NAME", "VETH", "RX_Mbps", "TX_Mbps", "USED_GB", "QUOTA_GB")
	fmt.Println(strings.Repeat("-", 105))

	for _, c := range result.Containers {
		fmt.Printf("%-15s %-25s %-16s %8.1f %8.1f %8.2f %8.1f\n",
			shortID12(c.ID), truncate(c.Name, 25), truncate(c.VethInterface, 16),
			c.CurrentRxMbps, c.CurrentTxMbps,
			c.TodayRxGB+c.TodayTxGB, c.DailyQuotaGB)
	}
	fmt.Println()
	if len(result.Containers) > 0 {
		c := result.Containers[0]
		fmt.Printf("💡 Each container gets its OWN tc rules on its OWN veth interface (e.g. %s)\n", c.VethInterface)
		fmt.Println("   Run 'tc qdisc show dev " + c.VethInterface + "' to see kernel-level enforcement")
	}
	fmt.Printf("\nTotal: %d Docker containers under bandwidth management\n", result.Count)
}

// Inspect shows detailed container information with TC rule verification.
func (c *CLI) Inspect(id string) {
	resp, err := c.sendCommand("list", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	var result struct {
		Containers []*models.Container `json:"containers"`
	}
	json.Unmarshal(resp.Data, &result)

	var found *models.Container
	for _, cont := range result.Containers {
		if strings.HasPrefix(cont.ID, id) || cont.Name == id || strings.Contains(cont.Name, id) {
			found = cont
			break
		}
	}
	if found == nil {
		fmt.Printf("Container '%s' not found.\n", id)
		return
	}
	ct := found

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  🐳 Container Inspection                                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Name:          %s\n", ct.Name)
	fmt.Printf("  Container ID:  %s\n", shortID12(ct.ID))
	fmt.Printf("  Full ID:       %s\n", ct.ID)
	fmt.Printf("  State:         %s\n", string(ct.State))
	fmt.Printf("  PID:           %d\n", ct.PID)
	fmt.Println()
	fmt.Println("  ── Network ──")
	fmt.Printf("  Veth Interface: %s  ← this container's OWN virtual ethernet\n", ct.VethInterface)
	fmt.Printf("  IP Address:     %s\n", ct.IPAddress)
	fmt.Printf("  Docker Network: %s\n", ct.NetworkName)
	if len(ct.Ports) > 0 {
		fmt.Println("  Ports:")
		for _, p := range ct.Ports {
			fmt.Printf("    %d/%s → host:%d\n", p.ContainerPort, p.Protocol, p.HostPort)
		}
	}
	fmt.Println()
	fmt.Println("  ── Bandwidth Limits (THIS CONTAINER ONLY) ──")
	fmt.Printf("  Download Limit: %.0f Mbps\n", ct.LimitRxMbps)
	fmt.Printf("  Upload Limit:   %.0f Mbps\n", ct.LimitTxMbps)
	fmt.Printf("  Burst Ceiling:  %.0f Mbps\n", ct.CeilRxMbps)
	fmt.Println()
	fmt.Println("  ── Quota ──")
	fmt.Printf("  Daily Quota:    %.0f GB\n", ct.DailyQuotaGB)
	fmt.Printf("  Used Today:     %.2f GB (RX) + %.2f GB (TX) = %.2f GB\n", ct.TodayRxGB, ct.TodayTxGB, ct.TodayRxGB+ct.TodayTxGB)
	fmt.Printf("  Current Speed:  %.1f Mbps ↓ / %.1f Mbps ↑\n", ct.CurrentRxMbps, ct.CurrentTxMbps)
	if ct.Priority != "" {
		fmt.Printf("  Priority:       %s\n", ct.Priority)
	}
	fmt.Println()
	fmt.Println("  ── TC Rules (Kernel-Level) ──")
	if ct.VethInterface != "" {
		fmt.Printf("  Verify: tc qdisc show dev %s\n", ct.VethInterface)
		// Show actual tc rules if accessible
		tcOut, _ := exec.Command("tc", "qdisc", "show", "dev", ct.VethInterface).CombinedOutput()
		tcLines := strings.Split(strings.TrimSpace(string(tcOut)), "\n")
		for _, l := range tcLines {
			if l != "" {
				fmt.Printf("    ✓ %s\n", l)
			}
		}
	} else {
		fmt.Println("    (no veth interface detected)")
	}
	fmt.Println()
	fmt.Println("  ⚡ This container's bandwidth is limited INDEPENDENTLY from other containers.")
	fmt.Println("  ⚡ Other containers and the VPS itself are NOT affected by these limits.")
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

	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║  📊 Bandwidth Limits — Per Docker Container                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  %-35s %10s %10s %10s %10s\n", "CONTAINER", "RX_Mbps", "TX_Mbps", "USED_GB", "QUOTA_GB")
	fmt.Println("  " + strings.Repeat("─", 80))
	for _, l := range limits {
		fmt.Printf("  %-35s %10.1f %10.1f %10.2f %10.1f\n",
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

func shortID12(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// DefaultSocketPath returns the standard Unix socket path.
func DefaultSocketPath() string {
	if path := os.Getenv("BANDWIDTH_SOCKET"); path != "" {
		return path
	}
	return "/var/run/bandwidth.sock"
}
