// Package config provides YAML-based configuration loading, validation, and management.
// It supports defaults, file loading, environment overrides, and interactive setup.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure. All fields are configurable via YAML.
type Config struct {
	General        GeneralConfig        `yaml:"general"`
	Logging        LoggingConfig        `yaml:"logging"`
	Database       DatabaseConfig       `yaml:"database"`
	Docker         DockerConfig         `yaml:"docker"`
	Bandwidth      BandwidthConfig      `yaml:"bandwidth"`
	Quota          QuotaConfig          `yaml:"quota"`
	Webhook        WebhookConfig        `yaml:"webhook"`
	Scheduler      SchedulerConfig      `yaml:"scheduler"`
	Status         StatusConfig         `yaml:"status"`
	History        HistoryConfig        `yaml:"history"`
	Cleanup        CleanupConfig        `yaml:"cleanup"`
	Metrics        MetricsConfig        `yaml:"metrics"`
	API            APIConfig            `yaml:"api"`
	TUI            TUIConfig            `yaml:"tui"`
	RateLimiter    RateLimiterConfig    `yaml:"rate_limiter"`
	Labels         LabelsConfig         `yaml:"labels"`
	TrafficControl TrafficControlConfig `yaml:"traffic_control"`
	Defaults       DefaultsConfig       `yaml:"defaults"`
	Timezone       string               `yaml:"timezone"`
}

// GeneralConfig holds top-level settings.
type GeneralConfig struct {
	SocketPath string `yaml:"socket_path"` // Unix socket for CLI↔daemon communication
	LockFile   string `yaml:"lock_file"`
	PIDFile    string `yaml:"pid_file"`
}

// LoggingConfig configures structured logging.
type LoggingConfig struct {
	Level      string `yaml:"level"` // debug, info, warn, error
	Console    bool   `yaml:"console"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxAgeDays int    `yaml:"max_age_days"`
	MaxBackups int    `yaml:"max_backups"`
	Compress   bool   `yaml:"compress"`
	Format     string `yaml:"format"` // text, json
}

// DatabaseConfig configures the SQLite database.
type DatabaseConfig struct {
	Path         string `yaml:"path"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
	JournalMode  string `yaml:"journal_mode"` // WAL, DELETE, TRUNCATE
	Synchronous  string `yaml:"synchronous"`  // OFF, NORMAL, FULL
	CacheSizeKB  int    `yaml:"cache_size_kb"`
	AutoMigrate  bool   `yaml:"auto_migrate"`
}

// DockerConfig configures Docker engine connection.
type DockerConfig struct {
	Endpoint          string        `yaml:"endpoint"` // unix:///var/run/docker.sock
	APIVersion        string        `yaml:"api_version"`
	TLSVerify         bool          `yaml:"tls_verify"`
	TLSCertPath       string        `yaml:"tls_cert_path"`
	DiscoveryInterval time.Duration `yaml:"discovery_interval"`
	WatchEvents       bool          `yaml:"watch_events"`
}

// BandwidthConfig configures default bandwidth limits.
type BandwidthConfig struct {
	DefaultRxMbps    float64       `yaml:"default_rx_mbps"`
	DefaultTxMbps    float64       `yaml:"default_tx_mbps"`
	DefaultCeilMbps  float64       `yaml:"default_ceil_mbps"`
	DefaultBurstMbps float64       `yaml:"default_burst_mbps"`
	PollInterval     time.Duration `yaml:"poll_interval"`
}

// QuotaConfig configures daily quota settings.
type QuotaConfig struct {
	DefaultQuotaGB  float64 `yaml:"default_quota_gb"`
	ExceededSpeedRx float64 `yaml:"exceeded_speed_rx_mbps"` // throttle when quota hit
	ExceededSpeedTx float64 `yaml:"exceeded_speed_tx_mbps"`
	WarningPercent  float64 `yaml:"warning_percent"`
	Disconnect      bool    `yaml:"disconnect_on_exceeded"` // if true, kill bandwidth entirely
}

