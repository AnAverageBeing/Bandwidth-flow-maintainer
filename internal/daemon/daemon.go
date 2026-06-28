package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/api"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/cleanup"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/config"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/database"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/docker"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/health"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/metrics"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/monitor"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/quota"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/scheduler"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/tc"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/webhook"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Daemon is the core orchestrator that ties all subsystems together.
type Daemon struct {
	cfg       *config.Config
	log       *logger.Logger
	db        *database.DB
	discovery *docker.Discovery
	tcMgr     *tc.Manager
	mon       *monitor.Monitor
	sched     *scheduler.Scheduler
	quotaMgr  *quota.Manager
	whMgr     *webhook.Manager
	cleanup   *cleanup.Manager
	health    *health.Checker
	apiSrv    *api.Server
	metrics   *metrics.Exporter

	startTime time.Time
	mu        sync.RWMutex
	stopCh    chan struct{}
	socketLn  net.Listener
}

// New creates and initializes all subsystems from configuration.
func New(cfg *config.Config) (*Daemon, error) {
	// Logger
	logCfg := logger.Config{
		Level:     cfg.Logging.Level,
		Console:   cfg.Logging.Console,
		File:      cfg.Logging.File,
		Format:    cfg.Logging.Format,
		MaxSizeMB: cfg.Logging.MaxSizeMB,
	}
	l, err := logger.New(logCfg)
	if err != nil {
		return nil, fmt.Errorf("daemon: logger: %w", err)
	}

	// Database
	dbCfg := database.Config{
		Path:         cfg.Database.Path,
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
		JournalMode:  cfg.Database.JournalMode,
		Synchronous:  cfg.Database.Synchronous,
		CacheSizeKB:  cfg.Database.CacheSizeKB,
	}
	db, err := database.Open(dbCfg)
	if err != nil {
		return nil, fmt.Errorf("daemon: database: %w", err)
	}
	l.Info("daemon: database opened at %s", cfg.Database.Path)

	// Docker discovery
	discCfg := docker.Config{
		Endpoint:    cfg.Docker.Endpoint,
		APIVersion:  cfg.Docker.APIVersion,
		TLSVerify:   cfg.Docker.TLSVerify,
		TLSCertPath: cfg.Docker.TLSCertPath,
		Interval:    cfg.Docker.DiscoveryInterval,
		WatchEvents: cfg.Docker.WatchEvents,
	}
	disc, err := docker.NewDiscovery(discCfg, l)
	if err != nil {
		return nil, fmt.Errorf("daemon: docker: %w", err)
	}

	// TC manager
	tcCfg := tc.Config{
		Enabled:       cfg.TrafficControl.Enabled,
		DefaultQdisc:  cfg.TrafficControl.DefaultQdisc,
		HandleRoot:    cfg.TrafficControl.HandleRoot,
		DefaultClass:  cfg.TrafficControl.DefaultClass,
		VerifyOnApply: cfg.TrafficControl.VerifyOnApply,
		RepairOnFail:  cfg.TrafficControl.RepairOnFail,
		MaxRetries:    cfg.TrafficControl.MaxRetries,
	}
	tcMgr := tc.NewManager(tcCfg, l)

	// Monitor
	monCfg := monitor.Config{
		PollInterval: cfg.Bandwidth.PollInterval,
	}
	mon := monitor.NewMonitor(monCfg, l)

	// Scheduler
	schedCfg := scheduler.Config{
		Enabled:       cfg.Scheduler.Enabled,
		CheckInterval: cfg.Scheduler.CheckInterval,
	}
	sched := scheduler.New(schedCfg, l)

	// Quota manager
	quotaCfg := quota.Config{
		ExceededSpeedRx: cfg.Quota.ExceededSpeedRx,
		ExceededSpeedTx: cfg.Quota.ExceededSpeedTx,
		DefaultQuotaGB:  cfg.Quota.DefaultQuotaGB,
		WarningPercent:  cfg.Quota.WarningPercent,
	}
	quotaMgr := quota.NewManager(quotaCfg, l)

	// Webhook manager
	var whEndpoints []webhook.EndpointConfig
	for _, ep := range cfg.Webhook.Endpoints {
		whEndpoints = append(whEndpoints, webhook.EndpointConfig{
			Name:    ep.Name,
			URL:     ep.URL,
			Type:    ep.Type,
			Enabled: ep.Enabled,
		})
	}
	whCfg := webhook.Config{
		Enabled:   cfg.Webhook.Enabled,
		Endpoints: whEndpoints,
		QueueSize: cfg.Webhook.QueueSize,
	}
	whMgr := webhook.NewManager(whCfg, l)

	// Cleanup
	cleanupMgr := cleanup.NewManager(cleanup.Config{
		Enabled:             cfg.Cleanup.Enabled,
		Interval:            cfg.Cleanup.Interval,
		StaleContainerHours: cfg.Cleanup.StaleContainerHours,
		RemoveTCRules:       cfg.Cleanup.RemoveTCRules,
		CompactDB:           cfg.Cleanup.CompactDB,
	}, l, db, tcMgr, disc)

	// Health checker
	healthChecker := health.NewChecker(l, db, disc, tcMgr)

	// API server
	apiSrv := api.NewServer(api.Config{
		Enabled:    cfg.API.Enabled,
		SocketPath: cfg.API.SocketPath,
		TCPPort:    cfg.API.TCPPort,
		AuthToken:  cfg.API.AuthToken,
		ReadOnly:   cfg.API.ReadOnly,
	}, l, db, disc, mon, tcMgr)

	// Metrics exporter
	metricsExp := metrics.NewExporter(metrics.Config{
		Enabled: cfg.Metrics.Enabled,
		Port:    cfg.Metrics.Port,
		Path:    cfg.Metrics.Path,
	}, l, disc, tcMgr, whMgr)

	return &Daemon{
		cfg:       cfg,
		log:       l,
		db:        db,
		discovery: disc,
		tcMgr:     tcMgr,
		mon:       mon,
		sched:     sched,
		quotaMgr:  quotaMgr,
		whMgr:     whMgr,
		cleanup:   cleanupMgr,
		health:    healthChecker,
		apiSrv:    apiSrv,
		metrics:   metricsExp,
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
	}, nil
}

