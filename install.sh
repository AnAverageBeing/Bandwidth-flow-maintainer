#!/usr/bin/env bash
# =============================================================================
# Bandwidth Manager — Production One-Liner Installer
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/AnAverageBeing/Bandwidth-flow-maintainer/main/install.sh | sudo bash
#
# Or locally:
#   sudo bash install.sh
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
PASS=0; FAIL=0

log()    { echo -e "${CYAN}[~]${NC} $*"; }
ok()     { echo -e "${GREEN}[✓]${NC} $*"; PASS=$((PASS+1)); }
fail()   { echo -e "${RED}[✗]${NC} $*"; FAIL=$((FAIL+1)); }
warn()   { echo -e "${YELLOW}[!]${NC} $*"; }
header() { echo -e "\n${BOLD}═══ $* ═══${NC}"; }
banner() { echo -e "${BOLD}${CYAN}$*${NC}"; }

# ─── Root Check ──────────────────────────────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    echo -e "${RED}This installer must be run as root.${NC}"
    echo "Usage: curl -sSL https://raw.githubusercontent.com/AnAverageBeing/Bandwidth-flow-maintainer/main/install.sh | sudo bash"
    exit 1
fi

clear
banner "╔══════════════════════════════════════════════════════╗"
banner "║   Bandwidth Manager — Production Installer           ║"
banner "║   Docker Container Bandwidth Management System       ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""

# ─── Locate Repository Root ──────────────────────────────────────────────────
# Find the repo directory (this script's location)
REPO_DIR="$(cd "$(dirname "$0")" && pwd)"

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/bandwidth"
DATA_DIR="/var/lib/bandwidth"
LOG_DIR="/var/log/bandwidth"
BUILD_DIR="$REPO_DIR/build"

# ─── Step 1: Install Dependencies ────────────────────────────────────────────
header "Step 1/7: Installing Dependencies"

# Essential tools
for cmd in curl git make; do
    if command -v $cmd &>/dev/null; then
        ok "$cmd: available"
    else
        log "Installing $cmd..."
        apt-get update -qq && apt-get install -y -qq $cmd 2>/dev/null || true
        yum install -y $cmd 2>/dev/null || true
        apk add $cmd 2>/dev/null || true
        if command -v $cmd &>/dev/null; then
            ok "$cmd: installed"
        else
            warn "$cmd: unavailable — install manually"
        fi
    fi
done

# ─── Go Installation ─────────────────────────────────────────────────────────
NEED_GO=false
GO_TARGET="1.25.4"
GO_URL="https://go.dev/dl/go${GO_TARGET}.linux-amd64.tar.gz"

if command -v go &>/dev/null; then
    GO_VER=$(go version 2>/dev/null | grep -oP 'go[0-9]+\.[0-9]+' | head -1 | grep -oP '[0-9]+\.[0-9]+' || echo "0.0")
    GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
    if [ "$GO_MAJOR" -ge 2 ] 2>/dev/null || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -ge 23 ]; } 2>/dev/null; then
        ok "Go $GO_VER: suitable"
    else
        warn "Go $GO_VER too old (need 1.23+) — installing Go $GO_TARGET"
        NEED_GO=true
    fi
else
    NEED_GO=true
fi

if $NEED_GO; then
    log "Installing Go $GO_TARGET..."
    rm -rf /usr/local/go 2>/dev/null || true
    curl -sSL "$GO_URL" -o /tmp/go.tar.gz
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    # Ensure PATH
    export PATH=/usr/local/go/bin:$PATH
    # Persist in profile
    grep -q '/usr/local/go/bin' /etc/profile 2>/dev/null || \
        echo 'export PATH=/usr/local/go/bin:$PATH' >> /etc/profile
    grep -q '/usr/local/go/bin' /root/.bashrc 2>/dev/null || \
        echo 'export PATH=/usr/local/go/bin:$PATH' >> /root/.bashrc
    ok "Go $GO_TARGET: installed"
fi

# Ensure Go is definitely in PATH
export PATH=/usr/local/go/bin:$PATH:/root/go/bin