// WebhookConfig holds webhook notification settings.
type WebhookConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Endpoints    []WebhookEndpoint `yaml:"endpoints"`
	MaxRetries   int               `yaml:"max_retries"`
	BackoffBase  time.Duration     `yaml:"backoff_base"`
	QueueSize    int               `yaml:"queue_size"`
	PersistQueue bool              `yaml:"persist_queue"`
}

// WebhookEndpoint defines a single webhook destination.
type WebhookEndpoint struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Type    string `yaml:"type"` // discord, slack, generic
	Enabled bool   `yaml:"enabled"`
}

// SchedulerConfig configures the internal job scheduler.
type SchedulerConfig struct {
	Enabled          bool          `yaml:"enabled"`
	ResetCron        string        `yaml:"reset_cron"`         // default: "0 0 * * *"
	CleanupCron      string        `yaml:"cleanup_cron"`       // default: "0 */6 * * *"
	HistoryCron      string        `yaml:"history_cron"`       // default: "5 0 * * *"
	HealthCron       string        `yaml:"health_cron"`        // default: "*/5 * * * *"
	WebhookRetryCron string        `yaml:"webhook_retry_cron"` // default: "*/1 * * * *"
	CheckInterval    time.Duration `yaml:"check_interval"`
}

// StatusConfig configures the status tracking system.
type StatusConfig struct {
	HistorySize    int           `yaml:"history_size"`
	UpdateInterval time.Duration `yaml:"update_interval"`
}

// HistoryConfig configures historical usage tracking.
type HistoryConfig struct {
	Enabled        bool          `yaml:"enabled"`
	RetentionDays  int           `yaml:"retention_days"`
	RollupInterval time.Duration `yaml:"rollup_interval"`
	ExportEnabled  bool          `yaml:"export_enabled"`
	ExportPath     string        `yaml:"export_path"`
}

// CleanupConfig configures automatic cleanup.
type CleanupConfig struct {
	Enabled             bool          `yaml:"enabled"`
	Interval            time.Duration `yaml:"interval"`
	StaleContainerHours int           `yaml:"stale_container_hours"`
	RemoveTCRules       bool          `yaml:"remove_tc_rules"`
	CompactDB           bool          `yaml:"compact_db"`
}

// MetricsConfig configures Prometheus metrics exposure.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// APIConfig configures the optional REST API.
type APIConfig struct {
	Enabled    bool   `yaml:"enabled"`
	SocketPath string `yaml:"socket_path"`
	TCPPort    int    `yaml:"tcp_port"`
	AuthToken  string `yaml:"auth_token"`
	ReadOnly   bool   `yaml:"read_only"`
}

// TUIConfig configures the terminal UI.
type TUIConfig struct {
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	DefaultSort     string        `yaml:"default_sort"`
	MaxContainers   int           `yaml:"max_containers"`
	ColorTheme      string        `yaml:"color_theme"`
}

// RateLimiterConfig configures token bucket rate smoothing.
type RateLimiterConfig struct {
	Enabled        bool          `yaml:"enabled"`
	PeakBurstSec   float64       `yaml:"peak_burst_seconds"`
	SustainedRate  float64       `yaml:"sustained_rate_factor"`
	RecoveryWindow time.Duration `yaml:"recovery_window"`
	GraceWindow    time.Duration `yaml:"grace_window"`
}

// LabelsConfig maps Docker label keys to their meaning.
type LabelsConfig struct {
	Enabled       string `yaml:"enabled"`        // bandwidth.enabled
	Speed         string `yaml:"speed"`          // bandwidth.speed
	DailyQuota    string `yaml:"daily_quota"`    // bandwidth.daily_quota
	Warning       string `yaml:"warning"`        // bandwidth.warning
	Webhook       string `yaml:"webhook"`        // bandwidth.webhook
	History       string `yaml:"history"`        // bandwidth.history
	Priority      string `yaml:"priority"`       // bandwidth.priority
	ExceededSpeed string `yaml:"exceeded_speed"` // bandwidth.exceeded_speed
}

