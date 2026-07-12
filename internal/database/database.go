// Package database provides the SQLite persistence layer for the bandwidth manager.
// It handles schema creation, migrations, and all CRUD operations through a clean interface.
package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection with convenience methods for the bandwidth system.
type DB struct {
	conn *sql.DB
	mu   sync.RWMutex
}

// Config holds database initialization parameters.
type Config struct {
	Path         string
	MaxOpenConns int
	MaxIdleConns int
	JournalMode  string
	Synchronous  string
	CacheSizeKB  int
}

// Open initializes the SQLite database, applies pragmas, and runs migrations.
func Open(cfg Config) (*DB, error) {
	conn, err := sql.Open("sqlite", cfg.Path+"?_journal_mode="+cfg.JournalMode+
		"&_synchronous="+cfg.Synchronous+
		"&_cache_size="+fmt.Sprintf("%d", cfg.CacheSizeKB))
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(0) // SQLite works best with persistent connections

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	db := &DB{conn: conn}

	// Apply pragmas
	for _, pragma := range []string{
		fmt.Sprintf("PRAGMA journal_mode=%s", cfg.JournalMode),
		fmt.Sprintf("PRAGMA synchronous=%s", cfg.Synchronous),
		fmt.Sprintf("PRAGMA cache_size=%d", cfg.CacheSizeKB),
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			return nil, fmt.Errorf("database: pragma: %w", err)
		}
	}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("database: migrate: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Ping checks database connectivity.
func (db *DB) Ping() error {
	return db.conn.Ping()
}

// ─── Migrations ───────────────────────────────────────────────────────────────

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS containers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			pid INTEGER DEFAULT 0,
			state TEXT NOT NULL DEFAULT 'unknown',
			veth_iface TEXT DEFAULT '',
			network_name TEXT DEFAULT '',
			ip_address TEXT DEFAULT '',
			ports TEXT DEFAULT '[]',
			labels TEXT DEFAULT '{}',
			restart_policy TEXT DEFAULT '',
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			limit_rx_mbps REAL NOT NULL DEFAULT 100,
			limit_tx_mbps REAL NOT NULL DEFAULT 100,
			ceil_rx_mbps REAL NOT NULL DEFAULT 200,
			ceil_tx_mbps REAL NOT NULL DEFAULT 200,
			daily_quota_gb REAL NOT NULL DEFAULT 500,
			warning_percent REAL NOT NULL DEFAULT 90,
			exceeded_speed_rx REAL NOT NULL DEFAULT 1,
			exceeded_speed_tx REAL NOT NULL DEFAULT 1,
			rx_bytes INTEGER NOT NULL DEFAULT 0,
			tx_bytes INTEGER NOT NULL DEFAULT 0,
			current_rx_mbps REAL NOT NULL DEFAULT 0,
			current_tx_mbps REAL NOT NULL DEFAULT 0,
			today_rx_gb REAL NOT NULL DEFAULT 0,
			today_tx_gb REAL NOT NULL DEFAULT 0,
			priority TEXT NOT NULL DEFAULT 'standard',
			enabled INTEGER NOT NULL DEFAULT 1,
			webhook INTEGER NOT NULL DEFAULT 0,
			history INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			container_id TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 0,
			rx_mbps REAL NOT NULL DEFAULT 100,
			tx_mbps REAL NOT NULL DEFAULT 100,
			burst_mbps REAL NOT NULL DEFAULT 150,
			latency_ms REAL NOT NULL DEFAULT 0,
			priority TEXT NOT NULL DEFAULT 'standard',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (container_id) REFERENCES containers(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			container_id TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 0,
			rx_bytes INTEGER NOT NULL DEFAULT 0,
			tx_bytes INTEGER NOT NULL DEFAULT 0,
			rx_mbps REAL NOT NULL DEFAULT 0,
			tx_mbps REAL NOT NULL DEFAULT 0,
			timestamp TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (container_id) REFERENCES containers(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			container_id TEXT NOT NULL,
			period TEXT NOT NULL,
			period_start TEXT NOT NULL,
			period_end TEXT NOT NULL,
			total_rx_bytes INTEGER NOT NULL DEFAULT 0,
			total_tx_bytes INTEGER NOT NULL DEFAULT 0,
			total_rx_gb REAL NOT NULL DEFAULT 0,
			total_tx_gb REAL NOT NULL DEFAULT 0,
			peak_rx_mbps REAL NOT NULL DEFAULT 0,
			peak_tx_mbps REAL NOT NULL DEFAULT 0,
			avg_rx_mbps REAL NOT NULL DEFAULT 0,
			avg_tx_mbps REAL NOT NULL DEFAULT 0,
			sample_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_container_ts ON usage(container_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_ts ON usage(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_history_container_period ON history(container_id, period, period_start)`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			container_id TEXT DEFAULT '',
			message TEXT NOT NULL,
			severity TEXT NOT NULL DEFAULT 'info',
			metadata TEXT DEFAULT '',
			timestamp TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_type_ts ON events(type, timestamp)`,
		`CREATE TABLE IF NOT EXISTS webhooks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'generic',
			enabled INTEGER NOT NULL DEFAULT 1,
			events TEXT NOT NULL DEFAULT '[]',
			timeout_sec INTEGER NOT NULL DEFAULT 10,
			max_retries INTEGER NOT NULL DEFAULT 3,
			backoff_base_sec INTEGER NOT NULL DEFAULT 5,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS webhook_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			webhook_id INTEGER NOT NULL,
			payload TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_attempt TEXT,
			next_attempt TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_queue_status ON webhook_queue(status, next_attempt)`,
		`CREATE TABLE IF NOT EXISTS scheduler (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			cron_expr TEXT NOT NULL,
			next_run TEXT NOT NULL,
			last_run TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("migration: %w\nSQL: %s", err, m)
		}
	}

	// Insert schema version tracking
	db.conn.Exec(`INSERT OR IGNORE INTO schema_version (version) VALUES (1)`)

	return nil
}

// ─── Container Operations ─────────────────────────────────────────────────────

// UpsertContainer inserts or updates a container record.
func (db *DB) UpsertContainer(c *models.Container) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`
		INSERT INTO containers (id, name, pid, state, veth_iface, network_name, ip_address, ports, labels,
			restart_policy, first_seen, last_seen, limit_rx_mbps, limit_tx_mbps, ceil_rx_mbps, ceil_tx_mbps,
			daily_quota_gb, warning_percent, exceeded_speed_rx, exceeded_speed_tx,
			rx_bytes, tx_bytes, current_rx_mbps, current_tx_mbps, today_rx_gb, today_tx_gb,
			priority, enabled, webhook, history)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, pid=excluded.pid, state=excluded.state,
			veth_iface=excluded.veth_iface, network_name=excluded.network_name,
			ip_address=excluded.ip_address, ports=excluded.ports, labels=excluded.labels,
			restart_policy=excluded.restart_policy, last_seen=excluded.last_seen,
			limit_rx_mbps=excluded.limit_rx_mbps, limit_tx_mbps=excluded.limit_tx_mbps,
			ceil_rx_mbps=excluded.ceil_rx_mbps, ceil_tx_mbps=excluded.ceil_tx_mbps,
			daily_quota_gb=excluded.daily_quota_gb, warning_percent=excluded.warning_percent,
			exceeded_speed_rx=excluded.exceeded_speed_rx, exceeded_speed_tx=excluded.exceeded_speed_tx,
			rx_bytes=excluded.rx_bytes, tx_bytes=excluded.tx_bytes,
			current_rx_mbps=excluded.current_rx_mbps, current_tx_mbps=excluded.current_tx_mbps,
			today_rx_gb=excluded.today_rx_gb, today_tx_gb=excluded.today_tx_gb,
			priority=excluded.priority, enabled=excluded.enabled, webhook=excluded.webhook, history=excluded.history`,
		c.ID, c.Name, c.PID, string(c.State), c.VethInterface, c.NetworkName, c.IPAddress,
		toJSON(c.Ports), toJSON(c.Labels), c.RestartPolicy,
		c.FirstSeen.Format(time.RFC3339), c.LastSeen.Format(time.RFC3339),
		c.LimitRxMbps, c.LimitTxMbps, c.CeilRxMbps, c.CeilTxMbps,
		c.DailyQuotaGB, c.WarningPercent, c.ExceededSpeedRx, c.ExceededSpeedTx,
		c.RxBytes, c.TxBytes, c.CurrentRxMbps, c.CurrentTxMbps, c.TodayRxGB, c.TodayTxGB,
		c.Priority, btoi(c.Enabled), btoi(c.Webhook), btoi(c.History),
	)
	return err
}

// GetContainer retrieves a container by ID.
func (db *DB) GetContainer(id string) (*models.Container, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	row := db.conn.QueryRow(`SELECT id, name, pid, state, veth_iface, network_name, ip_address,
		ports, labels, restart_policy, first_seen, last_seen,
		limit_rx_mbps, limit_tx_mbps, ceil_rx_mbps, ceil_tx_mbps,
		daily_quota_gb, warning_percent, exceeded_speed_rx, exceeded_speed_tx,
		rx_bytes, tx_bytes, current_rx_mbps, current_tx_mbps, today_rx_gb, today_tx_gb,
		priority, enabled, webhook, history
		FROM containers WHERE id=?`, id)

	return scanContainer(row)
}

// ListContainers returns all containers, optionally filtered by state.
func (db *DB) ListContainers(state string) ([]*models.Container, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var rows *sql.Rows
	var err error
	if state != "" {
		rows, err = db.conn.Query(`SELECT id, name, pid, state, veth_iface, network_name, ip_address,
			ports, labels, restart_policy, first_seen, last_seen,
			limit_rx_mbps, limit_tx_mbps, ceil_rx_mbps, ceil_tx_mbps,
			daily_quota_gb, warning_percent, exceeded_speed_rx, exceeded_speed_tx,
			rx_bytes, tx_bytes, current_rx_mbps, current_tx_mbps, today_rx_gb, today_tx_gb,
			priority, enabled, webhook, history
			FROM containers WHERE state=?`, state)
	} else {
		rows, err = db.conn.Query(`SELECT id, name, pid, state, veth_iface, network_name, ip_address,
			ports, labels, restart_policy, first_seen, last_seen,
			limit_rx_mbps, limit_tx_mbps, ceil_rx_mbps, ceil_tx_mbps,
			daily_quota_gb, warning_percent, exceeded_speed_rx, exceeded_speed_tx,
			rx_bytes, tx_bytes, current_rx_mbps, current_tx_mbps, today_rx_gb, today_tx_gb,
			priority, enabled, webhook, history
			FROM containers`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanContainers(rows)
}

// DeleteContainer removes a container and its related records.
func (db *DB) DeleteContainer(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM usage WHERE container_id=?`, id)
	tx.Exec(`DELETE FROM limits WHERE container_id=?`, id)
	tx.Exec(`DELETE FROM containers WHERE id=?`, id)

	return tx.Commit()
}

// UpdateContainerUsage updates only the bandwidth usage fields.
func (db *DB) UpdateContainerUsage(id string, rxBytes, txBytes uint64, rxMbps, txMbps, rxGB, txGB float64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`UPDATE containers SET
		rx_bytes=?, tx_bytes=?,
		current_rx_mbps=?, current_tx_mbps=?,
		today_rx_gb=?, today_tx_gb=?
		WHERE id=?`,
		rxBytes, txBytes, rxMbps, txMbps, rxGB, txGB, id)
	return err
}

// ResetDailyUsage zeros out today's usage for all containers.
func (db *DB) ResetDailyUsage() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`UPDATE containers SET
		today_rx_gb=0, today_tx_gb=0,
		rx_bytes=0, tx_bytes=0,
		current_rx_mbps=0, current_tx_mbps=0`)
	return err
}