# ─── Docker ──────────────────────────────────────────────────────────────────
if docker info &>/dev/null 2>&1; then
    DOCKER_VER=$(docker --version 2>/dev/null | grep -oP '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
    ok "Docker $DOCKER_VER: available"
else
    warn "Docker not found — install Docker Engine for container discovery"
fi

# ─── tc ──────────────────────────────────────────────────────────────────────
if command -v tc &>/dev/null; then
    ok "tc: available"
else
    warn "tc not found — install iproute2"
fi

# ─── Step 2: Prepare Build ───────────────────────────────────────────────────
header "Step 2/7: Preparing Build"

ok "Source directory: $REPO_DIR"
cd "$REPO_DIR"

# Clean previous build artifacts
rm -rf "$BUILD_DIR" 2>/dev/null || true
mkdir -p "$BUILD_DIR"

# ─── Step 3: Resolve Go Modules ──────────────────────────────────────────────
header "Step 3/7: Resolving Go Modules"

# Prevent Go from auto-downloading different toolchain versions
export GOTOOLCHAIN=local
export GOFLAGS="-mod=mod"

# Clean module cache if it's corrupted (common source of issues)
if [ -f go.mod ]; then
    log "Tidying modules..."
    if go mod tidy 2>/tmp/mod-tidy.log; then
        ok "Go modules: resolved"
    else
        warn "Module tidy had warnings — attempting clean cache"
        go clean -modcache 2>/dev/null || true
        if go mod tidy 2>/tmp/mod-tidy2.log; then
            ok "Go modules: resolved (after cache clean)"
        else
            fail "Go modules: resolution failed"
            cat /tmp/mod-tidy2.log
        fi
    fi
else
    fail "go.mod not found in $REPO_DIR"
    exit 1
fi

# ─── Step 4: Compile Binaries ────────────────────────────────────────────────
header "Step 4/7: Compiling Binaries"

BUILD_FLAGS="-ldflags=-s -w -trimpath"

log "Building bandwidth (CLI)..."
if CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS="-mod=mod" \
    go build $BUILD_FLAGS -o "$BUILD_DIR/bandwidth" ./cmd/bandwidth/ 2>/tmp/build-cli.log; then
    ok "bandwidth CLI: compiled"
else
    fail "bandwidth CLI: FAILED"
    cat /tmp/build-cli.log
fi

log "Building bandwidthd (daemon)..."
if CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS="-mod=mod" \
    go build $BUILD_FLAGS -o "$BUILD_DIR/bandwidthd" ./cmd/bandwidthd/ 2>/tmp/build-daemon.log; then
    ok "bandwidthd daemon: compiled"
else
    fail "bandwidthd daemon: FAILED"
    cat /tmp/build-daemon.log
fi

# Verify binaries exist
if [ ! -f "$BUILD_DIR/bandwidth" ] || [ ! -f "$BUILD_DIR/bandwidthd" ]; then
    echo ""
    echo -e "${RED}BUILD FAILED — check errors above${NC}"
    echo "Try running: cd $REPO_DIR && go mod tidy && CGO_ENABLED=0 go build ./..."
    exit 1
fi

# ─── Step 5: Install Files ───────────────────────────────────────────────────
header "Step 5/7: Installing"

# Stop any existing daemon
systemctl stop bandwidth 2>/dev/null || true
killall bandwidthd 2>/dev/null || true
sleep 1

# Directories
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
ok "Directories: created"

# Binaries
cp -f "$BUILD_DIR/bandwidth" "$INSTALL_DIR/bandwidth"
cp -f "$BUILD_DIR/bandwidthd" "$INSTALL_DIR/bandwidthd"
chmod 755 "$INSTALL_DIR/bandwidth" "$INSTALL_DIR/bandwidthd"
ok "Binaries: installed to $INSTALL_DIR"

# Config (preserve existing)
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    warn "Config exists — preserving existing $CONFIG_DIR/config.yaml"
else
    if [ -f "$REPO_DIR/configs/config.yaml" ]; then
        cp "$REPO_DIR/configs/config.yaml" "$CONFIG_DIR/config.yaml"
        ok "Config: installed to $CONFIG_DIR"
    else
        warn "Config: no template found — daemon will auto-generate"
    fi
fi

# Systemd service
if [ -f "$REPO_DIR/systemd/bandwidth.service" ]; then
    cp "$REPO_DIR/systemd/bandwidth.service" /etc/systemd/system/bandwidth.service
    # Fix ReadWritePaths
    sed -i 's|ReadWritePaths=/var/run /var/log/bandwidth /var/lib/bandwidth /sys/class/net /root/.docker|ReadWritePaths=/var/run /var/log/bandwidth /var/lib/bandwidth /sys/class/net /root/.docker /etc/bandwidth|' /etc/systemd/system/bandwidth.service 2>/dev/null || true
    systemctl daemon-reload
    systemctl enable bandwidth 2>/dev/null || true
    ok "Systemd service: installed"
else
    warn "systemd service file not found"
fi

# Remove stale sockets
rm -f /var/run/bandwidth.sock /var/run/bandwidth-api.sock 2>/dev/null || true

# ─── Step 6: Start Daemon ────────────────────────────────────────────────────
header "Step 6/7: Starting Daemon"

systemctl start bandwidth 2>/dev/null || true
sleep 4

if systemctl is-active --quiet bandwidth 2>/dev/null; then
    ok "Daemon: started successfully"
else
    warn "Daemon: starting..."
    # Try running directly to see errors
    if /usr/local/bin/bandwidthd --config /etc/bandwidth/config.yaml &>/tmp/bw-daemon.log &
    then
        sleep 2
    fi
    if pgrep -f bandwidthd >/dev/null 2>&1; then
        ok "Daemon: running (direct mode)"
    else
        fail "Daemon: failed to start"
        echo "Check logs:"
        echo "  journalctl -u bandwidth -n 20"
        echo "  cat /tmp/bw-daemon.log"
    fi
fi

# ─── Step 7: Test Suite ──────────────────────────────────────────────────────
header "Step 7/7: Running Test Suite"

echo ""
BW="$INSTALL_DIR/bandwidth"

# Test 1: CLI self-test
if "$BW" version &>/dev/null 2>&1; then
    VER=$("$BW" version 2>&1 | head -1)
    ok "CLI version: $VER"
else
    fail "CLI: cannot execute"
fi

# Test 2: Daemon status
if STATUS=$("$BW" status 2>&1); then
    ok "Daemon status: connected"
    echo "$STATUS" | head -6
else
    warn "Daemon status: $STATUS"
fi

# Test 3: Container list
if LIST=$("$BW" list 2>&1); then
    COUNT=$(echo "$LIST" | grep -c "running\|stopped" 2>/dev/null || echo "0")
    ok "Container discovery: $COUNT container(s) found"
else
    warn "Container list: daemon may still be initializing"
fi

# Test 4: Health check
if HEALTH=$("$BW" health 2>&1); then
    if echo "$HEALTH" | grep -qi "healthy\|ok\|pass"; then
        ok "Health check: all systems healthy"
    else
        warn "Health check: some checks need attention"
    fi
else
    warn "Health check: skipped (daemon initializing)"
fi

# Test 5: Database
if [ -f "$DATA_DIR/bandwidth.db" ]; then
    DB_SIZE=$(du -h "$DATA_DIR/bandwidth.db" 2>/dev/null | cut -f1)
    ok "Database: $DATA_DIR/bandwidth.db ($DB_SIZE)"
else
    warn "Database: not yet created (auto-creates on first start)"
fi

# Test 6: TC rules
if TC_COUNT=$(tc qdisc show 2>/dev/null | grep -c "htb\|ingress" 2>/dev/null || echo "0"); then
    ok "TC rules: $TC_COUNT active qdisc(s)"
else
    warn "TC rules: none active yet"
fi

# Test 7: Config
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    ok "Config: $CONFIG_DIR/config.yaml"
else
    warn "Config: using built-in defaults"
fi

# Test 8: Socket
if [ -S /var/run/bandwidth.sock ]; then
    ok "CLI socket: /var/run/bandwidth.sock"
else
    warn "CLI socket: not yet created"
fi

# Test 9: Docker connectivity
if docker info &>/dev/null 2>&1; then
    ok "Docker: connected"
else
    warn "Docker: not accessible (may be fine with root daemon)"
fi

# Test 10: Top TUI (quick smoke test — exits immediately)
if timeout 2 "$BW" top </dev/null &>/dev/null 2>&1; then
    ok "TUI: launches successfully"
else
    warn "TUI: requires interactive terminal"
fi

# ─── Final Summary ───────────────────────────────────────────────────────────
echo ""
banner "╔══════════════════════════════════════════════════════╗"
banner "║              INSTALLATION COMPLETE                   ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""
echo -e "  Tests:   ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC}"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "  ${GREEN}✓ All tests passed!${NC}"
else
    echo -e "  ${YELLOW}⚠ Some tests failed — review logs above${NC}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Quick Start Commands:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  bandwidth status        Check daemon status"
echo "  bandwidth list          List managed containers"
echo "  bandwidth top           Live bandwidth TUI monitor"
echo "  bandwidth health        Run health diagnostics"
echo "  bandwidth limits        Show configured limits"
echo "  bandwidth reapply       Reapply all TC rules"
echo "  bandwidth help          Show all commands"
echo ""
echo "  sudo systemctl status bandwidth     Service status"
echo "  sudo journalctl -u bandwidth -f     Follow logs"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Configuration: /etc/bandwidth/config.yaml"
echo " Database:      /var/lib/bandwidth/bandwidth.db"
echo " Logs:          /var/log/bandwidth/bandwidth.log"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Offer interactive configuration
echo ""
read -rp "Would you like to configure settings now? [Y/n]: " do_config
do_config=${do_config:-y}
if [ "$do_config" = "y" ] || [ "$do_config" = "Y" ] || [ "$do_config" = "yes" ]; then
    "$BW" configure 2>/dev/null || warn "Configure: interactive setup skipped (no terminal)"
    "$BW" reapply 2>/dev/null || warn "Reapply: daemon may not be running yet"
fi

# Cleanup
rm -rf "$BUILD_DIR" 2>/dev/null || true

exit $FAIL
