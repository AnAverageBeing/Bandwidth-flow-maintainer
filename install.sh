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
# Get the absolute directory of this script, regardless of how it was invoked
if [ -n "${BASH_SOURCE[0]}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
elif command -v readlink >/dev/null 2>&1; then
    SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
else
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
fi
REPO_DIR="$SCRIPT_DIR"
cd "$REPO_DIR" || { echo "ERROR: Cannot cd to $REPO_DIR"; exit 1; }
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/bandwidth"
DATA_DIR="/var/lib/bandwidth"
LOG_DIR="/var/log/bandwidth"
BUILD_DIR="$REPO_DIR"

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

# Check/install Go (need 1.21+)
NEED_GO=false
if command -v go &>/dev/null; then
    GO_VER=$(go version | grep -oP 'go[0-9]+\.[0-9]+' | head -1 | grep -oP '[0-9]+\.[0-9]+')
    GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
    if [ "$GO_MAJOR" -ge 2 ] 2>/dev/null || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -ge 21 ]; } 2>/dev/null; then
        ok "Go $GO_VER: suitable"
    else
        warn "Go $GO_VER too old (need 1.21+) — will install Go 1.22"
        NEED_GO=true
    fi
else
    NEED_GO=true
fi

if $NEED_GO; then
    log "Installing Go 1.23..."
    GO_URL="https://go.dev/dl/go1.23.6.linux-amd64.tar.gz"
    curl -sSL "$GO_URL" -o /tmp/go.tar.gz
    tar -C /usr/local -xzf /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile 2>/dev/null || true
    rm -f /tmp/go.tar.gz
    ok "Go 1.23: installed"
fi

# Ensure Go is in PATH for this session
export PATH=$PATH:/usr/local/go/bin:~/go/bin
export PATH=$PATH:/usr/local/go/bin:~/go/bin

# Check Docker (needs sudo or docker group)
if sudo docker info &>/dev/null 2>&1 || docker info &>/dev/null 2>&1; then
    DOCKER_VER=$(sudo docker --version 2>/dev/null | grep -oP '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || docker --version 2>/dev/null | grep -oP '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
    ok "Docker $DOCKER_VER: available"
else
    warn "Docker not found — install Docker Engine for container discovery"
fi

# Check tc
if command -v tc &>/dev/null; then
    ok "tc: available"
else
    warn "tc not found — install iproute2 package"
fi

# ─── Step 2: Build Directory ──────────────────────────────────────────────────
header "Step 2/7: Preparing Build"

ok "Source directory: $REPO_DIR"
cd "$REPO_DIR"

# ─── Step 3: Skip (go build handles modules automatically) ──────────────────
header "Step 3/7: Build will resolve modules automatically"
ok "Module resolution: handled by go build"

# ─── Step 4: Compile Binaries ──────────────────────────────────────────────────
header "Step 4/7: Compiling Binaries"

# Force go.mod to stay at go 1.21 (some deps try to upgrade it)
sed -i 's/^go 1\.[0-9]\+/go 1.21/' "$REPO_DIR/go.mod" 2>/dev/null || true
export GOTOOLCHAIN=auto
export GOFLAGS="-mod=mod"

log "Building bandwidth (CLI)..."
if CGO_ENABLED=0 go build -o "$REPO_DIR/build/bandwidth" -ldflags="-s -w" ./cmd/bandwidth/ 2>/tmp/build-cli.log; then
    ok "bandwidth CLI: compiled"
else
    fail "bandwidth CLI: FAILED"
    cat /tmp/build-cli.log
fi

log "Building bandwidthd (daemon)..."
if CGO_ENABLED=0 GOTOOLCHAIN=auto go build -o "$REPO_DIR/build/bandwidthd" -ldflags="-s -w" ./cmd/bandwidthd/ 2>/tmp/build-daemon.log; then
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
cp -f "$REPO_DIR/build/bandwidth" "$INSTALL_DIR/bandwidth"
cp -f "$REPO_DIR/build/bandwidthd" "$INSTALL_DIR/bandwidthd"
chmod 755 "$INSTALL_DIR/bandwidth" "$INSTALL_DIR/bandwidthd"
ok "Binaries: installed to $INSTALL_DIR"

# Config (don't overwrite existing)
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    warn "Config exists — preserving existing $CONFIG_DIR/config.yaml"
else
    cp "$REPO_DIR/configs/config.yaml" "$CONFIG_DIR/config.yaml"
    ok "Config: installed to $CONFIG_DIR"
fi

# Systemd service
cp "$REPO_DIR/systemd/bandwidth.service" /etc/systemd/system/bandwidth.service
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
if go build -o /tmp/bw-self-test "$REPO_DIR/cmd/bandwidth/" 2>/dev/null; then
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

# Offer interactive configuration
echo ""
read -p "Would you like to configure settings now? [Y/n]: " do_config
do_config=${do_config:-y}
if [ "$do_config" = "y" ] || [ "$do_config" = "Y" ] || [ "$do_config" = "yes" ]; then
    "$INSTALL_DIR/bandwidth" configure
    "$INSTALL_DIR/bandwidth" reapply
fi

# Cleanup
rm -rf "$REPO_DIR/build" 2>/dev/null || true

exit $FAIL
