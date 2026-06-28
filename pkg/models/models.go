// Package models defines all shared data structures for the bandwidth management system.
// These types are used across the daemon, CLI, TUI, and all internal packages.
package models

import (
	"sync"
	"time"
)

// ─── Container ────────────────────────────────────────────────────────────────

// ContainerState represents the runtime state of a managed container.
type ContainerState string

const (
	StateRunning  ContainerState = "running"
	StateStopped  ContainerState = "stopped"
	StatePaused   ContainerState = "paused"
	StateUnknown  ContainerState = "unknown"
	StateExceeded ContainerState = "exceeded" // quota exceeded
)

// Container represents a Docker container monitored by the system.
type Container struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	PID           int               `json:"pid"`
	State         ContainerState    `json:"state"`
	VethInterface string            `json:"veth_iface"`
	NetworkName   string            `json:"network_name"`
	IPAddress     string            `json:"ip_address"`
	Ports         []PortMapping     `json:"ports"`
	Labels        map[string]string `json:"labels"`
	RestartPolicy string            `json:"restart_policy"`
	FirstSeen     time.Time         `json:"first_seen"`
	LastSeen      time.Time         `json:"last_seen"`

	// Bandwidth limits
	LimitRxMbps float64 `json:"limit_rx_mbps"`
	LimitTxMbps float64 `json:"limit_tx_mbps"`
	CeilRxMbps  float64 `json:"ceil_rx_mbps"`
	CeilTxMbps  float64 `json:"ceil_tx_mbps"`

	// Quota
	DailyQuotaGB    float64 `json:"daily_quota_gb"`
	WarningPercent  float64 `json:"warning_percent"`
	ExceededSpeedRx float64 `json:"exceeded_speed_rx"`
	ExceededSpeedTx float64 `json:"exceeded_speed_tx"`

	// Current usage
	RxBytes       uint64  `json:"rx_bytes"`
	TxBytes       uint64  `json:"tx_bytes"`
	CurrentRxMbps float64 `json:"current_rx_mbps"`
	CurrentTxMbps float64 `json:"current_tx_mbps"`
	TodayRxGB     float64 `json:"today_rx_gb"`
	TodayTxGB     float64 `json:"today_tx_gb"`

	// Metadata
	Priority string `json:"priority"`
	Enabled  bool   `json:"enabled"`
	Webhook  bool   `json:"webhook"`
	History  bool   `json:"history"`
	mu       sync.RWMutex
}

// Lock acquires a write lock on the container.
func (c *Container) Lock() { c.mu.Lock() }

// Unlock releases a write lock on the container.
func (c *Container) Unlock() { c.mu.Unlock() }

// RLock acquires a read lock on the container.
func (c *Container) RLock() { c.mu.RLock() }

// RUnlock releases a read lock on the container.
func (c *Container) RUnlock() { c.mu.RUnlock() }

// TotalLimitMbps returns the combined RX+TX limit in Mbps for display.
func (c *Container) TotalLimitMbps() float64 { return c.LimitRxMbps + c.LimitTxMbps }

// TotalCurrentMbps returns the combined RX+TX current rate.
func (c *Container) TotalCurrentMbps() float64 { return c.CurrentRxMbps + c.CurrentTxMbps }

// TotalTodayGB returns the combined RX+TX usage today.
func (c *Container) TotalTodayGB() float64 { return c.TodayRxGB + c.TodayTxGB }

// RemainingQuotaGB returns how much quota remains today.
func (c *Container) RemainingQuotaGB() float64 {
	used := c.TodayRxGB + c.TodayTxGB
	if c.DailyQuotaGB <= 0 {
		return -1 // unlimited
	}
	remaining := c.DailyQuotaGB - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

// QuotaExceeded returns true if the container has exceeded its daily quota.
func (c *Container) QuotaExceeded() bool {
	if c.DailyQuotaGB <= 0 {
		return false
	}
	return c.TodayRxGB+c.TodayTxGB >= c.DailyQuotaGB
}

// QuotaWarning returns true if the container is within the warning threshold.
func (c *Container) QuotaWarning() bool {
	if c.DailyQuotaGB <= 0 || c.WarningPercent <= 0 {
		return false
	}
	used := c.TodayRxGB + c.TodayTxGB
	return used >= c.DailyQuotaGB*(c.WarningPercent/100.0)
}

// ─── Port Mapping ─────────────────────────────────────────────────────────────

// PortMapping represents a Docker port mapping.
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"` // tcp, udp
	HostIP        string `json:"host_ip"`
}