// TrafficControlConfig configures Linux tc behavior.
type TrafficControlConfig struct {
	Enabled       bool   `yaml:"enabled"`
	DefaultQdisc  string `yaml:"default_qdisc"` // htb, tbf
	HandleRoot    string `yaml:"handle_root"`
	DefaultClass  string `yaml:"default_class"`
	VerifyOnApply bool   `yaml:"verify_on_apply"`
	RepairOnFail  bool   `yaml:"repair_on_fail"`
	MaxRetries    int    `yaml:"max_retries"`
}

// DefaultsConfig provides fallback values applied when labels/config don't specify.
type DefaultsConfig struct {
	SpeedMbps      float64 `yaml:"speed_mbps"`
	DailyQuotaGB   float64 `yaml:"daily_quota_gb"`
	WarningPercent float64 `yaml:"warning_percent"`
	Priority       string  `yaml:"priority"`
	Webhook        bool    `yaml:"webhook"`
	History        bool    `yaml:"history"`
}

// ─── Constructor & Helpers ────────────────────────────────────────────────────

// DefaultConfig returns a Config populated with safe, production-ready defaults.
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			SocketPath: "/var/run/bandwidth.sock",
			LockFile:   "/var/run/bandwidth.lock",
			PIDFile:    "/var/run/bandwidth.pid",
		},
		Logging: LoggingConfig{
			Level:      "info",
			Console:    true,
			File:       "/var/log/bandwidth/bandwidth.log",
			MaxSizeMB:  100,
			MaxAgeDays: 30,
			MaxBackups: 10,
			Compress:   true,
			Format:     "json",
		},
		Database: DatabaseConfig{
			Path:         "/var/lib/bandwidth/bandwidth.db",
			MaxOpenConns: 1,
			MaxIdleConns: 1,
			JournalMode:  "WAL",
			Synchronous:  "NORMAL",
			CacheSizeKB:  32000,
			AutoMigrate:  true,
		},
		Docker: DockerConfig{
			Endpoint:          "unix:///var/run/docker.sock",
			APIVersion:        "1.44",
			DiscoveryInterval: 10 * time.Second,
			WatchEvents:       true,
		},
		Bandwidth: BandwidthConfig{
			DefaultRxMbps:    100,
			DefaultTxMbps:    100,
			DefaultCeilMbps:  200,
			DefaultBurstMbps: 150,
			PollInterval:     5 * time.Second,
		},
		Quota: QuotaConfig{
			DefaultQuotaGB:  500,
			ExceededSpeedRx: 1,
			ExceededSpeedTx: 1,
			WarningPercent:  90,
			Disconnect:      false,
		},
		Webhook: WebhookConfig{
			Enabled:      false,
			MaxRetries:   3,
			BackoffBase:  5 * time.Second,
			QueueSize:    10000,
			PersistQueue: true,
		},
		Scheduler: SchedulerConfig{
			Enabled:          true,
			ResetCron:        "0 0 * * *",
			CleanupCron:      "0 */6 * * *",
			HistoryCron:      "5 0 * * *",
			HealthCron:       "*/5 * * * *",
			WebhookRetryCron: "*/1 * * * *",
			CheckInterval:    30 * time.Second,
		},
		Status: StatusConfig{
			HistorySize:    1000,
			UpdateInterval: 10 * time.Second,
		},
		History: HistoryConfig{
			Enabled:        true,
			RetentionDays:  365,
			RollupInterval: 1 * time.Hour,
			ExportEnabled:  false,
			ExportPath:     "/var/lib/bandwidth/exports",
		},
		Cleanup: CleanupConfig{
			Enabled:             true,
			Interval:            1 * time.Hour,
			StaleContainerHours: 72,
			RemoveTCRules:       true,
			CompactDB:           true,
		},
		Metrics: MetricsConfig{
			Enabled: false,
			Port:    9090,
			Path:    "/metrics",
		},
		API: APIConfig{
			Enabled:    false,
			SocketPath: "/var/run/bandwidth-api.sock",
			TCPPort:    8080,
			ReadOnly:   true,
		},
		TUI: TUIConfig{
			RefreshInterval: 2 * time.Second,
			DefaultSort:     "name",
			MaxContainers:   500,
			ColorTheme:      "default",
		},
		RateLimiter: RateLimiterConfig{
			Enabled:        true,
			PeakBurstSec:   10,
			SustainedRate:  0.8,
			RecoveryWindow: 30 * time.Second,
			GraceWindow:    5 * time.Second,
		},
		Labels: LabelsConfig{
			Enabled:       "bandwidth.enabled",
			Speed:         "bandwidth.speed",
			DailyQuota:    "bandwidth.daily_quota",
			Warning:       "bandwidth.warning",
			Webhook:       "bandwidth.webhook",
			History:       "bandwidth.history",
			Priority:      "bandwidth.priority",
			ExceededSpeed: "bandwidth.exceeded_speed",
		},
		TrafficControl: TrafficControlConfig{
			Enabled:       true,
			DefaultQdisc:  "htb",
			HandleRoot:    "1:",
			DefaultClass:  "1:1",
			VerifyOnApply: true,
			RepairOnFail:  true,
			MaxRetries:    3,
		},
		Defaults: DefaultsConfig{
			SpeedMbps:      100,
			DailyQuotaGB:   500,
			WarningPercent: 90,
			Priority:       "standard",
			Webhook:        false,
			History:        true,
		},
		Timezone: "Asia/Kolkata",
	}
}

