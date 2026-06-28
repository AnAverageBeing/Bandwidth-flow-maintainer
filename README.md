# Bandwidth Manager

**Production-grade Docker bandwidth management system** — per-container speed limiting, daily traffic quotas, automatic container discovery, Linux tc enforcement, TUI, webhook notifications, and more.

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

---

## Overview

Bandwidth Manager provides enterprise-grade bandwidth control for Docker containers. It's designed for free hosting platforms (similar to Pterodactyl) where hundreds or thousands of containers need individual bandwidth limits and daily quotas.

### Key Features

- **Per-container speed limiting** — RX/TX Mbps limits via Linux `tc` (HTB + ingress policing)
- **Daily traffic quotas** — Configurable per-container daily GB limits with automatic midnight reset
- **Auto-discovery** — Detects Docker containers, veth interfaces, ports, labels — zero manual registration
- **Docker label support** — `bandwidth.enabled`, `bandwidth.speed`, `bandwidth.daily_quota`, etc.
- **Quota throttling** — Reduces speed when quota exceeded (configurable, defaults to 1 Mbps)
- **Webhook notifications** — Discord, Slack, and generic HTTP webhooks for events
- **Historical statistics** — Daily, weekly, monthly, lifetime usage tracking with CSV/JSON export
- **Modern TUI** — Terminal UI with BubbleTea for real-time monitoring
- **Internal scheduler** — No cron required; handles resets, cleanup, health checks
- **Graceful recovery** — Survives Docker daemon restarts, crashes, and reboots
- **SQLite database** — Zero external dependencies
- **Systemd integration** — Runs as a proper Linux service

---

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                     bandwidth (CLI)                   │
│            Unix Socket (/var/run/bandwidth.sock)      │
└────────────────────────┬─────────────────────────────┘
                         │ JSON-RPC
┌────────────────────────▼─────────────────────────────┐
│                   bandwidthd (Daemon)                  │
│                                                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────┐ │
│  │ Docker   │  │ Monitor  │  │ Traffic Control (tc) │ │
│  │ Discovery│  │ (Polling)│  │ HTB + Ingress Police │ │
│  └──────────┘  └──────────┘  └──────────────────────┘ │
│                                                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────┐ │
│  │ Quota    │  │ Scheduler│  │ Webhook Manager      │ │
│  │ Manager  │  │ (Internal)│  │ (Discord/Slack/HTTP)│ │
│  └──────────┘  └──────────┘  └──────────────────────┘ │
│                                                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────┐ │
│  │ SQLite   │  │ Health   │  │ Cleanup               │ │
│  │ Database │  │ Checker  │  │ Manager               │ │
│  └──────────┘  └──────────┘  └──────────────────────┘ │
│                                                        │
│  ┌──────────┐  ┌──────────┐                            │
│  │ REST API │  │ Metrics  │  (Optional, disabled by    │
│  │ (opt)    │  │ (Prometheus)  default)                │
│  └──────────┘  └──────────┘                            │
└──────────────────────────────────────────────────────┘
```

---

## Quick Start

### Prerequisites

- Linux kernel 4.15+
- Docker Engine 20.10+
- Go 1.21+ (for building from source)
- Root access (for `tc` and Docker socket)

### Installation

```bash
# Clone the repository
git clone https://github.com/AnAverageBeing/Bandwidth-flow-maintainer.git
cd Bandwidth-flow-maintainer

# Run the installer (requires root)
sudo bash scripts/install.sh
```

The installer will:
1. Compile `bandwidth` and `bandwidthd` binaries
2. Install to `/usr/local/bin/`
3. Create `/etc/bandwidth/config.yaml`
4. Create `/var/lib/bandwidth/` and `/var/log/bandwidth/`
5. Install and start the systemd service

### Verify Installation

```bash
bandwidth version
bandwidth status
bandwidth list
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `bandwidth setup` | Interactive setup wizard |
| `bandwidth status` | Show daemon status |
| `bandwidth list` | List all managed containers |
| `bandwidth inspect <id>` | Inspect container details |
| `bandwidth stats` | Show bandwidth statistics |
| `bandwidth limits` | Show configured limits |
| `bandwidth history <id>` | Show usage history |
| `bandwidth doctor` | Run health diagnostics |
| `bandwidth reapply` | Reapply all tc rules |
| `bandwidth reset <target>` | Reset quota |
| `bandwidth webhook test` | Test webhook config |
| `bandwidth export [format]` | Export historical data |
| `bandwidth cleanup` | Run cleanup cycle |
| `bandwidth health` | Health check |
| `bandwidth version` | Show version |
| `bandwidth help` | Show help |

---

## Docker Labels

Override defaults per container using Docker labels:

```bash
docker run -d \
  --label bandwidth.enabled=true \
  --label bandwidth.speed=250mbit \
  --label bandwidth.daily_quota=500GB \
  --label bandwidth.warning=90 \
  --label bandwidth.webhook=true \
  --label bandwidth.priority=premium \
  nginx
```