// ─── Bandwidth Limit ──────────────────────────────────────────────────────────

// BandwidthLimit defines a per-container or per-port bandwidth constraint.
type BandwidthLimit struct {
	ID          int64     `json:"id"`
	ContainerID string    `json:"container_id"`
	Port        int       `json:"port"` // 0 = container-wide
	RxMbps      float64   `json:"rx_mbps"`
	TxMbps      float64   `json:"tx_mbps"`
	BurstMbps   float64   `json:"burst_mbps"`
	LatencyMs   float64   `json:"latency_ms"`
	Priority    string    `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ─── Usage Record ─────────────────────────────────────────────────────────────

// UsageRecord represents a single polling snapshot of bandwidth usage.
type UsageRecord struct {
	ID          int64     `json:"id"`
	ContainerID string    `json:"container_id"`
	Port        int       `json:"port"`
	RxBytes     uint64    `json:"rx_bytes"`
	TxBytes     uint64    `json:"tx_bytes"`
	RxMbps      float64   `json:"rx_mbps"`
	TxMbps      float64   `json:"tx_mbps"`
	Timestamp   time.Time `json:"timestamp"`
}

// ─── History Record ───────────────────────────────────────────────────────────

// HistoryPeriod represents the aggregation period.
type HistoryPeriod string

const (
	PeriodDaily    HistoryPeriod = "daily"
	PeriodWeekly   HistoryPeriod = "weekly"
	PeriodMonthly  HistoryPeriod = "monthly"
	PeriodLifetime HistoryPeriod = "lifetime"
)

// HistoryRecord represents aggregated historical usage.
type HistoryRecord struct {
	ID           int64         `json:"id"`
	ContainerID  string        `json:"container_id"`
	Period       HistoryPeriod `json:"period"`
	PeriodStart  time.Time     `json:"period_start"`
	PeriodEnd    time.Time     `json:"period_end"`
	TotalRxBytes uint64        `json:"total_rx_bytes"`
	TotalTxBytes uint64        `json:"total_tx_bytes"`
	TotalRxGB    float64       `json:"total_rx_gb"`
	TotalTxGB    float64       `json:"total_tx_gb"`
	PeakRxMbps   float64       `json:"peak_rx_mbps"`
	PeakTxMbps   float64       `json:"peak_tx_mbps"`
	AvgRxMbps    float64       `json:"avg_rx_mbps"`
	AvgTxMbps    float64       `json:"avg_tx_mbps"`
	SampleCount  int64         `json:"sample_count"`
	CreatedAt    time.Time     `json:"created_at"`
}

// ─── Event ────────────────────────────────────────────────────────────────────

// EventType represents types of system events.
type EventType string

const (
	EventDaemonStarted    EventType = "daemon_started"
	EventDaemonStopped    EventType = "daemon_stopped"
	EventContainerFound   EventType = "container_found"
	EventContainerRemoved EventType = "container_removed"
	EventQuotaWarning     EventType = "quota_warning"
	EventQuotaExceeded    EventType = "quota_exceeded"
	EventReset            EventType = "reset"
	EventCleanup          EventType = "cleanup"
	EventError            EventType = "error"
	EventConfigUpdated    EventType = "config_updated"
	EventTCFailed         EventType = "tc_failed"
	EventDockerError      EventType = "docker_error"
	EventWebhookRetry     EventType = "webhook_retry"
	EventWebhookSuccess   EventType = "webhook_success"
	EventWebhookFailure   EventType = "webhook_failure"
)

// Event represents a system event for logging and webhook dispatch.
type Event struct {
	ID          int64     `json:"id"`
	Type        EventType `json:"type"`
	ContainerID string    `json:"container_id,omitempty"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"`           // info, warning, error, critical
	Metadata    string    `json:"metadata,omitempty"` // JSON-encoded extra data
	Timestamp   time.Time `json:"timestamp"`
}