// ─── Usage Operations ─────────────────────────────────────────────────────────

// InsertUsage records a bandwidth usage sample.
func (db *DB) InsertUsage(containerID string, port int, rxBytes, txBytes uint64, rxMbps, txMbps float64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`INSERT INTO usage (container_id, port, rx_bytes, tx_bytes, rx_mbps, tx_mbps, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		containerID, port, rxBytes, txBytes, rxMbps, txMbps, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ─── History Operations ───────────────────────────────────────────────────────

// InsertHistory records an aggregated history entry.
func (db *DB) InsertHistory(h *models.HistoryRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`INSERT INTO history (container_id, period, period_start, period_end,
		total_rx_bytes, total_tx_bytes, total_rx_gb, total_tx_gb,
		peak_rx_mbps, peak_tx_mbps, avg_rx_mbps, avg_tx_mbps, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ContainerID, string(h.Period), h.PeriodStart.Format(time.RFC3339), h.PeriodEnd.Format(time.RFC3339),
		h.TotalRxBytes, h.TotalTxBytes, h.TotalRxGB, h.TotalTxGB,
		h.PeakRxMbps, h.PeakTxMbps, h.AvgRxMbps, h.AvgTxMbps, h.SampleCount)
	return err
}

// GetHistory retrieves history records for a container and period.
func (db *DB) GetHistory(containerID string, period models.HistoryPeriod, limit int) ([]*models.HistoryRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.conn.Query(`SELECT id, container_id, period, period_start, period_end,
		total_rx_bytes, total_tx_bytes, total_rx_gb, total_tx_gb,
		peak_rx_mbps, peak_tx_mbps, avg_rx_mbps, avg_tx_mbps, sample_count, created_at
		FROM history WHERE container_id=? AND period=? ORDER BY period_start DESC LIMIT ?`,
		containerID, string(period), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.HistoryRecord
	for rows.Next() {
		h := &models.HistoryRecord{}
		var ps, pe, ca string
		if err := rows.Scan(&h.ID, &h.ContainerID, &h.Period, &ps, &pe,
			&h.TotalRxBytes, &h.TotalTxBytes, &h.TotalRxGB, &h.TotalTxGB,
			&h.PeakRxMbps, &h.PeakTxMbps, &h.AvgRxMbps, &h.AvgTxMbps, &h.SampleCount, &ca); err != nil {
			return nil, err
		}
		h.PeriodStart, _ = time.Parse(time.RFC3339, ps)
		h.PeriodEnd, _ = time.Parse(time.RFC3339, pe)
		h.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		results = append(results, h)
	}
	return results, rows.Err()
}

// ─── Event Operations ─────────────────────────────────────────────────────────

// InsertEvent records a system event.
func (db *DB) InsertEvent(eventType models.EventType, containerID, message, severity, metadata string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`INSERT INTO events (type, container_id, message, severity, metadata)
		VALUES (?, ?, ?, ?, ?)`,
		string(eventType), containerID, message, severity, metadata)
	return err
}

