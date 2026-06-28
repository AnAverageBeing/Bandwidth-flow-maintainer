package service

import (
	"fmt"
	"os"
	"os/exec"
)

// Manager handles systemd service operations for the bandwidth daemon.
type Manager struct {
	serviceName string
}

// NewManager creates a service manager.
func NewManager() *Manager {
	return &Manager{serviceName: "bandwidth"}
}

// Install copies the systemd service file and enables the service.
func (m *Manager) Install() error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("service install requires root privileges")
	}

	servicePath := "/etc/systemd/system/bandwidth.service"

	// Check if service file exists
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return fmt.Errorf("service file not found at %s — run install.sh first", servicePath)
	}

	// Reload systemd
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w\nOutput: %s", err, string(out))
	}

	// Enable service
	if out, err := exec.Command("systemctl", "enable", "bandwidth.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %w\nOutput: %s", err, string(out))
	}

	fmt.Println("Service enabled successfully.")
	return nil
}

// Start starts the bandwidth daemon via systemd.
func (m *Manager) Start() error {
	if out, err := exec.Command("systemctl", "start", "bandwidth.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl start: %w\nOutput: %s", err, string(out))
	}
	fmt.Println("Bandwidth daemon started.")
	return nil
}

// Stop stops the bandwidth daemon.
func (m *Manager) Stop() error {
	if out, err := exec.Command("systemctl", "stop", "bandwidth.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl stop: %w\nOutput: %s", err, string(out))
	}
	fmt.Println("Bandwidth daemon stopped.")
	return nil
}

// Restart restarts the bandwidth daemon.
func (m *Manager) Restart() error {
	if out, err := exec.Command("systemctl", "restart", "bandwidth.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart: %w\nOutput: %s", err, string(out))
	}
	fmt.Println("Bandwidth daemon restarted.")
	return nil
}

// Status shows the daemon's systemd status.
func (m *Manager) Status() error {
	cmd := exec.Command("systemctl", "status", "bandwidth.service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Logs shows recent daemon logs via journalctl.
func (m *Manager) Logs() error {
	cmd := exec.Command("journalctl", "-u", "bandwidth.service", "-n", "50", "--no-pager")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// IsRunning checks if the daemon is running.
func (m *Manager) IsRunning() bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "bandwidth.service").Run()
	return err == nil
}
