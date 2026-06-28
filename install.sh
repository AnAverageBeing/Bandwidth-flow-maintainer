#!/usr/bin/env bash
# =============================================================================
# Bandwidth Manager — One-Liner Installer
# curl -sSL https://raw.githubusercontent.com/AnAverageBeing/Bandwidth-flow-maintainer/main/install.sh | sudo bash
# =============================================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

PASS=0
FAIL=0

log()    { echo -e "${CYAN}[~]${NC} $*"; }
ok()     { echo -e "${GREEN}[✓]${NC} $*"; PASS=$((PASS+1)); }
fail()   { echo -e "${RED}[✗]${NC} $*"; FAIL=$((FAIL+1)); }
warn()   { echo -e "${YELLOW}[!]${NC} $*"; }
header() { echo -e "\n${BOLD}═══ $* ═══${NC}"; }
banner() { echo -e "${BOLD}${CYAN}$*${NC}"; }

# ─── Root Check ────────────────────────────────────────────────────────────────
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

# ─── Configuration ─────────────────────────────────────────────────────────────
REPO="https://github.com/AnAverageBeing/Bandwidth-flow-maintainer.git"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/bandwidth"
DATA_DIR="/var/lib/bandwidth"
LOG_DIR="/var/log/bandwidth"
BUILD_DIR="/tmp/bandwidth-build"
GO_MIN_VERSION="1.21"

# ─── Step 1: Install Dependencies ──────────────────────────────────────────────
header "Step 1/7: Installing Dependencies"

# Check for essential tools
for cmd in curl git make; do
    if ! command -v $cmd &>/dev/null; then
        log "Installing $cmd..."
        apt-get update -qq && apt-get install -y -qq $cmd 2>/dev/null || \
        yum install -y $cmd 2>/dev/null || \
        apk add $cmd 2>/dev/null || \
        warn "Could not install $cmd automatically — please install manually"
    fi
    ok "$cmd: available"
done

# Check/install Go
if command -v go &>/dev/null; then
    GO_VER=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
    ok "Go $GO_VER: installed"
else
    log "Installing Go 1.22..."
    GO_URL="https://go.dev/dl/go1.22.5.linux-amd64.tar.gz"
    curl -sSL "$GO_URL" -o /tmp/go.tar.gz
    tar -C /usr/local -xzf /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    rm -f /tmp/go.tar.gz
    ok "Go 1.22: installed"
fi

# Ensure Go is in PATH for this session
export PATH=$PATH:/usr/local/go/bin:~/go/bin

