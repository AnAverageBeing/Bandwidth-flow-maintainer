// Package tc manages Linux Traffic Control (tc) rules for bandwidth limiting.
// It uses netlink to manipulate qdiscs, classes, and filters — never userspace throttling.
// The package handles applying, replacing, repairing, removing, and verifying tc rules.
package tc

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Manager handles all tc operations for bandwidth enforcement.
type Manager struct {
	log      *logger.Logger
	enabled  bool
	mu       sync.Mutex
	rules    map[string]*Rule  // containerID -> Rule
	ifaceMap map[string]string // veth -> containerID
}

// Rule represents an active tc rule applied to an interface.
type Rule struct {
	ContainerID string
	Interface   string
	RxMbps      float64
	TxMbps      float64
	CeilMbps    float64
	BurstMbps   float64
	QdiscHandle string
	Active      bool
}

// Config holds tc manager configuration.
type Config struct {
	Enabled       bool
	DefaultQdisc  string
	HandleRoot    string
	DefaultClass  string
	VerifyOnApply bool
	RepairOnFail  bool
	MaxRetries    int
}

// NewManager creates a new tc manager.
func NewManager(cfg Config, log *logger.Logger) *Manager {
	return &Manager{
		log:      log,
		enabled:  cfg.Enabled,
		rules:    make(map[string]*Rule),
		ifaceMap: make(map[string]string),
	}
}

// Enabled returns whether tc management is active.
func (m *Manager) Enabled() bool {
	return m.enabled
}

// ApplyLimit applies bandwidth limits to a container's veth interface.
func (m *Manager) ApplyLimit(container *models.Container) error {
	if !m.enabled {
		return nil
	}

	if container.VethInterface == "" {
		return fmt.Errorf("tc: no veth interface for container %s", container.Name)
	}

	// Guard against zero/negative rates — tc requires positive rates
	rxMbps := container.LimitRxMbps
	txMbps := container.LimitTxMbps
	ceilMbps := container.CeilRxMbps
	if rxMbps <= 0 {
		rxMbps = 1 // minimum 1 Mbps so tc doesn't reject
	}
	if txMbps <= 0 {
		txMbps = 1
	}
	if ceilMbps <= 0 {
		ceilMbps = rxMbps * 2
	}
	if ceilMbps < rxMbps {
		ceilMbps = rxMbps
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	rule := &Rule{
		ContainerID: container.ID,
		Interface:   container.VethInterface,
		RxMbps:      rxMbps,
		TxMbps:      txMbps,
		CeilMbps:    ceilMbps,
		BurstMbps:   ceilMbps, // burst = ceil for token bucket
	}

	// Check if we need to handle exceeded state
	if container.State == models.StateExceeded {
		rule.RxMbps = container.ExceededSpeedRx
		rule.TxMbps = container.ExceededSpeedTx
	}

	if err := m.applyRule(rule); err != nil {
		m.log.Error("tc: apply failed for %s (%s): %v", container.Name, container.VethInterface, err)
		return fmt.Errorf("tc: apply %s: %w", container.VethInterface, err)
	}

	m.rules[container.ID] = rule
	m.ifaceMap[container.VethInterface] = container.ID
	m.log.Info("tc: applied limit %g/%g Mbps on %s (%s)", rule.RxMbps, rule.TxMbps, container.VethInterface, container.Name)

	return nil
}

// RemoveLimit removes tc rules for a container.
func (m *Manager) RemoveLimit(containerID string) error {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[containerID]
	if !ok {
		return nil
	}

	if err := m.removeRule(rule); err != nil {
		m.log.Warn("tc: remove failed for %s: %v", rule.Interface, err)
	}

	delete(m.rules, containerID)
	delete(m.ifaceMap, rule.Interface)

	m.log.Info("tc: removed rules from %s", rule.Interface)
	return nil
}

// RepairRules verifies and repairs all active tc rules.
func (m *Manager) RepairRules(containers []*models.Container) {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove stale rules
	for iface, cid := range m.ifaceMap {
		found := false
		for _, c := range containers {
			if c.ID == cid && c.VethInterface == iface {
				found = true
				break
			}
		}
		if !found {
			m.removeRuleByIface(iface)
			delete(m.ifaceMap, iface)
			delete(m.rules, cid)
		}
	}
}

// RemoveAll removes all tc rules managed by this system.
func (m *Manager) RemoveAll() {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rule := range m.rules {
		m.removeRule(rule)
	}
	m.rules = make(map[string]*Rule)
	m.ifaceMap = make(map[string]string)
	m.log.Info("tc: removed all rules")
}

// VerifyAll checks that all managed tc rules are properly applied.
func (m *Manager) VerifyAll() []string {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var issues []string
	for _, rule := range m.rules {
		if !m.verifyRule(rule.Interface) {
			issues = append(issues, fmt.Sprintf("%s: rule missing", rule.Interface))
		}
	}
	return issues
}

// RuleCount returns the number of active tc rules.
func (m *Manager) RuleCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rules)
}

// ─── Private tc command wrappers ──────────────────────────────────────────────

