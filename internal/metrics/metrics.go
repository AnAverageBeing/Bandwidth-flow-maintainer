package metrics

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/docker"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/tc"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/webhook"
)

// Exporter serves Prometheus-compatible metrics.
type Exporter struct {
	log       *logger.Logger
	discovery *docker.Discovery
	tcMgr     *tc.Manager
	webhook   *webhook.Manager
	config    Config
	httpSrv   *http.Server
	mu        sync.RWMutex
}

// Config holds metrics exporter settings.
type Config struct {
	Enabled bool
	Port    int
	Path    string
}

// NewExporter creates a metrics exporter.
func NewExporter(cfg Config, log *logger.Logger, disc *docker.Discovery, tcMgr *tc.Manager, wh *webhook.Manager) *Exporter {
	if cfg.Path == "" {
		cfg.Path = "/metrics"
	}
	return &Exporter{
		log:       log,
		discovery: disc,
		tcMgr:     tcMgr,
		webhook:   wh,
		config:    cfg,
	}
}

// Start begins serving Prometheus metrics.
func (e *Exporter) Start() error {
	if !e.config.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(e.config.Path, e.handleMetrics)

	e.httpSrv = &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	go func() {
		e.log.Info("metrics: listening on :%d%s", e.config.Port, e.config.Path)
		if err := e.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.log.Error("metrics: serve: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the metrics server.
func (e *Exporter) Stop() error {
	if e.httpSrv != nil {
		return e.httpSrv.Close()
	}
	return nil
}

func (e *Exporter) handleMetrics(w http.ResponseWriter, r *http.Request) {
	containers := e.discovery.ListContainers()
	whStats := e.webhook.Stats()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// Container metrics
	fmt.Fprintf(w, "# HELP bandwidth_containers_total Total discovered containers\n")
	fmt.Fprintf(w, "# TYPE bandwidth_containers_total gauge\n")
	fmt.Fprintf(w, "bandwidth_containers_total %d\n\n", len(containers))

	fmt.Fprintf(w, "# HELP bandwidth_tc_rules_total Active tc rules\n")
	fmt.Fprintf(w, "# TYPE bandwidth_tc_rules_total gauge\n")
	fmt.Fprintf(w, "bandwidth_tc_rules_total %d\n\n", e.tcMgr.RuleCount())

	// Per-container metrics
	fmt.Fprintf(w, "# HELP bandwidth_current_rx_mbps Current RX Mbps per container\n")
	fmt.Fprintf(w, "# TYPE bandwidth_current_rx_mbps gauge\n")
	for _, c := range containers {
		fmt.Fprintf(w, `bandwidth_current_rx_mbps{container="%s",name="%s"} %.2f`+"\n", shortID(c.ID), c.Name, c.CurrentRxMbps)
	}

	fmt.Fprintf(w, "\n# HELP bandwidth_current_tx_mbps Current TX Mbps per container\n")
	fmt.Fprintf(w, "# TYPE bandwidth_current_tx_mbps gauge\n")
	for _, c := range containers {
		fmt.Fprintf(w, `bandwidth_current_tx_mbps{container="%s",name="%s"} %.2f`+"\n", shortID(c.ID), c.Name, c.CurrentTxMbps)
	}

	fmt.Fprintf(w, "\n# HELP bandwidth_today_usage_gb Today's usage in GB\n")
	fmt.Fprintf(w, "# TYPE bandwidth_today_usage_gb gauge\n")
	for _, c := range containers {
		fmt.Fprintf(w, `bandwidth_today_usage_gb{container="%s",name="%s",direction="rx"} %.4f`+"\n", shortID(c.ID), c.Name, c.TodayRxGB)
		fmt.Fprintf(w, `bandwidth_today_usage_gb{container="%s",name="%s",direction="tx"} %.4f`+"\n", shortID(c.ID), c.Name, c.TodayTxGB)
	}

	// Webhook metrics
	fmt.Fprintf(w, "\n# HELP bandwidth_webhooks_sent_total Total webhooks successfully sent\n")
	fmt.Fprintf(w, "# TYPE bandwidth_webhooks_sent_total counter\n")
	fmt.Fprintf(w, "bandwidth_webhooks_sent_total %d\n\n", whStats.Sent)

	fmt.Fprintf(w, "# HELP bandwidth_webhooks_failed_total Total webhook failures\n")
	fmt.Fprintf(w, "# TYPE bandwidth_webhooks_failed_total counter\n")
	fmt.Fprintf(w, "bandwidth_webhooks_failed_total %d\n", whStats.Failed)
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