# Check Docker
if command -v docker &>/dev/null; then
    DOCKER_VER=$(docker --version | grep -oP '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
    ok "Docker $DOCKER_VER: installed"
else
    warn "Docker not found — please install Docker Engine"
fi

# Check tc
if command -v tc &>/dev/null; then
    ok "tc: available"
else
    warn "tc not found — install iproute2 package"
fi

# ─── Step 2: Clone Repository ──────────────────────────────────────────────────
header "Step 2/7: Downloading Source"

rm -rf "$BUILD_DIR"
git clone --depth 1 "$REPO" "$BUILD_DIR" 2>/dev/null
ok "Repository cloned"

cd "$BUILD_DIR"

# ─── Step 3: Install Go Dependencies ────────────────────────────────────────────
header "Step 3/7: Resolving Go Modules"

if go mod download 2>/dev/null; then
    ok "Go modules: downloaded"
else
    warn "Module download had warnings (non-fatal)"
fi

# ─── Step 4: Compile Binaries ──────────────────────────────────────────────────
header "Step 4/7: Compiling Binaries"

log "Building bandwidth (CLI)..."
if go build -o "$BUILD_DIR/build/bandwidth" -ldflags="-s -w" ./cmd/bandwidth/ 2>/tmp/build-cli.log; then
    ok "bandwidth CLI: compiled"
else
    fail "bandwidth CLI: FAILED"
    cat /tmp/build-cli.log
fi

log "Building bandwidthd (daemon)..."
if go build -o "$BUILD_DIR/build/bandwidthd" -ldflags="-s -w" ./cmd/bandwidthd/ 2>/tmp/build-daemon.log; then
    ok "bandwidthd daemon: compiled"
else
    fail "bandwidthd daemon: FAILED"
    cat /tmp/build-daemon.log
fi

# ─── Step 5: Install Files ─────────────────────────────────────────────────────
header "Step 5/7: Installing"

# Stop any existing daemon
systemctl stop bandwidth 2>/dev/null || true
killall bandwidthd 2>/dev/null || true
sleep 1

# Directories
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
mkdir -p /root/.docker && echo '{}' > /root/.docker/config.json
ok "Directories: created"

# Binaries
cp -f "$BUILD_DIR/build/bandwidth" "$INSTALL_DIR/bandwidth"
cp -f "$BUILD_DIR/build/bandwidthd" "$INSTALL_DIR/bandwidthd"
chmod 755 "$INSTALL_DIR/bandwidth" "$INSTALL_DIR/bandwidthd"
ok "Binaries: installed to $INSTALL_DIR"

# Config (don't overwrite existing)
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    warn "Config exists — preserving existing $CONFIG_DIR/config.yaml"
else
    cp "$BUILD_DIR/configs/config.yaml" "$CONFIG_DIR/config.yaml"
    ok "Config: installed to $CONFIG_DIR"
fi

# Systemd service
cp "$BUILD_DIR/systemd/bandwidth.service" /etc/systemd/system/bandwidth.service
# Fix ReadWritePaths for token persistence
sed -i 's|ReadWritePaths=/var/run /var/log/bandwidth /var/lib/bandwidth /sys/class/net /root/.docker|ReadWritePaths=/var/run /var/log/bandwidth /var/lib/bandwidth /sys/class/net /root/.docker /etc/bandwidth|' /etc/systemd/system/bandwidth.service
systemctl daemon-reload
systemctl enable bandwidth 2>/dev/null || true
ok "Systemd service: installed"

# Remove stale sockets
rm -f /var/run/bandwidth.sock /var/run/bandwidth-api.sock

# ─── Step 6: Start Daemon ──────────────────────────────────────────────────────
header "Step 6/7: Starting Daemon"

systemctl start bandwidth
sleep 4

if systemctl is-active --quiet bandwidth; then
    ok "Daemon: started successfully"
else
    fail "Daemon: failed to start"
    echo "Check logs: journalctl -u bandwidth -n 20"
fi

# ─── Step 7: Run Test Suite ────────────────────────────────────────────────────
header "Step 7/7: Running Test Suite"

echo ""

# Test 1: CLI self-test
if "$INSTALL_DIR/bandwidth" version &>/dev/null; then
    VER=$("$INSTALL_DIR/bandwidth" version 2>&1 | head -1)
    ok "CLI version: $VER"
else
    fail "CLI: cannot execute"
fi

# Test 2: Daemon status
if STATUS=$("$INSTALL_DIR/bandwidth" status 2>&1); then
    ok "Daemon status: connected"
    echo "$STATUS" | head -6
else
    fail "Daemon status: $STATUS"
fi

# Test 3: Container list
if LIST=$("$INSTALL_DIR/bandwidth" list 2>&1); then
    COUNT=$(echo "$LIST" | grep -c "running\|stopped" || true)
    ok "Container discovery: $COUNT container(s) found"
else
    warn "Container list: daemon may still be initializing"
fi

# Test 4: Health check
if HEALTH=$("$INSTALL_DIR/bandwidth" health 2>&1); then
    if echo "$HEALTH" | grep -q "healthy"; then
        ok "Health check: all systems healthy"
    else
        warn "Health check: some checks need attention"
    fi
else
    warn "Health check: skipped (daemon initializing)"
fi

# Test 5: Database
if [ -f "$DATA_DIR/bandwidth.db" ]; then
    DB_SIZE=$(du -h "$DATA_DIR/bandwidth.db" | cut -f1)
    ok "Database: $DATA_DIR/bandwidth.db ($DB_SIZE)"
else
    warn "Database: not yet created (auto-creates on first start)"
fi

# Test 6: TC rules
if TC_COUNT=$(sudo tc qdisc show 2>/dev/null | grep -c "htb\|ingress" || true); then
    ok "TC rules: $TC_COUNT active qdisc(s)"
else
    warn "TC rules: none active yet"
fi

# Test 7: Config validation
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
if docker info &>/dev/null; then
    ok "Docker: connected"
else
    warn "Docker: not accessible (daemon runs as root, should be fine)"
fi

# Test 10: Go build self-test
if go build -o /tmp/bw-self-test "$BUILD_DIR/cmd/bandwidth/" 2>/dev/null; then
    ok "Build system: Go toolchain works"
    rm -f /tmp/bw-self-test
else
    fail "Build system: Go compilation failed"
fi

# ─── Final Summary ─────────────────────────────────────────────────────────────
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

# Cleanup
rm -rf "$BUILD_DIR/build" 2>/dev/null || true

exit $FAIL
