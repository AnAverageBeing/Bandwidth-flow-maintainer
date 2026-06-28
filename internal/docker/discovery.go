// Package docker provides Docker container discovery and event watching.
// It interfaces with the Docker Engine via the docker CLI for maximum compatibility,
// avoiding Go SDK version mismatches with newer Docker Engine releases.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Discovery handles automatic Docker container detection and lifecycle tracking.
type Discovery struct {
	log      *logger.Logger
	interval time.Duration
	watch    bool

	mu         sync.RWMutex
	containers map[string]*models.Container
	callback   func(event DiscoveryEvent)
	stopCh     chan struct{}
}

// DiscoveryEvent represents a container lifecycle change.
type DiscoveryEvent struct {
	Type      string // "found", "removed", "started", "stopped", "restarted"
	Container *models.Container
}

// Config holds Docker connection parameters.
type Config struct {
	Endpoint    string
	APIVersion  string
	TLSVerify   bool
	TLSCertPath string
	Interval    time.Duration
	WatchEvents bool
}

// NewDiscovery creates a new Docker discovery service.
func NewDiscovery(cfg Config, log *logger.Logger) (*Discovery, error) {
	// Verify docker CLI is accessible
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("docker: docker CLI not found in PATH")
	}

	// Quick connectivity check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker: docker info failed: %v (output: %s)", err, strings.TrimSpace(string(out)))
	}

	return &Discovery{
		log:        log,
		interval:   cfg.Interval,
		watch:      cfg.WatchEvents,
		containers: make(map[string]*models.Container),
		stopCh:     make(chan struct{}),
	}, nil
}

// OnEvent registers a callback for container lifecycle events.
func (d *Discovery) OnEvent(cb func(event DiscoveryEvent)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callback = cb
}

// Discover performs a full scan of running Docker containers.
func (d *Discovery) Discover(ctx context.Context) ([]*models.Container, error) {
	// Get all container IDs using docker ps (full IDs with --no-trunc)
	cmd := exec.CommandContext(ctx, "docker", "ps", "-q", "--no-trunc")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker: ps: %w", err)
	}

	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}

	// First, inspect all containers WITHOUT holding the mutex
	// (inspectContainer needs d.mu.RLock which would deadlock if we hold Lock)
	type result struct {
		id   string
		cont *models.Container
		err  error
	}
	var results []result
	for _, id := range ids {
		cont, err := d.inspectContainer(ctx, id)
		results = append(results, result{id: id, cont: cont, err: err})
	}

	// Now update the map under lock
	d.mu.Lock()
	defer d.mu.Unlock()

	var discovered []*models.Container
	seen := make(map[string]bool)

	for _, r := range results {
		if r.err != nil {
			d.log.Warn("docker: inspect %s: %v", shortID(r.id), r.err)
			continue
		}

		seen[r.id] = true

		// Check if new
		if _, exists := d.containers[r.id]; !exists {
			d.log.Info("docker: found container %s (%s)", r.cont.Name, shortID(r.id))
			if d.callback != nil {
				d.callback(DiscoveryEvent{Type: "found", Container: r.cont})
			}
		}

		d.containers[r.id] = r.cont
		discovered = append(discovered, r.cont)
	}

	// Detect removed containers
	for id := range d.containers {
		if !seen[id] {
			if existing, ok := d.containers[id]; ok {
				d.log.Info("docker: container removed %s (%s)", existing.Name, shortID(id))
				if d.callback != nil {
					d.callback(DiscoveryEvent{Type: "removed", Container: existing})
				}
			}
			delete(d.containers, id)
		}
	}

	return discovered, nil
}

// WatchEvents starts listening for Docker container lifecycle events.
// Uses docker events command piped through.
func (d *Discovery) WatchEvents(ctx context.Context) error {
	if !d.watch {
		return nil
	}

	d.log.Info("docker: watching container events")

	cmd := exec.CommandContext(ctx, "docker", "events",
		"--filter", "type=container",
		"--filter", "event=start",
		"--filter", "event=die",
		"--filter", "event=destroy",
		"--filter", "event=pause",
		"--filter", "event=unpause",
		"--format", "{{json .}}",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker: events start: %w", err)
	}

	decoder := json.NewDecoder(stdout)
	go func() {
		for {
			select {
			case <-d.stopCh:
				cmd.Process.Kill()
				return
			case <-ctx.Done():
				cmd.Process.Kill()
				return
			default:
				var evt struct {
					Action string `json:"Action"`
					Actor  struct {
						ID string `json:"ID"`
					} `json:"Actor"`
				}
				if err := decoder.Decode(&evt); err != nil {
					return
				}
				d.handleEvent(ctx, evt.Action, evt.Actor.ID)
			}
		}
	}()

	return nil
}

func (d *Discovery) handleEvent(ctx context.Context, action, containerID string) {
	d.log.Debug("docker: event %s on %s", action, shortID(containerID))

	switch action {
	case "start", "unpause":
		cont, err := d.inspectContainer(ctx, containerID)
		if err != nil {
			d.log.Warn("docker: re-inspect %s: %v", shortID(containerID), err)
			return
		}
		d.mu.Lock()
		d.containers[containerID] = cont
		d.mu.Unlock()
		if d.callback != nil {
			d.callback(DiscoveryEvent{Type: "started", Container: cont})
		}
	case "die", "pause":
		d.mu.Lock()
		if cont, ok := d.containers[containerID]; ok {
			cont.State = models.StateStopped
			if d.callback != nil {
				d.callback(DiscoveryEvent{Type: "stopped", Container: cont})
			}
		}
		d.mu.Unlock()
	case "destroy":
		d.mu.Lock()
		delete(d.containers, containerID)
		d.mu.Unlock()
		d.log.Info("docker: container destroyed %s", shortID(containerID))
	}
}