// Start begins the main daemon loop.
func (d *Daemon) Start(ctx context.Context) error {
	d.log.Info("daemon: starting bandwidth manager v1.0.0")

	// Send startup webhook
	d.whMgr.Send(models.EventDaemonStarted, "", "Bandwidth manager daemon started", "info")

	// Initial discovery (with timeout)
	d.log.Info("daemon: starting initial discovery...")
	discoveryCtx, discoveryCancel := context.WithTimeout(ctx, 15*time.Second)
	containers, err := d.discovery.Discover(discoveryCtx)
	discoveryCancel()
	if err != nil {
		d.log.Error("daemon: initial discovery: %v", err)
	}
	d.log.Info("daemon: discovered %d containers", len(containers))

	// Apply config defaults to containers that don't have limits set
	for _, c := range containers {
		if c.LimitRxMbps <= 0 {
			c.LimitRxMbps = d.cfg.Bandwidth.DefaultRxMbps
		}
		if c.LimitTxMbps <= 0 {
			c.LimitTxMbps = d.cfg.Bandwidth.DefaultTxMbps
		}
		if c.CeilRxMbps <= 0 {
			c.CeilRxMbps = d.cfg.Bandwidth.DefaultCeilMbps
		}
		if c.CeilTxMbps <= 0 {
			c.CeilTxMbps = d.cfg.Bandwidth.DefaultCeilMbps
		}
		if c.DailyQuotaGB <= 0 {
			c.DailyQuotaGB = d.cfg.Quota.DefaultQuotaGB
		}
		if c.WarningPercent <= 0 {
			c.WarningPercent = d.cfg.Quota.WarningPercent
		}
		if c.ExceededSpeedRx <= 0 {
			c.ExceededSpeedRx = d.cfg.Quota.ExceededSpeedRx
		}
		if c.ExceededSpeedTx <= 0 {
			c.ExceededSpeedTx = d.cfg.Quota.ExceededSpeedTx
		}
	}

	// Persist containers to DB
	for _, c := range containers {
		d.db.UpsertContainer(c)
	}

	// Apply tc rules to all enabled containers
	for _, c := range containers {
		if c.Enabled && c.State == models.StateRunning {
			if err := d.tcMgr.ApplyLimit(c); err != nil {
				d.log.Warn("daemon: tc apply %s: %v", c.Name, err)
			}
		}
	}

	// Register scheduled jobs
	d.sched.AddJob("discovery", d.cfg.Docker.DiscoveryInterval, func(ctx context.Context) error {
		containers, err := d.discovery.Discover(ctx)
		if err != nil {
			return err
		}
		for _, c := range containers {
			d.db.UpsertContainer(c)
		}
		d.log.Debug("daemon: discovery found %d containers", len(containers))
		return nil
	})

	d.sched.AddJob("quota-reset", 60*time.Second, func(ctx context.Context) error {
		loc, err := d.cfg.GetTimezoneLocation()
		if err != nil {
			return err
		}
		if d.quotaMgr.ShouldReset(loc) {
			containers := d.discovery.ListContainers()
			count := d.quotaMgr.ResetDaily(containers)
			d.db.ResetDailyUsage()
			d.whMgr.Send(models.EventReset, "", fmt.Sprintf("Daily quota reset: %d containers", count), "info")
			// Re-apply tc rules to restore original speeds
			for _, c := range containers {
				d.tcMgr.ApplyLimit(c)
			}
		}
		return nil
	})

	d.sched.AddJob("cleanup", d.cfg.Cleanup.Interval, func(ctx context.Context) error {
		return d.cleanup.Run(ctx)
	})

	d.sched.AddJob("health-check", 5*time.Minute, func(ctx context.Context) error {
		report := d.health.Run()
		d.log.Debug("daemon: health: %s", report.Overall)
		return nil
	})

	d.sched.Start()

	// Start API if enabled
	if err := d.apiSrv.Start(ctx); err != nil {
		d.log.Warn("daemon: api: %v", err)
	}

	// Start metrics if enabled
	if err := d.metrics.Start(); err != nil {
		d.log.Warn("daemon: metrics: %v", err)
	}

	// Start Unix socket listener for CLI
	if err := d.startSocketListener(); err != nil {
		d.log.Warn("daemon: socket listener: %v", err)
	}

	// Start Docker event watcher
	go func() {
		if err := d.discovery.WatchEvents(ctx); err != nil {
			d.log.Error("daemon: docker events: %v", err)
		}
	}()

	// Main polling loop
	pollTicker := time.NewTicker(d.cfg.Bandwidth.PollInterval)
	defer pollTicker.Stop()

	d.log.Info("daemon: running (poll interval: %s)", d.cfg.Bandwidth.PollInterval)

	for {
		select {
		case <-d.stopCh:
			d.log.Info("daemon: stopping")
			return nil
		case <-ctx.Done():
			d.log.Info("daemon: context cancelled")
			return ctx.Err()
		case <-pollTicker.C:
			d.poll(ctx)
		}
	}
}

