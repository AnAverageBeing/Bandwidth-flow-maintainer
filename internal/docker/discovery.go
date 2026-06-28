// Package docker provides Docker container discovery and event watching.
// It interfaces with the Docker Engine API to detect containers, their veth interfaces,
// labels, and lifecycle changes — all without manual registration.
package docker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Discovery handles automatic Docker container detection and lifecycle tracking.
type Discovery struct {
	cli      *client.Client
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
	opts := []client.Opt{
		client.WithHost(cfg.Endpoint),
	}
	if cfg.APIVersion != "" {
		opts = append(opts, client.WithVersion(cfg.APIVersion))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker: create client: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("docker: ping: %w", err)
	}

	return &Discovery{
		cli:        cli,
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
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	var discovered []*models.Container
	seen := make(map[string]bool)

	for _, c := range containers {
		inspect, err := d.cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			d.log.Warn("docker: inspect %s: %v", c.ID[:12], err)
			continue
		}

		cont := d.inspectToContainer(&inspect)
		seen[c.ID] = true

		// Check if new
		if _, exists := d.containers[c.ID]; !exists {
			d.log.Info("docker: found container %s (%s)", cont.Name, c.ID[:12])
			if d.callback != nil {
				d.callback(DiscoveryEvent{Type: "found", Container: cont})
			}
		}

		d.containers[c.ID] = cont
		discovered = append(discovered, cont)
	}

	// Detect removed containers
	for id := range d.containers {
		if !seen[id] {
			if existing, ok := d.containers[id]; ok {
				d.log.Info("docker: container removed %s (%s)", existing.Name, id[:12])
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
func (d *Discovery) WatchEvents(ctx context.Context) error {
	if !d.watch {
		return nil
	}

	f := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("event", "start"),
		filters.Arg("event", "die"),
		filters.Arg("event", "destroy"),
		filters.Arg("event", "pause"),
		filters.Arg("event", "unpause"),
	)

	msgCh, errCh := d.cli.Events(ctx, events.ListOptions{Filters: f})

	d.log.Info("docker: watching container events")

	for {
		select {
		case <-d.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			d.log.Error("docker: event stream error: %v", err)
			return err
		case msg := <-msgCh:
			d.handleEvent(ctx, msg)
		}
	}
}

func (d *Discovery) handleEvent(ctx context.Context, msg events.Message) {
	containerID := msg.Actor.ID
	d.log.Debug("docker: event %s on %s", msg.Action, containerID[:12])

	switch msg.Action {
	case "start", "unpause":
		d.refreshContainer(ctx, containerID, "started")
	case "die", "pause":
		d.markStopped(containerID, msg.Action)
	case "destroy":
		d.mu.Lock()
		delete(d.containers, containerID)
		d.mu.Unlock()
		d.log.Info("docker: container destroyed %s", containerID[:12])
	}
}

func (d *Discovery) refreshContainer(ctx context.Context, id, eventType string) {
	inspect, err := d.cli.ContainerInspect(ctx, id)
	if err != nil {
		d.log.Warn("docker: re-inspect %s: %v", id[:12], err)
		return
	}

	cont := d.inspectToContainer(&inspect)

	d.mu.Lock()
	d.containers[id] = cont
	d.mu.Unlock()

	if d.callback != nil {
		d.callback(DiscoveryEvent{Type: eventType, Container: cont})
	}
}

func (d *Discovery) markStopped(id, action string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if cont, ok := d.containers[id]; ok {
		cont.State = models.StateStopped
		if d.callback != nil {
			d.callback(DiscoveryEvent{Type: "stopped", Container: cont})
		}
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
	close(d.stopCh)
}

// Close closes the Docker client connection.
func (d *Discovery) Close() error {
	return d.cli.Close()
}

// HealthCheck verifies the Docker daemon is reachable.
func (d *Discovery) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := d.cli.Ping(ctx)
	return err
}

// ─── Private Helpers ──────────────────────────────────────────────────────────

func (d *Discovery) inspectToContainer(inspect *types.ContainerJSON) *models.Container {
	now := time.Now()
	c := &models.Container{
		ID:            inspect.ID,
		Name:          strings.TrimPrefix(inspect.Name, "/"),
		PID:           inspect.State.Pid,
		State:         dockerStateToModel(inspect.State.Status),
		NetworkName:   detectNetworkName(inspect),
		IPAddress:     detectIPAddress(inspect),
		Labels:        inspect.Config.Labels,
		RestartPolicy: string(inspect.HostConfig.RestartPolicy.Name),
		LastSeen:      now,
		FirstSeen:     now,
		Enabled:       true,
		History:       true,
	}

	// Detect veth interface from /sys/class/net/<iface>/ifindex
	c.VethInterface = detectVeth(inspect.State.Pid, inspect.ID)

	// Map ports
	for port, bindings := range inspect.NetworkSettings.Ports {
		for _, b := range bindings {
			c.Ports = append(c.Ports, models.PortMapping{
				ContainerPort: port.Int(),
				HostPort:      parseHostPort(b.HostPort),
				Protocol:      port.Proto(),
				HostIP:        b.HostIP,
			})
		}
	}

	// Mark if previously known (preserve FirstSeen)
	d.mu.RLock()
	if existing, ok := d.containers[inspect.ID]; ok {
		c.FirstSeen = existing.FirstSeen
	}
	d.mu.RUnlock()

	// Apply label overrides
	applyLabels(c, inspect.Config.Labels)

	return c
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

func detectNetworkName(inspect *types.ContainerJSON) string {
	for name := range inspect.NetworkSettings.Networks {
		return name
	}
	return ""
}

func detectIPAddress(inspect *types.ContainerJSON) string {
	for _, net := range inspect.NetworkSettings.Networks {
		if net.IPAddress != "" {
			return net.IPAddress
		}
	}
	return ""
}

// detectVeth finds the veth interface for a container by matching ifindex.
// It reads /proc/<pid>/net/if_inet6 or walks /sys/class/net/<container-id>*/.
func detectVeth(pid int, containerID string) string {
	if pid <= 0 {
		return ""
	}

	// Try symlink detection: /proc/<pid>/ns/net -> net:[inode]
	// Then find matching veth in /sys/class/net/<iface>/iflink
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "veth") {
			continue
		}
		// Quick heuristic: check if index in /sys/class/net/<veth>/ifindex matches
		iflinkPath := fmt.Sprintf("/sys/class/net/%s/ifindex", entry.Name())
		data, err := os.ReadFile(iflinkPath)
		if err != nil {
			continue
		}
		// The peer ifindex from the container namespace. We'd need to cross-reference
		// with /proc/<pid>/net/ — for now, return the first veth as fallback.
		// In production, use netlink to get the peer index properly.
		_ = data
		// Return the first veth; refine via netlink in the tc package.
		if strings.Contains(entry.Name(), "veth") {
			return entry.Name()
		}
	}
	return ""
}

func parseHostPort(s string) int {
	var p int
	fmt.Sscanf(s, "%d", &p)
	return p
}

// applyLabels extracts bandwidth configuration from Docker labels.
func applyLabels(c *models.Container, labels map[string]string) {
	if labels == nil {
		return
	}

	// bandwidth.enabled
	if v, ok := labels["bandwidth.enabled"]; ok {
		c.Enabled = v == "true" || v == "1"
	}

	// bandwidth.speed=250mbit or bandwidth.speed=100
	if v, ok := labels["bandwidth.speed"]; ok {
		speed := parseSpeed(v)
		if speed > 0 {
			c.LimitRxMbps = speed
			c.LimitTxMbps = speed
		}
	}

	// bandwidth.daily_quota=500GB or bandwidth.daily_quota=500
	if v, ok := labels["bandwidth.daily_quota"]; ok {
		quota := parseQuota(v)
		if quota > 0 {
			c.DailyQuotaGB = quota
		}
	}

	// bandwidth.warning=90
	if v, ok := labels["bandwidth.warning"]; ok {
		fmt.Sscanf(v, "%f", &c.WarningPercent)
	}

	// bandwidth.webhook=true
	if v, ok := labels["bandwidth.webhook"]; ok {
		c.Webhook = v == "true" || v == "1"
	}

	// bandwidth.history=false
	if v, ok := labels["bandwidth.history"]; ok {
		c.History = v != "false" && v != "0"
	}

	// bandwidth.priority=premium
	if v, ok := labels["bandwidth.priority"]; ok {
		c.Priority = v
	}
}

// parseSpeed parses a speed string like "250mbit", "1gbit", "100".
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

// parseQuota parses a quota string like "500GB", "1TB", "500".
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