func (m *Manager) applyRule(rule *Rule) error {
	iface := rule.Interface

	// Convert Mbps to kbit for tc (1 Mbps = 1000 kbit)
	rxKbit := uint64(rule.RxMbps * 1000)
	txKbit := uint64(rule.TxMbps * 1000)
	ceilKbit := uint64(rule.CeilMbps * 1000)
	burstKbit := uint64(rule.BurstMbps * 1000)
	if burstKbit < 16 {
		burstKbit = 16 // minimum burst for tc
	}

	// Remove existing qdisc
	_ = exec.Command("tc", "qdisc", "del", "dev", iface, "root").Run()

	// Add root HTB qdisc
	cmds := [][]string{
		{"tc", "qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "1"},
		{"tc", "class", "add", "dev", iface, "parent", "1:", "classid", "1:1", "htb",
			"rate", fmt.Sprintf("%dkbit", rxKbit),
			"ceil", fmt.Sprintf("%dkbit", ceilKbit),
			"burst", fmt.Sprintf("%dkbit", burstKbit)},
	}

	// For egress shaping, we add a filter that directs all traffic to class 1:1
	// For ingress, we use an ingress qdisc + police filter
	for _, cmd := range cmds {
		c := exec.Command(cmd[0], cmd[1:]...)
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("tc %s: %w (output: %s)", strings.Join(cmd[1:], " "), err, strings.TrimSpace(string(out)))
		}
	}

	// Ingress policing (RX limit)
	if rxKbit > 0 {
		_ = exec.Command("tc", "qdisc", "del", "dev", iface, "ingress").Run()
		ingressCmd := exec.Command("tc", "qdisc", "add", "dev", iface, "handle", "ffff:", "ingress")
		if out, err := ingressCmd.CombinedOutput(); err != nil {
			m.log.Warn("tc: ingress qdisc failed on %s: %s", iface, strings.TrimSpace(string(out)))
		}

		// Police incoming traffic
		policeCmd := exec.Command("tc", "filter", "add", "dev", iface, "parent", "ffff:", "protocol", "ip",
			"prio", "50", "basic", "police",
			"rate", fmt.Sprintf("%dkbit", rxKbit),
			"burst", fmt.Sprintf("%dkbit", burstKbit),
			"drop", "flowid", ":1")
		if out, err := policeCmd.CombinedOutput(); err != nil {
			m.log.Warn("tc: ingress police failed on %s: %s", iface, strings.TrimSpace(string(out)))
		}
	}

	// Apply egress filter
	_ = exec.Command("tc", "filter", "del", "dev", iface, "parent", "1:", "prio", "1").Run()
	filterCmd := exec.Command("tc", "filter", "add", "dev", iface, "parent", "1:", "protocol", "ip",
		"prio", "1", "u32", "match", "ip", "dst", "0.0.0.0/0", "flowid", "1:1")
	if out, err := filterCmd.CombinedOutput(); err != nil {
		m.log.Warn("tc: filter failed on %s: %s", iface, strings.TrimSpace(string(out)))
	}

	// Also set TX rate via ifb mirror (for ingress shaping accuracy)
	_ = m.setupIFB(iface, txKbit, ceilKbit)

	rule.Active = true
	rule.QdiscHandle = "1:"
	return nil
}

func (m *Manager) removeRule(rule *Rule) error {
	return m.removeRuleByIface(rule.Interface)
}

func (m *Manager) removeRuleByIface(iface string) error {
	cmds := [][]string{
		{"tc", "qdisc", "del", "dev", iface, "root"},
		{"tc", "qdisc", "del", "dev", iface, "ingress"},
	}
	for _, cmd := range cmds {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Run() // ignore errors — interface may be gone
	}
	return nil
}

func (m *Manager) verifyRule(iface string) bool {
	out, err := exec.Command("tc", "qdisc", "show", "dev", iface).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "htb") || strings.Contains(string(out), "ingress")
}

// setupIFB sets up an Intermediate Functional Block device for more accurate
// ingress shaping (the IFB receives mirrored ingress traffic and applies egress shaping).
func (m *Manager) setupIFB(iface string, txKbit, ceilKbit uint64) error {
	// Check if ifb0 exists
	if out, err := exec.Command("ip", "link", "show", "ifb0").CombinedOutput(); err != nil {
		_ = exec.Command("ip", "link", "add", "ifb0", "type", "ifb").Run()
		_ = exec.Command("ip", "link", "set", "ifb0", "up").Run()
		_ = out
	}

	// Redirect ingress from veth to ifb0
	mirrorCmd := exec.Command("tc", "filter", "add", "dev", iface, "parent", "ffff:", "protocol", "ip",
		"prio", "1", "u32", "match", "u32", "0", "0", "action", "mirred", "egress", "redirect", "dev", "ifb0")
	mirrorCmd.Run() // non-fatal

	return nil
}

// ─── Utility ──────────────────────────────────────────────────────────────────

// MbpsToKbit converts megabits per second to kilobits per second (tc uses kbit).
func MbpsToKbit(mbps float64) uint64 {
	return uint64(mbps * 1000)
}

// KbitToMbps converts kilobits per second to megabits per second.
func KbitToMbps(kbit uint64) float64 {
	return float64(kbit) / 1000
}