func (d *Daemon) poll(ctx context.Context) {
	containers := d.discovery.ListContainers()

	for _, c := range containers {
		if c.State != models.StateRunning {
			continue
		}
		if !c.Enabled {
			continue
		}

		// Collect stats
		if err := d.mon.Collect(c); err != nil {
			d.log.Debug("daemon: collect %s: %v", c.Name, err)
			continue
		}

		// Check quota
		exceeded, warning := d.quotaMgr.CheckQuota(c)
		if exceeded {
			if c.State != models.StateExceeded {
				d.quotaMgr.ApplyExceededThrottle(c)
				d.tcMgr.ApplyLimit(c)
				d.whMgr.Send(models.EventQuotaExceeded, c.Name,
					fmt.Sprintf("Quota exceeded: %.2f/%.2f GB", c.TodayRxGB+c.TodayTxGB, c.DailyQuotaGB), "error")
			}
		} else if warning {
			d.whMgr.Send(models.EventQuotaWarning, c.Name,
				fmt.Sprintf("Quota warning: %.2f/%.2f GB", c.TodayRxGB+c.TodayTxGB, c.DailyQuotaGB), "warning")
		}

		// Persist usage
		d.db.InsertUsage(c.ID, 0, c.RxBytes, c.TxBytes, c.CurrentRxMbps, c.CurrentTxMbps)
		d.db.UpdateContainerUsage(c.ID, c.RxBytes, c.TxBytes, c.CurrentRxMbps, c.CurrentTxMbps, c.TodayRxGB, c.TodayTxGB)
	}
}

// Stop gracefully shuts down the daemon and all subsystems.
func (d *Daemon) Stop() {
	d.log.Info("daemon: shutting down")

	d.whMgr.Send(models.EventDaemonStopped, "", "Bandwidth manager daemon stopping", "info")

	// Stop socket listener
	if d.socketLn != nil {
		d.socketLn.Close()
	}

	// Stop scheduler
	d.sched.Stop()

	// Stop metrics
	d.metrics.Stop()

	// Stop API
	d.apiSrv.Stop()

	// Remove all tc rules
	d.tcMgr.RemoveAll()

	// Stop webhook manager
	d.whMgr.Stop()

	// Close docker client
	d.discovery.Close()

	// Close database
	d.db.Close()

	// Close logger
	d.log.Close()

	close(d.stopCh)
}

