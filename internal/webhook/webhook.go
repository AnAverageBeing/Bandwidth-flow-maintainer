package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/logger"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// Manager handles webhook dispatch to Discord, Slack, and generic HTTP endpoints.
type Manager struct {
	log       *logger.Logger
	enabled   bool
	endpoints []Endpoint
	queue     chan *webhookTask
	mu        sync.RWMutex
	stats     Stats
	wg        sync.WaitGroup
	stopCh    chan struct{}
}

// Endpoint represents a single webhook destination.
type Endpoint struct {
	Name       string
	URL        string
	Type       string // discord, slack, generic
	Enabled    bool
	MaxRetries int
	Backoff    time.Duration
	Timeout    time.Duration
}

// Stats tracks webhook delivery statistics.
type Stats struct {
	Sent    int64
	Failed  int64
	Retried int64
	mu      sync.Mutex
}

type webhookTask struct {
	Endpoint Endpoint
	Payload  models.WebhookPayload
	Attempt  int
	MaxTries int
}

// Config holds webhook manager settings.
type Config struct {
	Enabled   bool
	Endpoints []EndpointConfig
	QueueSize int
}

// EndpointConfig is a simplified endpoint definition from the main config.
type EndpointConfig struct {
	Name    string
	URL     string
	Type    string
	Enabled bool
}

// NewManager creates a new webhook manager.
func NewManager(cfg Config, log *logger.Logger) *Manager {
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = 10000
	}

	endpoints := make([]Endpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		if ep.URL == "" {
			continue
		}
		endpoints = append(endpoints, Endpoint{
			Name:       ep.Name,
			URL:        ep.URL,
			Type:       ep.Type,
			Enabled:    ep.Enabled,
			MaxRetries: 3,
			Backoff:    5 * time.Second,
			Timeout:    10 * time.Second,
		})
	}

	m := &Manager{
		log:       log,
		enabled:   cfg.Enabled && len(endpoints) > 0,
		endpoints: endpoints,
		queue:     make(chan *webhookTask, queueSize),
		stopCh:    make(chan struct{}),
	}

	if m.enabled {
		m.startWorkers(4)
	}

	return m
}

// Send dispatches a webhook event to all enabled endpoints.
func (m *Manager) Send(eventType models.EventType, containerName, message, severity string) {
	if !m.enabled {
		return
	}

	payload := models.WebhookPayload{
		Event:     eventType,
		Timestamp: time.Now().UTC(),
		Message:   message,
		Container: containerName,
		Severity:  severity,
	}

	for _, ep := range m.endpoints {
		if !ep.Enabled {
			continue
		}
		select {
		case m.queue <- &webhookTask{
			Endpoint: ep,
			Payload:  payload,
			MaxTries: ep.MaxRetries,
		}:
		default:
			m.log.Warn("webhook: queue full, dropping event %s", eventType)
		}
	}
}

// Stats returns the current webhook delivery statistics.
func (m *Manager) Stats() Stats {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()
	return m.stats
}

// Stop gracefully shuts down the webhook manager.
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	close(m.queue)
}

func (m *Manager) startWorkers(n int) {
	for i := 0; i < n; i++ {
		m.wg.Add(1)
		go m.worker()
	}
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stopCh:
			return
		case task, ok := <-m.queue:
			if !ok {
				return
			}
			m.deliver(task)
		}
	}
}

func (m *Manager) deliver(task *webhookTask) {
	body, err := m.formatPayload(task.Endpoint.Type, task.Payload)
	if err != nil {
		m.log.Error("webhook: format payload: %v", err)
		return
	}

	client := &http.Client{Timeout: task.Endpoint.Timeout}

	for attempt := 1; attempt <= task.MaxTries; attempt++ {
		req, err := http.NewRequest("POST", task.Endpoint.URL, bytes.NewReader(body))
		if err != nil {
			m.log.Error("webhook: create request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			m.log.Warn("webhook: attempt %d/%d to %s failed: %v", attempt, task.MaxTries, task.Endpoint.Name, err)
			if attempt < task.MaxTries {
				time.Sleep(task.Endpoint.Backoff * time.Duration(attempt))
			}
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			m.stats.mu.Lock()
			m.stats.Sent++
			m.stats.mu.Unlock()
			m.log.Debug("webhook: delivered to %s (HTTP %d)", task.Endpoint.Name, resp.StatusCode)
			return
		}

		m.log.Warn("webhook: %s returned HTTP %d (attempt %d/%d)", task.Endpoint.Name, resp.StatusCode, attempt, task.MaxTries)
		if attempt < task.MaxTries {
			time.Sleep(task.Endpoint.Backoff * time.Duration(attempt))
		}
	}

	m.stats.mu.Lock()
	m.stats.Failed++
	m.stats.mu.Unlock()
	m.log.Error("webhook: failed to deliver to %s after %d attempts", task.Endpoint.Name, task.MaxTries)
}

// formatPayload creates the JSON body based on the webhook type.
func (m *Manager) formatPayload(webhookType string, p models.WebhookPayload) ([]byte, error) {
	switch webhookType {
	case "discord":
		return m.discordPayload(p)
	case "slack":
		return m.slackPayload(p)
	default:
		return json.Marshal(p)
	}
}

func (m *Manager) discordPayload(p models.WebhookPayload) ([]byte, error) {
	color := 0x00FF00 // green default
	switch p.Severity {
	case "warning":
		color = 0xFFA500
	case "error", "critical":
		color = 0xFF0000
	}

	embed := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("[%s] %s", p.Event, p.Severity),
				"description": p.Message,
				"color":       color,
				"timestamp":   p.Timestamp.Format(time.RFC3339),
				"fields": []map[string]interface{}{
					{"name": "Container", "value": p.Container, "inline": true},
					{"name": "Event", "value": string(p.Event), "inline": true},
				},
			},
		},
	}
	return json.Marshal(embed)
}

func (m *Manager) slackPayload(p models.WebhookPayload) ([]byte, error) {
	msg := map[string]interface{}{
		"text": fmt.Sprintf("*[%s]* %s\nContainer: `%s` | Severity: `%s`",
			p.Event, p.Message, p.Container, p.Severity),
	}
	return json.Marshal(msg)
}

// Test sends a test webhook to verify endpoint connectivity.
func (m *Manager) Test(url, webhookType string) error {
	payload := models.WebhookPayload{
		Event:     "test",
		Timestamp: time.Now().UTC(),
		Message:   "Bandwidth Manager webhook test",
		Severity:  "info",
	}

	var body []byte
	var err error
	switch webhookType {
	case "discord":
		body, err = m.discordPayload(payload)
	case "slack":
		body, err = m.slackPayload(payload)
	default:
		body, err = json.Marshal(payload)
	}
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook test: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook test: HTTP %d", resp.StatusCode)
	}

	return nil
}