// ─── Webhook ──────────────────────────────────────────────────────────────────

// WebhookPayload represents the JSON payload sent to webhook endpoints.
type WebhookPayload struct {
	Event     EventType `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Container string    `json:"container,omitempty"`
	Severity  string    `json:"severity"`
	Metadata  string    `json:"metadata,omitempty"`
}

// WebhookConfig holds per-webhook endpoint configuration.
type WebhookConfig struct {
	URL         string        `json:"url"`
	Type        string        `json:"type"` // discord, slack, generic
	Enabled     bool          `json:"enabled"`
	Events      []EventType   `json:"events"` // which events to send
	Timeout     time.Duration `json:"timeout"`
	MaxRetries  int           `json:"max_retries"`
	BackoffBase time.Duration `json:"backoff_base"`
}

// ─── Scheduler Job ────────────────────────────────────────────────────────────

// JobType represents the type of scheduled job.
type JobType string

const (
	JobReset         JobType = "reset"
	JobCleanup       JobType = "cleanup"
	JobHistoryRollup JobType = "history_rollup"
	JobWebhookRetry  JobType = "webhook_retry"
	JobHealthCheck   JobType = "health_check"
)

// ScheduledJob represents a job in the internal scheduler.
type ScheduledJob struct {
	ID        int64     `json:"id"`
	Type      JobType   `json:"type"`
	CronExpr  string    `json:"cron_expr"`
	NextRun   time.Time `json:"next_run"`
	LastRun   time.Time `json:"last_run,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── Daemon Status ────────────────────────────────────────────────────────────

// DaemonStatus represents the current status of the bandwidth daemon.
type DaemonStatus struct {
	Version        string    `json:"version"`
	Uptime         string    `json:"uptime"`
	StartTime      time.Time `json:"start_time"`
	State          string    `json:"state"` // running, stopped, error
	ContainerCount int       `json:"container_count"`
	ManagedCount   int       `json:"managed_count"`
	ExceededCount  int       `json:"exceeded_count"`

	// Resource usage
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`

	// Health
	DockerHealthy bool `json:"docker_healthy"`
	DatabaseOK    bool `json:"database_ok"`
	TCHealthy     bool `json:"tc_healthy"`

	// Counters
	EventsProcessed int64     `json:"events_processed"`
	WebhooksSent    int64     `json:"webhooks_sent"`
	WebhooksFailed  int64     `json:"webhooks_failed"`
	TCRulesApplied  int64     `json:"tc_rules_applied"`
	LastReset       time.Time `json:"last_reset"`

	// Configuration
	PollIntervalSec int    `json:"poll_interval_sec"`
	Timezone        string `json:"timezone"`
}

// ─── Health Report ────────────────────────────────────────────────────────────

// HealthReport represents a detailed system health check.
type HealthReport struct {
	Overall   string        `json:"overall"` // healthy, degraded, unhealthy
	Timestamp time.Time     `json:"timestamp"`
	Checks    []HealthCheck `json:"checks"`
}

// HealthCheck represents a single health check result.
type HealthCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok, warning, error
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// ─── Config (embedded model for CLI/TUI access) ───────────────────────────────

// ConfigSummary is a lightweight view of configuration for the CLI/TUI.
type ConfigSummary struct {
	ConfigPath     string  `json:"config_path"`
	DBPath         string  `json:"db_path"`
	LogPath        string  `json:"log_path"`
	SocketPath     string  `json:"socket_path"`
	Timezone       string  `json:"timezone"`
	PollInterval   int     `json:"poll_interval_s"`
	APIPort        int     `json:"api_port"`
	MetricsPort    int     `json:"metrics_port"`
	APIEnabled     bool    `json:"api_enabled"`
	MetricsEnabled bool    `json:"metrics_enabled"`
	DefaultRxMbps  float64 `json:"default_rx_mbps"`
	DefaultTxMbps  float64 `json:"default_tx_mbps"`
	DefaultQuotaGB float64 `json:"default_quota_gb"`
	ExceededSpeed  float64 `json:"exceeded_speed_mbps"`
	CleanupHours   int     `json:"cleanup_age_hours"`
}
