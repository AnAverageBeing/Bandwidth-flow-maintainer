// bandwidthd — Docker bandwidth management daemon.
// Runs continuously, discovers containers, applies tc rules, tracks usage,
// enforces quotas, and exposes a Unix socket for CLI communication.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/config"
	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/daemon"
)

func main() {
	configPath := flag.String("config", "/etc/bandwidth/config.yaml", "Path to configuration file")
	flag.Parse()

	// Allow override via environment
	if envPath := os.Getenv("BANDWIDTH_CONFIG"); envPath != "" {
		*configPath = envPath
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Configuration errors:\n")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		os.Exit(1)
	}

	// Create daemon
	d, err := daemon.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize daemon: %v\n", err)
		os.Exit(1)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run daemon in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx)
	}()

	// Wait for signal or error
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
		}
	case sig := <-daemon.WaitForSignal():
		fmt.Fprintf(os.Stderr, "Received signal: %v\n", sig)
	}

	// Graceful shutdown
	d.Stop()
	fmt.Fprintf(os.Stderr, "Daemon stopped.\n")
}