// GetRecentEvents returns the most recent events.
func (db *DB) GetRecentEvents(limit int) ([]*models.Event, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.conn.Query(`SELECT id, type, container_id, message, severity, metadata, timestamp
		FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*models.Event
	for rows.Next() {
		e := &models.Event{}
		var ts string
		if err := rows.Scan(&e.ID, &e.Type, &e.ContainerID, &e.Message, &e.Severity, &e.Metadata, &ts); err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, ts)
		events = append(events, e)
	}
	return events, rows.Err()
}

// ─── Cleanup ──────────────────────────────────────────────────────────────────

// CleanupStaleContainers removes containers not seen within the given duration.
// If maxAge <= 0, this is a no-op (safety guard against accidental full deletion).
func (db *DB) CleanupStaleContainers(maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, nil // safety: never delete everything
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339)
	res, err := db.conn.Exec(`DELETE FROM containers WHERE last_seen < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CleanupOldUsage removes usage records older than the given duration.
func (db *DB) CleanupOldUsage(maxAge time.Duration) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339)
	res, err := db.conn.Exec(`DELETE FROM usage WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Compact runs VACUUM on the database to reclaim space.
func (db *DB) Compact() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`VACUUM`)
	return err
}

// ─── Config Operations ────────────────────────────────────────────────────────

// SetConfig sets a key-value config entry.
func (db *DB) SetConfig(key, value string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec(`INSERT OR REPLACE INTO config (key, value, updated_at) VALUES (?, ?, ?)`,
		key, value, time.Now().UTC().Format(time.RFC3339))
	return err
}

// GetConfig retrieves a config value by key.
func (db *DB) GetConfig(key string) (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var value string
	err := db.conn.QueryRow(`SELECT value FROM config WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// ─── Stats ────────────────────────────────────────────────────────────────────

// CountContainers returns the count of containers, optionally filtered by state.
func (db *DB) CountContainers(state string) (int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var count int
	var err error
	if state != "" {
		err = db.conn.QueryRow(`SELECT COUNT(*) FROM containers WHERE state=?`, state).Scan(&count)
	} else {
		err = db.conn.QueryRow(`SELECT COUNT(*) FROM containers`).Scan(&count)
	}
	return count, err
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func scanContainer(row *sql.Row) (*models.Container, error) {
	c := &models.Container{}
	var portsJSON, labelsJSON, firstSeen, lastSeen string
	var enabled, webhook, history int

	err := row.Scan(&c.ID, &c.Name, &c.PID, &c.State, &c.VethInterface, &c.NetworkName, &c.IPAddress,
		&portsJSON, &labelsJSON, &c.RestartPolicy, &firstSeen, &lastSeen,
		&c.LimitRxMbps, &c.LimitTxMbps, &c.CeilRxMbps, &c.CeilTxMbps,
		&c.DailyQuotaGB, &c.WarningPercent, &c.ExceededSpeedRx, &c.ExceededSpeedTx,
		&c.RxBytes, &c.TxBytes, &c.CurrentRxMbps, &c.CurrentTxMbps, &c.TodayRxGB, &c.TodayTxGB,
		&c.Priority, &enabled, &webhook, &history)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	c.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
	c.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
	c.Ports = fromJSONPorts(portsJSON)
	c.Labels = fromJSONMap(labelsJSON)
	c.Enabled = enabled != 0
	c.Webhook = webhook != 0
	c.History = history != 0

	return c, nil
}

func scanContainers(rows *sql.Rows) ([]*models.Container, error) {
	var containers []*models.Container
	for rows.Next() {
		c := &models.Container{}
		var portsJSON, labelsJSON, firstSeen, lastSeen string
		var enabled, webhook, history int

		err := rows.Scan(&c.ID, &c.Name, &c.PID, &c.State, &c.VethInterface, &c.NetworkName, &c.IPAddress,
			&portsJSON, &labelsJSON, &c.RestartPolicy, &firstSeen, &lastSeen,
			&c.LimitRxMbps, &c.LimitTxMbps, &c.CeilRxMbps, &c.CeilTxMbps,
			&c.DailyQuotaGB, &c.WarningPercent, &c.ExceededSpeedRx, &c.ExceededSpeedTx,
			&c.RxBytes, &c.TxBytes, &c.CurrentRxMbps, &c.CurrentTxMbps, &c.TodayRxGB, &c.TodayTxGB,
			&c.Priority, &enabled, &webhook, &history)
		if err != nil {
			return nil, err
		}

		c.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
		c.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		c.Ports = fromJSONPorts(portsJSON)
		c.Labels = fromJSONMap(labelsJSON)
		c.Enabled = enabled != 0
		c.Webhook = webhook != 0
		c.History = history != 0
		containers = append(containers, c)
	}
	return containers, rows.Err()
}

// JSON helpers for storing structured fields in SQLite.
func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func fromJSONPorts(s string) []models.PortMapping {
	if s == "" {
		return nil
	}
	var ports []models.PortMapping
	if err := json.Unmarshal([]byte(s), &ports); err != nil {
		return nil
	}
	return ports
}

func fromJSONMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
