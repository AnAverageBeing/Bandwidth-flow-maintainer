package monitor

import (
	"testing"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

func TestCollectHandlesCounterReset(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Console: false})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	defer log.Close()

	m := NewMonitor(Config{PollInterval: 5 * time.Second}, log)
	c := &models.Container{ID: "reset-test", VethInterface: "lo"}

	// First collect: store a high previous reading.
	m.prev[c.ID] = snapshot{RxBytes: 1e9, TxBytes: 1e9, Timestamp: time.Now().Add(-time.Second)}

	// Simulate interface counters resetting (e.g., container restarted).
	// We can't easily mock readInterfaceStats, but we can verify that a
	// counter decrease does not produce negative rates by setting prev
	// directly and checking the stored snapshot is updated without panic.
	// Since readInterfaceStats reads the real interface, we at least verify
	// the struct update path for a reset scenario using the previous value.
	beforeRx := c.RxBytes
	beforeTx := c.TxBytes

	_ = m.Collect(c)

	// After collect with real interface, rates should be non-negative.
	if c.CurrentRxMbps < 0 {
		t.Errorf("negative rx rate: %g", c.CurrentRxMbps)
	}
	if c.CurrentTxMbps < 0 {
		t.Errorf("negative tx rate: %g", c.CurrentTxMbps)
	}
	// Container counters are updated to the real interface values.
	if c.RxBytes == beforeRx {
		t.Error("rx bytes were not updated")
	}
	if c.TxBytes == beforeTx {
		t.Error("tx bytes were not updated")
	}
}