// ─── Unix Socket Protocol ─────────────────────────────────────────────────────

type socketRequest struct {
	Command string          `json:"command"`
	Args    json.RawMessage `json:"args,omitempty"`
}

type socketResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

func (d *Daemon) startSocketListener() error {
	socketPath := d.cfg.General.SocketPath
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("daemon: listen socket: %w", err)
	}
	d.socketLn = ln

	// Set permissions so CLI can connect (world-readable/writable for all users)
	os.Chmod(socketPath, 0666)

	go func() {
		d.log.Info("daemon: listening on %s", socketPath)
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-d.stopCh:
					return
				default:
					d.log.Error("daemon: socket accept: %v", err)
					continue
				}
			}
			go d.handleSocketConn(conn)
		}
	}()

	return nil
}

func (d *Daemon) handleSocketConn(conn net.Conn) {
	defer conn.Close()

	var req socketRequest
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&req); err != nil {
		d.writeResponse(conn, false, nil, fmt.Sprintf("invalid request: %v", err))
		return
	}

	var data interface{}
	var err error

	switch req.Command {
	case "status":
		data = d.getStatus()
	case "doctor":
		data = d.health.Run()
	case "list":
		containers := d.discovery.ListContainers()
		data = map[string]interface{}{"containers": containers, "count": len(containers)}
	case "reapply":
		err = d.reapply()
	case "reload":
		err = d.reload()
	case "stats":
		containers := d.discovery.ListContainers()
		data = containers
	case "health":
		data = d.health.Run()
	case "daemon":
		data = d.getStatus()
	case "limits":
		containers := d.discovery.ListContainers()
		limits := make([]map[string]interface{}, 0)
		for _, c := range containers {
			limits = append(limits, map[string]interface{}{
				"container": c.Name,
				"rx_mbps":   c.LimitRxMbps,
				"tx_mbps":   c.LimitTxMbps,
				"quota_gb":  c.DailyQuotaGB,
				"used_gb":   c.TodayRxGB + c.TodayTxGB,
			})
		}
		data = limits
	default:
		err = fmt.Errorf("unknown command: %s", req.Command)
	}

	if err != nil {
		d.writeResponse(conn, false, nil, err.Error())
	} else {
		d.writeResponse(conn, true, data, "")
	}
}

func (d *Daemon) writeResponse(conn net.Conn, ok bool, data interface{}, errMsg string) {
	resp := socketResponse{OK: ok, Error: errMsg}
	if data != nil {
		raw, _ := json.Marshal(data)
		resp.Data = raw
	}
	json.NewEncoder(conn).Encode(resp)
}

func (d *Daemon) getStatus() *models.DaemonStatus {
	containers := d.discovery.ListContainers()
	exceeded := 0
	for _, c := range containers {
		if c.State == models.StateExceeded {
			exceeded++
		}
	}

	return &models.DaemonStatus{
		Version:         "1.0.0",
		Uptime:          time.Since(d.startTime).String(),
		StartTime:       d.startTime,
		State:           "running",
		ContainerCount:  len(containers),
		ManagedCount:    d.tcMgr.RuleCount(),
		ExceededCount:   exceeded,
		DockerHealthy:   d.discovery.HealthCheck(context.Background()) == nil,
		DatabaseOK:      d.db.Ping() == nil,
		TCHealthy:       len(d.tcMgr.VerifyAll()) == 0,
		TCRulesApplied:  int64(d.tcMgr.RuleCount()),
		PollIntervalSec: int(d.cfg.Bandwidth.PollInterval.Seconds()),
		Timezone:        d.cfg.Timezone,
		LastReset:       d.quotaMgr.LastReset(),
	}
}

func (d *Daemon) reapply() error {
	containers := d.discovery.ListContainers()
	for _, c := range containers {
		if c.Enabled && c.State == models.StateRunning {
			if err := d.tcMgr.ApplyLimit(c); err != nil {
				return fmt.Errorf("reapply %s: %w", c.Name, err)
			}
		}
	}
	d.log.Info("daemon: reapplied rules to all containers")
	return nil
}

func (d *Daemon) reload() error {
	d.log.Info("daemon: config reload requested — restart daemon to apply")
	return nil
}

// WaitForSignal blocks until SIGTERM or SIGINT is received.
func WaitForSignal() os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	return <-sigCh
}
