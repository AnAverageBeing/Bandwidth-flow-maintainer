package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
)

// JobFunc is the function signature for scheduled job callbacks.
type JobFunc func(ctx context.Context) error

// ScheduledJob represents a recurring task.
type ScheduledJob struct {
	Name     string
	Interval time.Duration
	LastRun  time.Time
	Enabled  bool
	Fn       JobFunc
}

// Scheduler manages recurring jobs without external cron.
type Scheduler struct {
	log    *logger.Logger
	mu     sync.RWMutex
	jobs   map[string]*ScheduledJob
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Config holds scheduler parameters.
type Config struct {
	Enabled       bool
	CheckInterval time.Duration
}

// New creates a new internal scheduler.
func New(cfg Config, log *logger.Logger) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		log:    log,
		jobs:   make(map[string]*ScheduledJob),
		ctx:    ctx,
		cancel: cancel,
	}
}

// AddJob registers a recurring job. Interval must be > 0.
func (s *Scheduler) AddJob(name string, interval time.Duration, fn JobFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs[name] = &ScheduledJob{
		Name:     name,
		Interval: interval,
		Enabled:  true,
		Fn:       fn,
	}
	s.log.Info("scheduler: registered job %q every %s", name, interval)
}

// RunNow triggers a job immediately by name.
func (s *Scheduler) RunNow(name string) error {
	s.mu.RLock()
	job, ok := s.jobs[name]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("scheduler: job %q not found", name)
	}

	s.log.Info("scheduler: running job %q manually", name)
	return job.Fn(s.ctx)
}

// Start begins the scheduler loop. Checks jobs every 30 seconds.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		s.log.Info("scheduler: started")

		for {
			select {
			case <-s.ctx.Done():
				s.log.Info("scheduler: stopped")
				return
			case <-ticker.C:
				s.tick()
			}
		}
	}()
}

// Stop gracefully stops the scheduler, waiting for running jobs to finish.
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *Scheduler) tick() {
	s.mu.RLock()
	jobs := make([]*ScheduledJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	s.mu.RUnlock()

	now := time.Now()
	for _, job := range jobs {
		if !job.Enabled {
			continue
		}
		if job.LastRun.IsZero() || now.Sub(job.LastRun) >= job.Interval {
			s.log.Debug("scheduler: executing job %q", job.Name)
			if err := job.Fn(s.ctx); err != nil {
				s.log.Error("scheduler: job %q failed: %v", job.Name, err)
			}
			s.mu.Lock()
			job.LastRun = now
			s.mu.Unlock()
		}
	}
}

// ListJobs returns the names of all registered jobs.
func (s *Scheduler) ListJobs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.jobs))
	for name := range s.jobs {
		names = append(names, name)
	}
	return names
}