// Load reads a YAML config file and merges it with defaults.
// Returns the merged config. Missing keys retain their default values.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // return defaults if no config file
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return cfg, nil
}

// Save writes the configuration to a YAML file.
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	if err := os.MkdirAll(dirname(path), 0755); err != nil {
		return fmt.Errorf("config: mkdir: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}

	return nil
}

// Validate checks the configuration for correctness and returns all errors found.
func (c *Config) Validate() []error {
	var errs []error

	if c.General.SocketPath == "" {
		errs = append(errs, fmt.Errorf("general.socket_path is required"))
	}
	if c.Database.Path == "" {
		errs = append(errs, fmt.Errorf("database.path is required"))
	}
	if c.Bandwidth.DefaultRxMbps <= 0 {
		errs = append(errs, fmt.Errorf("bandwidth.default_rx_mbps must be positive"))
	}
	if c.Bandwidth.DefaultTxMbps <= 0 {
		errs = append(errs, fmt.Errorf("bandwidth.default_tx_mbps must be positive"))
	}
	if c.Bandwidth.PollInterval <= 0 {
		errs = append(errs, fmt.Errorf("bandwidth.poll_interval must be positive"))
	}
	if c.Quota.DefaultQuotaGB < 0 {
		errs = append(errs, fmt.Errorf("quota.default_quota_gb must be >= 0"))
	}
	if c.Quota.WarningPercent < 0 || c.Quota.WarningPercent > 100 {
		errs = append(errs, fmt.Errorf("quota.warning_percent must be 0-100"))
	}
	if c.TrafficControl.Enabled && c.TrafficControl.DefaultQdisc == "" {
		errs = append(errs, fmt.Errorf("traffic_control.default_qdisc is required when tc is enabled"))
	}
	if c.Cleanup.StaleContainerHours < 1 {
		errs = append(errs, fmt.Errorf("cleanup.stale_container_hours must be >= 1"))
	}
	if c.Scheduler.Enabled && c.Scheduler.ResetCron == "" {
		errs = append(errs, fmt.Errorf("scheduler.reset_cron is required when scheduler is enabled"))
	}
	if c.Metrics.Enabled && (c.Metrics.Port < 1 || c.Metrics.Port > 65535) {
		errs = append(errs, fmt.Errorf("metrics.port must be 1-65535"))
	}
	if c.API.Enabled && c.API.TCPPort > 0 && (c.API.TCPPort < 1 || c.API.TCPPort > 65535) {
		errs = append(errs, fmt.Errorf("api.tcp_port must be 1-65535"))
	}
	if c.Timezone == "" {
		errs = append(errs, fmt.Errorf("timezone is required"))
	}

	return errs
}

// GetTimezoneLocation returns the Go *time.Location for the configured timezone.
func (c *Config) GetTimezoneLocation() (*time.Location, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", c.Timezone, err)
	}
	return loc, nil
}

// dirname is a simple path dirname helper to avoid importing path/filepath in tiny contexts.
func dirname(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
