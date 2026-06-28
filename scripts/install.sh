#!/usr/bin/env bash
# =============================================================================
# Bandwidth Manager — Production Installer
# Installs /usr/local/bin/bandwidth (CLI) and /usr/local/bin/bandwidthd (daemon)
# Creates config, database, logs, and systemd service.
# =============================================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

log()  { echo -e "${CYAN}[INSTALL]${NC} $*"; }
ok()   { echo -e "${GREEN}[  OK  ]${NC} $*"; }
warn() { echo -e "${YELLOW}[ WARN ]${NC} $*"; }
err()  { echo -e "${RED}[ERROR ]${NC} $*"; exit 1; }

# ─── Privilege Check ──────────────────────────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    err "This installer must be run as root (sudo ./install.sh)"
fi

# ─── Paths ────────────────────────────────────────────────────────────────────
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/bandwidth"
DATA_DIR="/var/lib/bandwidth"
LOG_DIR="/var/log/bandwidth"
RUN_DIR="/var/run"
SYSTEMD_DIR="/etc/systemd/system"
SOURCE_DIR="$(cd "$(dirname "$0")/.." && pwd)"

log "Bandwidth Manager Installer"
log "Source directory: $SOURCE_DIR"
log ""

# ─── Step 1: Compile ──────────────────────────────────────────────────────────
log "Step 1/8: Compiling binaries..."

if ! command -v go &>/dev/null; then
    err "Go is not installed. Please install Go 1.21+ first."
fi

cd "$SOURCE_DIR"

log "  → Building bandwidth (CLI)..."
go build -o build/bandwidth -ldflags="-s -w" ./cmd/bandwidth/ || err "Failed to build bandwidth CLI"

log "  → Building bandwidthd (daemon)..."
go build -o build/bandwidthd -ldflags="-s -w" ./cmd/bandwidthd/ || err "Failed to build bandwidthd daemon"

ok "Binaries compiled successfully"

# ─── Step 2: Install Binaries ─────────────────────────────────────────────────
log "Step 2/8: Installing binaries to $INSTALL_DIR..."

cp -f build/bandwidth "$INSTALL_DIR/bandwidth"
cp -f build/bandwidthd "$INSTALL_DIR/bandwidthd"
chmod 755 "$INSTALL_DIR/bandwidth" "$INSTALL_DIR/bandwidthd"

ok "Binaries installed"

# ─── Step 3: Create Directories ───────────────────────────────────────────────
log "Step 3/8: Creating directories..."

mkdir -p "$CONFIG_DIR"
mkdir -p "$DATA_DIR"
mkdir -p "$LOG_DIR"
mkdir -p "$RUN_DIR"

chmod 755 "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"

ok "Directories created"

# ─── Step 4: Create Configuration ─────────────────────────────────────────────
log "Step 4/8: Setting up configuration..."

if [ -f "$CONFIG_DIR/config.yaml" ]; then
    warn "Configuration already exists at $CONFIG_DIR/config.yaml"
    warn "Skipping — to regenerate, remove the file and re-run install.sh"
else
    if [ -f "$SOURCE_DIR/configs/config.yaml" ]; then
        cp "$SOURCE_DIR/configs/config.yaml" "$CONFIG_DIR/config.yaml"
        ok "Configuration installed to $CONFIG_DIR/config.yaml"
    else
        warn "No config template found — daemon will use defaults"
    fi
fi

# ─── Step 5: Create Database ──────────────────────────────────────────────────
log "Step 5/8: Database will be auto-created on first daemon start"
ok "Database path: $DATA_DIR/bandwidth.db (auto-created)"

# ─── Step 6: Install Systemd Service ──────────────────────────────────────────
log "Step 6/8: Installing systemd service..."

if [ -f "$SOURCE_DIR/systemd/bandwidth.service" ]; then
    cp "$SOURCE_DIR/systemd/bandwidth.service" "$SYSTEMD_DIR/bandwidth.service"
    chmod 644 "$SYSTEMD_DIR/bandwidth.service"
    systemctl daemon-reload
    ok "Systemd service installed"
else
    err "systemd/bandwidth.service not found in source directory"
fi

# ─── Step 7: Start Daemon ─────────────────────────────────────────────────────
log "Step 7/8: Starting daemon..."

systemctl enable bandwidth.service 2>/dev/null || warn "Could not enable service (may already be enabled)"
systemctl start bandwidth.service 2>/dev/null || warn "Could not start service"

# Give it a moment to start
sleep 2

if systemctl is-active --quiet bandwidth.service; then
    ok "Daemon is running"
else
    warn "Daemon may not have started — check: systemctl status bandwidth"
fi

# ─── Step 8: Verify Installation ──────────────────────────────────────────────
log "Step 8/8: Verifying installation..."

VERIFY_OK=true

# Check binaries
for bin in bandwidth bandwidthd; do
    if [ -x "$INSTALL_DIR/$bin" ]; then
        ok "Binary: $INSTALL_DIR/$bin"
    else
        err "Missing binary: $INSTALL_DIR/$bin"
        VERIFY_OK=false
    fi
done

# Check config
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    ok "Config: $CONFIG_DIR/config.yaml"
else
    warn "Config: $CONFIG_DIR/config.yaml (missing — using defaults)"
fi

# Check directories
for dir in "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"; do
    if [ -d "$dir" ]; then
        ok "Directory: $dir"
    else
        err "Missing directory: $dir"
        VERIFY_OK=false
    fi
done

# Check service
if [ -f "$SYSTEMD_DIR/bandwidth.service" ]; then
    ok "Service: $SYSTEMD_DIR/bandwidth.service"
else
    err "Missing service file"
    VERIFY_OK=false
fi

# Test CLI
if "$INSTALL_DIR/bandwidth" version &>/dev/null; then
    ok "CLI: bandwidth version works"
else
    warn "CLI: bandwidth version failed (daemon may not be running)"
fi

echo ""
if $VERIFY_OK; then
    echo -e "${GREEN}╔════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║   Bandwidth Manager installed successfully!   ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════╝${NC}"
else
    echo -e "${YELLOW}Installation completed with warnings — see above.${NC}"
fi

echo ""
echo "Quick Start:"
echo "  bandwidth status       # Check daemon status"
echo "  bandwidth list         # List managed containers"
echo "  bandwidth help         # Show all commands"
echo "  sudo systemctl status bandwidth  # Service status"
echo "  sudo journalctl -u bandwidth -f  # Follow logs"
echo ""
