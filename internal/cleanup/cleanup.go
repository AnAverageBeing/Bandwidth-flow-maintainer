package cleanup

import (
	"context"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/database"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/docker"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/tc"
)

// Manager handles automatic cleanup of stale records, missing containers, and old tc rules.
type Manager struct {
	log        *logger.Logger
	db         *database.DB
	tc         *tc.Manager
	discovery  *docker.Discovery
	interval   time.Duration
	staleHours int
	compact    bool
	enabled    bool
}

// Config holds cleanup settings.
type Config struct {
	Enabled             bool
	Interval            time.Duration
	StaleContainerHours int
	RemoveTCRules       bool
	CompactDB           bool
}

// NewManager creates a cleanup manager.
func NewManager(cfg Config, log *logger.Logger, db *database.DB, tcMgr *tc.Manager, disc *docker.Discovery) *Manager {
	return &Manager{
		log:        log,
		db:         db,
		tc:         tcMgr,
		discovery:  disc,
		interval:   cfg.Interval,
		staleHours: cfg.StaleContainerHours,
		compact:    cfg.CompactDB,
		enabled:    cfg.Enabled,
	}
}

// Run executes a full cleanup cycle.
func (m *Manager) Run(ctx context.Context) error {
	if !m.enabled {
		m.log.Debug("cleanup: disabled — skipping cycle")
		return nil
	}

	m.log.Info("cleanup: starting cycle")

	// 1. Remove stale containers from DB (skip if staleHours <= 0)
	if m.staleHours > 0 {
		staleAge := time.Duration(m.staleHours) * time.Hour
		if removed, err := m.db.CleanupStaleContainers(staleAge); err != nil {
			m.log.Error("cleanup: stale containers: %v", err)
		} else if removed > 0 {
			m.log.Info("cleanup: removed %d stale container records", removed)
		}
	} else {
		m.log.Debug("cleanup: stale container deletion disabled (stale_container_hours <= 0)")
	}

	// 2. Clean up old usage records (> 30 days) — only if stale cleanup is enabled
	if m.staleHours > 0 {
		if removed, err := m.db.CleanupOldUsage(30 * 24 * time.Hour); err != nil {
			m.log.Error("cleanup: old usage: %v", err)
		} else if removed > 0 {
			m.log.Info("cleanup: removed %d old usage records", removed)
		}
	}

	// 3. Repair tc rules (remove rules for containers that no longer exist)
	if m.tc != nil && m.discovery != nil {
		containers := m.discovery.ListContainers()
		m.tc.RepairRules(containers)
	}

	// 4. Compact database
	if m.compact {
		if err := m.db.Compact(); err != nil {
			m.log.Error("cleanup: compact: %v", err)
		} else {
			m.log.Debug("cleanup: database compacted")
		}
	}

	m.log.Info("cleanup: cycle complete")
	return nil
}