// GetContainer returns a container by ID.
func (d *Discovery) GetContainer(id string) *models.Container {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.containers[id]
}

// ListContainers returns all known containers.
func (d *Discovery) ListContainers() []*models.Container {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*models.Container, 0, len(d.containers))
	for _, c := range d.containers {
		result = append(result, c)
	}
	return result
}

// Count returns the number of known containers.
func (d *Discovery) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.containers)
}

// Stop signals the discovery service to stop.
func (d *Discovery) Stop() {
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
}

// Close closes resources (no-op for CLI-based discovery).
func (d *Discovery) Close() error {
	d.Stop()
	return nil
}

// HealthCheck verifies the Docker daemon is reachable.
func (d *Discovery) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	return cmd.Run()
}

// ─── Private Helpers ──────────────────────────────────────────────────────────

// dockerInspectJSON is the JSON structure returned by `docker inspect`.
type dockerInspectJSON struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Status string `json:"Status"`
		Pid    int    `json:"Pid"`
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func (d *Discovery) inspectContainer(ctx context.Context, id string) (*models.Container, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", id)
	// Use Output() instead of CombinedOutput() to separate stderr (warnings)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker: inspect %s: %w", shortID(id), err)
	}

	// Find JSON array start in case stderr leaked into output
	start := strings.Index(string(out), "[")
	if start >= 0 {
		out = out[start:]
	}

	var inspects []dockerInspectJSON
	if err := json.Unmarshal(out, &inspects); err != nil {
		return nil, fmt.Errorf("docker: parse inspect: %w", err)
	}
	if len(inspects) == 0 {
		return nil, fmt.Errorf("docker: empty inspect for %s", shortID(id))
	}

	inspect := inspects[0]
	now := time.Now()

	c := &models.Container{
		ID:            inspect.ID,
		Name:          strings.TrimPrefix(inspect.Name, "/"),
		PID:           inspect.State.Pid,
		State:         dockerStateToModel(inspect.State.Status),
		Labels:        inspect.Config.Labels,
		RestartPolicy: inspect.HostConfig.RestartPolicy.Name,
		LastSeen:      now,
		FirstSeen:     now,
		Enabled:       true,
		History:       true,
	}

	// Detect veth interface
	c.VethInterface = detectVeth(inspect.State.Pid, inspect.ID)

	// Detect network name and IP
	for name, net := range inspect.NetworkSettings.Networks {
		c.NetworkName = name
		c.IPAddress = net.IPAddress
		break
	}

	// Map ports
	for portProto, bindings := range inspect.NetworkSettings.Ports {
		parts := strings.SplitN(portProto, "/", 2)
		containerPort, _ := strconv.Atoi(parts[0])
		protocol := "tcp"
		if len(parts) > 1 {
			protocol = parts[1]
		}
		for _, b := range bindings {
			hostPort, _ := strconv.Atoi(b.HostPort)
			c.Ports = append(c.Ports, models.PortMapping{
				ContainerPort: containerPort,
				HostPort:      hostPort,
				Protocol:      protocol,
				HostIP:        b.HostIP,
			})
		}
	}

	// Preserve FirstSeen from existing record
	d.mu.RLock()
	if existing, ok := d.containers[inspect.ID]; ok {
		c.FirstSeen = existing.FirstSeen
		d.mu.RUnlock()
	} else {
		d.mu.RUnlock()
	}

	// Apply label overrides
	applyLabels(c, inspect.Config.Labels)

	return c, nil
}

func dockerStateToModel(status string) models.ContainerState {
	switch status {
	case "running":
		return models.StateRunning
	case "exited", "dead":
		return models.StateStopped
	case "paused":
		return models.StatePaused
	default:
		return models.StateUnknown
	}
}

func detectVeth(pid int, containerID string) string {
	if pid <= 0 {
		return ""
	}

	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "veth") {
			continue
		}
		return entry.Name()
	}
	return ""
}

func applyLabels(c *models.Container, labels map[string]string) {
	if labels == nil {
		return
	}

	if v, ok := labels["bandwidth.enabled"]; ok {
		c.Enabled = v == "true" || v == "1"
	}

	if v, ok := labels["bandwidth.speed"]; ok {
		speed := parseSpeed(v)
		if speed > 0 {
			c.LimitRxMbps = speed
			c.LimitTxMbps = speed
		}
	}

	if v, ok := labels["bandwidth.daily_quota"]; ok {
		quota := parseQuota(v)
		if quota > 0 {
			c.DailyQuotaGB = quota
		}
	}

	if v, ok := labels["bandwidth.warning"]; ok {
		fmt.Sscanf(v, "%f", &c.WarningPercent)
	}

	if v, ok := labels["bandwidth.webhook"]; ok {
		c.Webhook = v == "true" || v == "1"
	}

	if v, ok := labels["bandwidth.history"]; ok {
		c.History = v != "false" && v != "0"
	}

	if v, ok := labels["bandwidth.priority"]; ok {
		c.Priority = v
	}
}

func parseSpeed(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	var value float64
	var unit string
	fmt.Sscanf(s, "%f%s", &value, &unit)
	switch unit {
	case "kbit", "kbps":
		return value / 1000
	case "mbit", "mbps", "":
		return value
	case "gbit", "gbps":
		return value * 1000
	default:
		return value
	}
}

func parseQuota(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	var value float64
	var unit string
	fmt.Sscanf(s, "%f%s", &value, &unit)
	switch unit {
	case "mb":
		return value / 1000
	case "gb", "":
		return value
	case "tb":
		return value * 1000
	default:
		return value
	}
}

// shortID returns a safe truncated container ID for logging.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