| Label | Description | Example |
|-------|-------------|---------|
| `bandwidth.enabled` | Enable/disable | `true` / `false` |
| `bandwidth.speed` | Speed limit | `250mbit`, `1gbit`, `100` |
| `bandwidth.daily_quota` | Daily quota | `500GB`, `1TB`, `100` |
| `bandwidth.warning` | Warning threshold % | `90` |
| `bandwidth.webhook` | Send webhooks | `true` / `false` |
| `bandwidth.history` | Track history | `true` / `false` |
| `bandwidth.priority` | Priority tier | `premium`, `standard` |

---

## Configuration

Full configuration reference in [`configs/config.yaml`](configs/config.yaml).

Key sections:
- **general** — Socket paths, lock files
- **logging** — Log levels, rotation, format
- **database** — SQLite path, WAL settings
- **docker** — Docker endpoint, discovery interval
- **bandwidth** — Default speed limits, poll interval
- **quota** — Default quota, exceeded speed
- **webhook** — Discord/Slack endpoints, retry settings
- **scheduler** — Job intervals
- **traffic_control** — tc qdisc, verify, repair settings
- **timezone** — Timezone for midnight reset (default: `Asia/Kolkata`)

---

## Service Management

```bash
# Systemd
sudo systemctl status bandwidth
sudo systemctl stop bandwidth
sudo systemctl restart bandwidth
sudo journalctl -u bandwidth -f    # Follow logs

# CLI
bandwidth start
bandwidth stop
bandwidth restart
```

---

## How Traffic Control Works

The system uses Linux `tc` (traffic control) with HTB (Hierarchy Token Bucket) qdiscs:

1. **Egress shaping** — HTB qdisc on the container's veth interface with token bucket classes
2. **Ingress policing** — Ingress qdisc with police filters to rate-limit incoming traffic
3. **IFB mirroring** — Optional IFB device for more accurate ingress shaping
4. **Auto-repair** — Verifies and repairs rules automatically

Traffic is never shaped in userspace — everything happens at the kernel level for maximum efficiency.

---

## Rate Smoothing

Token bucket strategy prevents punishing short traffic bursts:

- **Peak Burst** — Allow bursts up to `ceil` rate for `peak_burst_seconds`
- **Sustained Rate** — 80% of configured limit as sustained rate
- **Recovery Window** — Time to refill burst tokens
- **Grace Window** — Initial grace period before enforcement

---

## Webhook Events

| Event | Trigger |
|-------|---------|
| `daemon_started` | Daemon starts |
| `daemon_stopped` | Daemon stops |
| `container_found` | New container discovered |
| `container_removed` | Container removed |
| `quota_warning` | Container at warning threshold |
| `quota_exceeded` | Container exceeded quota |
| `reset` | Daily quota reset |
| `error` | System error |
| `tc_failed` | tc rule application failed |

---

## Project Structure

```
Bandwidth-monitor/
├── cmd/
│   ├── bandwidth/          # CLI entry point
│   └── bandwidthd/         # Daemon entry point
├── internal/
│   ├── api/                # REST API (optional)
│   ├── cleanup/            # Stale record cleanup
│   ├── cli/                # CLI command implementations
│   ├── config/             # YAML config loader
│   ├── daemon/             # Core daemon orchestrator
│   ├── database/           # SQLite persistence
│   ├── docker/             # Container discovery
│   ├── health/             # Health checking
│   ├── logger/             # Structured logging
│   ├── metrics/            # Prometheus metrics
│   ├── monitor/            # Bandwidth polling
│   ├── quota/              # Quota management
│   ├── scheduler/          # Internal job scheduler
│   ├── service/            # Systemd integration
│   ├── tc/                 # Traffic control (tc)
│   ├── tui/                # Terminal UI (BubbleTea)
│   └── webhook/            # Webhook notifications
├── pkg/
│   └── models/             # Shared data types
├── configs/                # Example configuration
├── systemd/                # Systemd service file
├── scripts/                # Install script
├── Makefile                # Build system
├── go.mod                  # Go module definition
└── README.md               # This file
```

---

## Building from Source

```bash
# Install dependencies
make deps

# Build binaries
make build

# Run tests
make test

# Install locally
sudo make install
```

---

## Performance

Designed for servers running **1000+ Docker containers**:

- Minimal CPU usage (kernel-level tc, efficient polling)
- SQLite with WAL mode for concurrent reads
- Connection pooling
- No busy loops — ticker-based polling
- Batched database writes
- Memory-efficient data structures

---

## Security

- Validates all configuration before applying
- Prevents duplicate/invalid tc rules
- Mutex-based locking for thread safety
- Recovers after crashes and Docker restarts
- Never panics — all errors are handled gracefully
- Systemd security hardening (NoNewPrivileges, ProtectSystem, etc.)
- Optional API authentication

---

## License

MIT License — see [LICENSE](LICENSE) file.

---

## Contributing

Contributions are welcome! Please ensure:

1. Code follows idiomatic Go conventions
2. All packages have interfaces for testability
3. No global mutable state
4. Comprehensive error handling
5. Tests cover new functionality

---

**Built for production. Designed for scale.**
