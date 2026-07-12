package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/database"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/docker"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/monitor"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/tc"
)

// Server provides an optional REST API for external monitoring and management.
type Server struct {
	log       *logger.Logger
	db        *database.DB
	discovery *docker.Discovery
	monitor   *monitor.Monitor
	tcMgr     *tc.Manager
	config    Config
	httpSrv   *http.Server
	mu        sync.RWMutex
}

// Config holds API server settings.
type Config struct {
	Enabled    bool
	SocketPath string
	TCPPort    int
	AuthToken  string
	ReadOnly   bool
}

// NewServer creates a new API server.
func NewServer(cfg Config, log *logger.Logger, db *database.DB, disc *docker.Discovery, mon *monitor.Monitor, tcMgr *tc.Manager) *Server {
	return &Server{
		log:       log,
		db:        db,
		discovery: disc,
		monitor:   mon,
		tcMgr:     tcMgr,
		config:    cfg,
	}
}

// Start begins listening for API requests.
func (s *Server) Start(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}

	mux := http.NewServeMux()

	// Health / status
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/status", s.handleStatus)

	// Containers
	mux.HandleFunc("/api/v1/containers", s.handleListContainers)
	mux.HandleFunc("/api/v1/containers/", s.handleContainer)

	// Stats
	mux.HandleFunc("/api/v1/stats", s.handleStats)

	// History
	mux.HandleFunc("/api/v1/history", s.handleHistory)

	// Events
	mux.HandleFunc("/api/v1/events", s.handleEvents)

	handler := s.authMiddleware(mux)

	listeners := make([]net.Listener, 0)

	// Unix socket
	if s.config.SocketPath != "" {
		_ = removeSocket(s.config.SocketPath)
		unixLn, err := net.Listen("unix", s.config.SocketPath)
		if err != nil {
			s.log.Warn("api: unix socket %s: %v", s.config.SocketPath, err)
		} else {
			listeners = append(listeners, unixLn)
			os.Chmod(s.config.SocketPath, 0666)
			s.log.Info("api: listening on unix://%s", s.config.SocketPath)
		}
	}

	// TCP
	if s.config.TCPPort > 0 {
		tcpLn, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.TCPPort))
		if err != nil {
			s.log.Warn("api: tcp port %d: %v", s.config.TCPPort, err)
		} else {
			listeners = append(listeners, tcpLn)
			s.log.Info("api: listening on :%d", s.config.TCPPort)
		}
	}

	if len(listeners) == 0 {
		return fmt.Errorf("api: no listeners available")
	}

	// Single server shared across all listeners so Stop() shuts everything down.
	s.httpSrv = &http.Server{Handler: handler}

	// Serve on all listeners
	for _, ln := range listeners {
		go func(l net.Listener) {
			if err := s.httpSrv.Serve(l); err != nil && err != http.ErrServerClosed {
				s.log.Error("api: serve: %v", err)
			}
		}(ln)
	}

	return nil
}

// Stop shuts down the API server.
func (s *Server) Stop() error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(context.Background())
	}
	return nil
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.config.AuthToken != "" {
			token := r.Header.Get("Authorization")
			expected := "Bearer " + s.config.AuthToken
			if token != expected {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	containers := s.discovery.ListContainers()
	writeJSON(w, map[string]interface{}{
		"containers":    len(containers),
		"tc_rules":      s.tcMgr.RuleCount(),
		"poll_interval": s.monitor.Interval().String(),
	})
}

func (s *Server) handleListContainers(w http.ResponseWriter, r *http.Request) {
	containers := s.discovery.ListContainers()
	writeJSON(w, map[string]interface{}{
		"containers": containers,
		"count":      len(containers),
	})
}

func (s *Server) handleContainer(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v1/containers/"):]
	if id == "" {
		http.Error(w, `{"error":"missing container id"}`, http.StatusBadRequest)
		return
	}

	// Find container
	for _, c := range s.discovery.ListContainers() {
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		if c.ID == id || shortID == id || c.Name == id {
			writeJSON(w, c)
			return
		}
	}

	http.Error(w, `{"error":"container not found"}`, http.StatusNotFound)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	containers := s.discovery.ListContainers()
	writeJSON(w, map[string]interface{}{
		"stats": containers,
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("container")
	if containerID == "" {
		http.Error(w, `{"error":"container query param required"}`, http.StatusBadRequest)
		return
	}
	// Delegate to DB
	writeJSON(w, map[string]interface{}{
		"container": containerID,
		"history":   []interface{}{},
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.db.GetRecentEvents(100)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{
		"events": events,
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func removeSocket(path string) error {
	// Best effort — the socket may not exist
	return os.Remove(path)
}
