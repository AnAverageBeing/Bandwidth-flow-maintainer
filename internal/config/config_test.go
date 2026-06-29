package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.General.SocketPath != "/var/run/bandwidth.sock" {
		t.Errorf("expected socket path /var/run/bandwidth.sock, got %s", cfg.General.SocketPath)
	}
	if cfg.Bandwidth.DefaultRxMbps != 100 {
		t.Errorf("expected default rx 100, got %g", cfg.Bandwidth.DefaultRxMbps)
	}
	if cfg.Quota.DefaultQuotaGB != 500 {
		t.Errorf("expected default quota 500, got %g", cfg.Quota.DefaultQuotaGB)
	}
	if cfg.Timezone != "Asia/Kolkata" {
		t.Errorf("expected timezone Asia/Kolkata, got %s", cfg.Timezone)
	}
	if cfg.TrafficControl.Enabled != true {
		t.Error("expected tc enabled by default")
	}
}

func TestDefaultConfigValidation(t *testing.T) {
	cfg := DefaultConfig()
	errs := cfg.Validate()
	if len(errs) > 0 {
		t.Errorf("default config should validate cleanly, got %d errors:", len(errs))
		for _, e := range errs {
			t.Logf("  - %v", e)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load should not error on missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load should return default config for missing file")
	}
	// Should have default values
	if cfg.Timezone != "Asia/Kolkata" {
		t.Errorf("expected default timezone, got %s", cfg.Timezone)
	}
}

func TestLoadValidConfig(t *testing.T) {
	yaml := `
general:
  socket_path: /tmp/test.sock
bandwidth:
  default_rx_mbps: 250
  default_tx_mbps: 250
quota:
  default_quota_gb: 100
timezone: UTC
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yaml)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Load valid config: %v", err)
	}

	if cfg.Bandwidth.DefaultRxMbps != 250 {
		t.Errorf("expected rx 250, got %g", cfg.Bandwidth.DefaultRxMbps)
	}
	if cfg.Quota.DefaultQuotaGB != 100 {
		t.Errorf("expected quota 100, got %g", cfg.Quota.DefaultQuotaGB)
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("expected UTC timezone, got %s", cfg.Timezone)
	}
	// Socket path should be overridden
	if cfg.General.SocketPath != "/tmp/test.sock" {
		t.Errorf("expected socket /tmp/test.sock, got %s", cfg.General.SocketPath)
	}
}

func TestValidateMissingSocketPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.General.SocketPath = ""
	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Error("expected validation error for missing socket path")
	}
}

func TestValidateNegativeBandwidth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Bandwidth.DefaultRxMbps = -1
	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Error("expected validation error for negative bandwidth")
	}
}

func TestGetTimezoneLocation(t *testing.T) {
	cfg := DefaultConfig()
	loc, err := cfg.GetTimezoneLocation()
	if err != nil {
		t.Fatalf("GetTimezoneLocation: %v", err)
	}
	if loc.String() != "Asia/Kolkata" {
		t.Errorf("expected Asia/Kolkata, got %s", loc.String())
	}
}
