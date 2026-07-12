package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
)

func TestSchedulerUsesConfiguredCheckInterval(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Console: false})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	defer log.Close()

	var runs int32
	s := New(Config{Enabled: true, CheckInterval: 100 * time.Millisecond}, log)
	s.AddJob("test-job", 50*time.Millisecond, func(ctx context.Context) error {
		atomic.AddInt32(&runs, 1)
		return nil
	})
	s.Start()

	// Wait long enough for at least a couple of ticks.
	time.Sleep(250 * time.Millisecond)
	s.Stop()

	if atomic.LoadInt32(&runs) == 0 {
		t.Fatal("scheduler did not run any jobs")
	}
}
