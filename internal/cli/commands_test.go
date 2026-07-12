package cli

import (
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

// startMockDaemon starts a minimal Unix-socket responder that speaks the
// bandwidthd socket protocol. It returns the socket path and a cleanup func.
func startMockDaemon(t *testing.T, handler func(cmd string) (interface{}, error)) (string, func()) {
	t.Helper()
	socketPath := t.TempDir() + "/bandwidth-test.sock"

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	stopCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopCh:
				return
			default:
			}

			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-stopCh:
					return
				default:
					t.Logf("accept: %v", err)
					continue
				}
			}

			go func(c net.Conn) {
				defer c.Close()
				var req struct {
					Command string `json:"command"`
				}
				if err := json.NewDecoder(c).Decode(&req); err != nil {
					return
				}

				resp := socketResponse{OK: true}
				if handler != nil {
					data, err := handler(req.Command)
					if err != nil {
						resp.OK = false
						resp.Error = err.Error()
					} else if data != nil {
						raw, _ := json.Marshal(data)
						resp.Data = raw
					}
				}
				json.NewEncoder(c).Encode(resp)
			}(conn)
		}
	}()

	cleanup := func() {
		close(stopCh)
		ln.Close()
		os.Remove(socketPath)
	}

	// Give the goroutine a moment to start listening.
	time.Sleep(50 * time.Millisecond)
	return socketPath, cleanup
}

func TestSendCommandStatus(t *testing.T) {
	socketPath, cleanup := startMockDaemon(t, func(cmd string) (interface{}, error) {
		if cmd != "status" {
			t.Fatalf("expected status command, got %q", cmd)
		}
		return &models.DaemonStatus{
			Version:        "1.0.0",
			State:          "running",
			ContainerCount: 3,
		}, nil
	})
	defer cleanup()

	c := NewCLI(socketPath)
	resp, err := c.sendCommand("status", nil)
	if err != nil {
		t.Fatalf("sendCommand: %v", err)
	}
	if !resp.OK {
		t.Fatalf("response not ok: %s", resp.Error)
	}

	var status models.DaemonStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status.State != "running" {
		t.Fatalf("expected state running, got %q", status.State)
	}
	if status.ContainerCount != 3 {
		t.Fatalf("expected 3 containers, got %d", status.ContainerCount)
	}
}

func TestSendCommandNoDaemon(t *testing.T) {
	// Use a path that does not exist.
	c := NewCLI(t.TempDir() + "/nonexistent.sock")
	_, err := c.sendCommand("status", nil)
	if err == nil {
		t.Fatal("expected error when daemon is not listening")
	}
}

func TestIsDaemonRunning(t *testing.T) {
	socketPath, cleanup := startMockDaemon(t, func(cmd string) (interface{}, error) {
		return nil, nil
	})
	defer cleanup()

	c := NewCLI(socketPath)
	if !c.isDaemonRunning() {
		t.Fatal("expected daemon to be running")
	}

	c2 := NewCLI(t.TempDir() + "/nonexistent.sock")
	if c2.isDaemonRunning() {
		t.Fatal("expected daemon to not be running")
	}
}

func TestShortID12(t *testing.T) {
	if got := shortID12("abcdef1234567890abcdef"); got != "abcdef123456" {
		t.Fatalf("expected 12-char short id, got %q", got)
	}
	if got := shortID12("short"); got != "short" {
		t.Fatalf("expected unchanged short id, got %q", got)
	}
}

func TestExtractYAMLValue(t *testing.T) {
	cfg := `bandwidth:
  default_rx_mbps: 100
  default_tx_mbps: 50
# comment
quota:
  default_quota_gb: 500
nested:
  key: value
timezone: Asia/Kolkata
`
	if got := extractYAMLValue(cfg, "bandwidth.default_rx_mbps"); got != "100" {
		t.Errorf("expected 100, got %q", got)
	}
	if got := extractYAMLValue(cfg, "quota.default_quota_gb"); got != "500" {
		t.Errorf("expected 500, got %q", got)
	}
	if got := extractYAMLValue(cfg, "timezone"); got != "Asia/Kolkata" {
		t.Errorf("expected Asia/Kolkata, got %q", got)
	}
	if got := extractYAMLValue(cfg, "missing.key"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	// Should not cross-match keys from different sections.
	if got := extractYAMLValue(cfg, "nested.default_rx_mbps"); got != "" {
		t.Errorf("expected empty for cross-section key, got %q", got)
	}
}

func TestReplaceYAMLValue(t *testing.T) {
	cfg := "bandwidth:\n  default_rx_mbps: 100\n  default_tx_mbps: 50\nquota:\n  default_quota_gb: 500\ntimezone: Asia/Kolkata\n"

	got := replaceYAMLValue(cfg, "bandwidth.default_rx_mbps", "200")
	want := "bandwidth:\n  default_rx_mbps: 200\n  default_tx_mbps: 50\nquota:\n  default_quota_gb: 500\ntimezone: Asia/Kolkata\n"
	if got != want {
		t.Errorf("replace section key mismatch:\n got: %q\nwant: %q", got, want)
	}

	got2 := replaceYAMLValue(cfg, "timezone", "UTC")
	want2 := "bandwidth:\n  default_rx_mbps: 100\n  default_tx_mbps: 50\nquota:\n  default_quota_gb: 500\ntimezone: UTC\n"
	if got2 != want2 {
		t.Errorf("replace top-level key mismatch:\n got: %q\nwant: %q", got2, want2)
	}

	// Unknown key leaves string unchanged.
	if got := replaceYAMLValue(cfg, "unknown.key", "x"); got != cfg {
		t.Errorf("expected unchanged for unknown key, got %q", got)
	}
}
