package quota

import (
	"sync"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Manager handles daily traffic quotas and midnight resets.
type Manager struct {
	log             *logger.Logger
	mu              sync.RWMutex
	exceededSpeedRx float64
	exceededSpeedTx float64
	defaultQuotaGB  float64
	warningPercent  float64
	lastReset       time.Time
}

// Config holds quota manager settings.
type Config struct {
	ExceededSpeedRx float64
	ExceededSpeedTx float64
	DefaultQuotaGB  float64
	WarningPercent  float64
}

// NewManager creates a new quota manager.
func NewManager(cfg Config, log *logger.Logger) *Manager {
	return &Manager{
		log:             log,
		exceededSpeedRx: cfg.ExceededSpeedRx,
		exceededSpeedTx: cfg.ExceededSpeedTx,
		defaultQuotaGB:  cfg.DefaultQuotaGB,
		warningPercent:  cfg.WarningPercent,
	}
}

// CheckQuota evaluates whether a container has exceeded its daily quota.
// Returns (exceeded, warning, shouldThrottle).
func (m *Manager) CheckQuota(c *models.Container) (exceeded bool, warning bool) {
	if c.DailyQuotaGB <= 0 {
		return false, false // unlimited
	}

	used := c.TodayRxGB + c.TodayTxGB
	if used >= c.DailyQuotaGB {
		m.log.Warn("quota: container %s exceeded quota (%.2f/%.2f GB)", c.Name, used, c.DailyQuotaGB)
		return true, true
	}

	threshold := c.WarningPercent
	if threshold <= 0 {
		threshold = m.warningPercent
	}
	if threshold > 0 && used >= c.DailyQuotaGB*(threshold/100.0) {
		m.log.Info("quota: container %s warning (%.2f/%.2f GB = %.0f%%)", c.Name, used, c.DailyQuotaGB, (used/c.DailyQuotaGB)*100)
		return false, true
	}

	return false, false
}

// ApplyExceededThrottle adjusts a container's limits when quota is exceeded.
func (m *Manager) ApplyExceededThrottle(c *models.Container) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c.State = models.StateExceeded
	// Save original limits before throttling
	if c.ExceededSpeedRx <= 0 {
		c.ExceededSpeedRx = m.exceededSpeedRx
	}
	if c.ExceededSpeedTx <= 0 {
		c.ExceededSpeedTx = m.exceededSpeedTx
	}
	m.log.Warn("quota: throttled %s to %.1f/%.1f Mbps", c.Name, c.ExceededSpeedRx, c.ExceededSpeedTx)
}

// RestoreSpeed restores original limits after quota reset.
func (m *Manager) RestoreSpeed(c *models.Container, origRx, origTx float64) {
	c.State = models.StateRunning
	c.LimitRxMbps = origRx
	c.LimitTxMbps = origTx
}

// ResetDaily resets daily counters for all containers.
func (m *Manager) ResetDaily(containers []*models.Container) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, c := range containers {
		if c.State == models.StateExceeded {
			c.State = models.StateRunning
		}
		c.TodayRxGB = 0
		c.TodayTxGB = 0
		c.RxBytes = 0
		c.TxBytes = 0
		count++
	}
	m.lastReset = time.Now()
	m.log.Info("quota: reset daily usage for %d containers", count)
	return count
}

// ShouldReset returns true if midnight has passed since the last reset.
func (m *Manager) ShouldReset(loc *time.Location) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.lastReset.IsZero() {
		return true // first run
	}

	now := time.Now().In(loc)
	lastInLoc := m.lastReset.In(loc)

	// Check if the calendar day has changed
	return now.YearDay() != lastInLoc.YearDay() || now.Year() != lastInLoc.Year()
}

// LastReset returns the timestamp of the last reset.
func (m *Manager) LastReset() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastReset
}

// SetLastReset explicitly sets the last reset time (e.g., after startup).
func (m *Manager) SetLastReset(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastReset = t
}
