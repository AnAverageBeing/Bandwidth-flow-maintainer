// Package monitor handles periodic bandwidth usage collection from Linux
// network interface statistics (/sys/class/net/<iface>/statistics).
// It tracks RX/TX bytes and computes Mbps rates with configurable polling intervals.
package monitor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Monitor collects bandwidth statistics from container veth interfaces.
type Monitor struct {
	log      *logger.Logger
	interval time.Duration
	mu       sync.RWMutex
	prev     map[string]snapshot // containerID -> previous reading
}

type snapshot struct {
	RxBytes   uint64
	TxBytes   uint64
	Timestamp time.Time
}

// Config holds monitor parameters.
type Config struct {
	PollInterval time.Duration
}

// NewMonitor creates a new bandwidth monitor.
func NewMonitor(cfg Config, log *logger.Logger) *Monitor {
	return &Monitor{
		log:      log,
		interval: cfg.PollInterval,
		prev:     make(map[string]snapshot),
	}
}

// Interval returns the configured polling interval.
func (m *Monitor) Interval() time.Duration {
	return m.interval
}

// Collect reads current RX/TX bytes from the container's veth interface
// and computes Mbps rates based on the previous reading.
func (m *Monitor) Collect(container *models.Container) error {
	if container.VethInterface == "" {
		return fmt.Errorf("monitor: no veth interface for %s", container.ID[:12])
	}

	rxBytes, txBytes, err := readInterfaceStats(container.VethInterface)
	if err != nil {
		return fmt.Errorf("monitor: read stats %s: %w", container.VethInterface, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	prev, exists := m.prev[container.ID]

	// Compute rates
	var rxMbps, txMbps float64
	if exists && prev.Timestamp.Before(now) {
		elapsed := now.Sub(prev.Timestamp).Seconds()
		if elapsed > 0 {
			rxDiff := rxBytes - prev.RxBytes
			txDiff := txBytes - prev.TxBytes
			// bytes -> bits -> megabits / seconds = Mbps
			rxMbps = float64(rxDiff*8) / elapsed / 1e6
			txMbps = float64(txDiff*8) / elapsed / 1e6
		}
	}

	// Store current snapshot
	m.prev[container.ID] = snapshot{
		RxBytes:   rxBytes,
		TxBytes:   txBytes,
		Timestamp: now,
	}

	// Update container
	container.RxBytes = rxBytes
	container.TxBytes = txBytes
	container.CurrentRxMbps = rxMbps
	container.CurrentTxMbps = txMbps

	// Accumulate today's usage (approximate)
	elapsedSinceLast := m.interval.Seconds()
	if exists && elapsedSinceLast > 0 {
		container.TodayRxGB += rxMbps * elapsedSinceLast / 8000
		container.TodayTxGB += txMbps * elapsedSinceLast / 8000
	}

	return nil
}

// ResetDaily zeroes out today's usage for a container.
func (m *Monitor) ResetDaily(container *models.Container) {
	container.TodayRxGB = 0
	container.TodayTxGB = 0
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// readInterfaceStats reads RX/TX bytes from /sys/class/net/<iface>/statistics/.
func readInterfaceStats(iface string) (uint64, uint64, error) {
	base := fmt.Sprintf("/sys/class/net/%s/statistics", iface)

	rxData, err := os.ReadFile(base + "/rx_bytes")
	if err != nil {
		return 0, 0, fmt.Errorf("read rx_bytes: %w", err)
	}

	txData, err := os.ReadFile(base + "/tx_bytes")
	if err != nil {
		return 0, 0, fmt.Errorf("read tx_bytes: %w", err)
	}

	rx, err := strconv.ParseUint(strings.TrimSpace(string(rxData)), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse rx_bytes: %w", err)
	}

	tx, err := strconv.ParseUint(strings.TrimSpace(string(txData)), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse tx_bytes: %w", err)
	}

	return rx, tx, nil
}
