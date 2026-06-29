# Changelog

All notable changes to Bandwidth Manager.

## [1.1.0] — Unreleased

### Added
- Shell completion for bash, zsh, fish (`bandwidth completion <shell>`)
- GitHub Actions CI/CD: automated lint, test, multi-arch build, release
- Discord rich embeds with emoji, color-coded events, timestamps, footer branding
- Branded CLI output across all commands
- Zero-rate guard in tc manager (minimum 1 Mbps fallback)

### Fixed
- Systemd service compatibility (removed over-aggressive ProtectSystem)
- Interactive config now uses /dev/tty (works when piped from curl)
- Installer auto-clones repo when run via `curl | bash`
- Installer skips TUI test when no terminal (no more hangs)
- TC `rate 0kbit` error for containers with zero limits
- Graph TX overlay properly renders RX+TX simultaneously
- Webhook Stats mutex copy warning (go vet)
- Cross-architecture Go installation (amd64, arm64, arm)

### Changed
- TUI completely redesigned — btop-quality dashboard with braille graphs
- Unified theme system with consistent naming
- Help output categorized (Management/Monitoring/Containers/Other)
- Version output now shows developer credits

## [1.0.0] — 2026-06-28

### Initial Release
- Per-container bandwidth speed limiting via Linux tc (HTB + ingress)
- Per-container daily traffic quotas with automatic midnight reset
- Automatic Docker container discovery (zero manual registration)
- Docker label support for per-container overrides
- Modern TUI with BubbleTea for real-time monitoring
- Discord/Slack/generic HTTP webhook notifications
- Historical usage tracking (daily, weekly, monthly)
- CSV/JSON export
- REST API with token authentication
- Prometheus metrics endpoint
- Internal scheduler (no cron dependency)
- SQLite database with automatic migrations
- Systemd integration with graceful shutdown
- Interactive configuration wizard
- Health monitoring system
- Automatic cleanup of stale records and tc rules
