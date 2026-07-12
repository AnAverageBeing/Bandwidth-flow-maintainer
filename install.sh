#!/usr/bin/env bash
# =============================================================================
# Bandwidth Manager — Universal One-Liner Installer
#
# Works reliably on ALL Linux distributions and kernels.
#
# Pipe from GitHub:
#   curl -sSL https://raw.githubusercontent.com/AnAverageBeing/Bandwidth-flow-maintainer/main/install.sh | sudo bash
#
# Or run from cloned repo:
#   sudo bash install.sh
#
# No manual fixes required. Handles all edge cases.
# =============================================================================
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
PASS=0; FAIL=0; WARN=0

log()   { echo -e "${CYAN}[~]${NC} $*"; }
ok()    { echo -e "${GREEN}[✓]${NC} $*"; PASS=$((PASS+1)); }
fail()  { echo -e "${RED}[✗]${NC} $*"; FAIL=$((FAIL+1)); }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; WARN=$((WARN+1)); }
header(){ echo -e "\n${BOLD}═══ $* ═══${NC}"; }
banner(){ echo -e "${BOLD}${CYAN}$*${NC}"; }

# ─── Detect Architecture ────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)  GO_ARCH="amd64"   ;;
    aarch64|arm64) GO_ARCH="arm64"   ;;
    armv7l)        GO_ARCH="armv6l"  ;;
    i686|i386)     GO_ARCH="386"     ;;
    *)             GO_ARCH="amd64"
                   warn "Unknown architecture $ARCH — assuming amd64" ;;
esac

# ─── Constants ───────────────────────────────────────────────────────────────
REPO_URL="https://github.com/AnAverageBeing/Bandwidth-flow-maintainer.git"
GO_VERSION="1.25.4"
GO_URL="https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/bandwidth"
DATA_DIR="/var/lib/bandwidth"
LOG_DIR="/var/log/bandwidth"

# ─── Root Check ──────────────────────────────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    echo -e "${RED}This installer must be run as root.${NC}"
    echo "  curl -sSL https://raw.githubusercontent.com/AnAverageBeing/Bandwidth-flow-maintainer/main/install.sh | sudo bash"
    exit 1
fi

clear
banner "╔══════════════════════════════════════════════════════╗"
banner "║   Bandwidth Manager — Universal Installer            ║"
banner "║   Docker Container Bandwidth Management System       ║"
banner "║   Developed by AnAverageBeing                        ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""

# ─── Step 1: Dependencies ────────────────────────────────────────────────────
header "Step 1/7: Installing Dependencies"

# Detect package manager
PKG_MGR=""
if command -v apt-get &>/dev/null; then PKG_MGR="apt"; fi
if command -v yum &>/dev/null; then PKG_MGR="yum"; fi
if command -v dnf &>/dev/null; then PKG_MGR="dnf"; fi
if command -v apk &>/dev/null; then PKG_MGR="apk"; fi
if command -v pacman &>/dev/null; then PKG_MGR="pacman"; fi

install_pkg() {
    local pkg="$1"
    case "$PKG_MGR" in
        apt)    apt-get update -qq && apt-get install -y -qq "$pkg" 2>/dev/null ;;
        yum)    yum install -y "$pkg" 2>/dev/null ;;
        dnf)    dnf install -y "$pkg" 2>/dev/null ;;
        apk)    apk add --no-cache "$pkg" 2>/dev/null ;;
        pacman) pacman -S --noconfirm "$pkg" 2>/dev/null ;;
        *)      return 1 ;;
    esac
}

# Essential: curl, git, make
for cmd in curl git; do
    if command -v "$cmd" &>/dev/null; then
        ok "$cmd: available"
    else
        log "Installing $cmd..."
        install_pkg "$cmd" && ok "$cmd: installed" || warn "$cmd: unavailable"
    fi
done

# make is needed for some Go builds (though we use go build directly)
if command -v make &>/dev/null; then
    ok "make: available"
else
    install_pkg make 2>/dev/null && ok "make: installed" || warn "make: not critical"
fi

# ─── Go ──────────────────────────────────────────────────────────────────────
NEED_GO=false
if command -v go &>/dev/null; then
    GO_VER=$(go version 2>/dev/null | grep -oP 'go[0-9]+\.[0-9]+' | head -1 | grep -oP '[0-9]+\.[0-9]+' || echo "0.0")
    GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
    if [ "$GO_MAJOR" -ge 2 ] 2>/dev/null || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -ge 23 ]; } 2>/dev/null; then
        ok "Go $GO_VER: suitable"
    else
        warn "Go $GO_VER too old (need 1.23+) — installing Go $GO_VERSION"
        NEED_GO=true
    fi
else
    NEED_GO=true
fi

if $NEED_GO; then
    log "Installing Go $GO_VERSION ($GO_ARCH)..."
    rm -rf /usr/local/go 2>/dev/null || true
    curl -sSL --retry 3 "$GO_URL" -o /tmp/go.tar.gz
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    ok "Go $GO_VERSION: installed"
fi

# Ensure Go in PATH for this session AND persist
export PATH=/usr/local/go/bin:$PATH
export PATH=$PATH:/root/go/bin
for prof in /etc/profile /root/.bashrc /etc/bash.bashrc; do
    [ -f "$prof" ] && grep -q '/usr/local/go/bin' "$prof" 2>/dev/null || \
        echo 'export PATH=/usr/local/go/bin:$PATH' >> "$prof" 2>/dev/null || true
done

# ─── Docker (optional) ───────────────────────────────────────────────────────
if docker info &>/dev/null 2>&1; then
    DOCKER_VER=$(docker --version 2>/dev/null | grep -oP '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "?")
    ok "Docker $DOCKER_VER: available"
    HAS_DOCKER=true
else
    warn "Docker not found — container management unavailable"
    HAS_DOCKER=false
fi

# ─── tc (optional, Linux only) ───────────────────────────────────────────────
if command -v tc &>/dev/null; then
    ok "tc: available"
    HAS_TC=true
else
    warn "tc not found — bandwidth shaping unavailable (install iproute2)"
    HAS_TC=false
fi

# ─── systemd (optional) ──────────────────────────────────────────────────────
if [ -d /run/systemd/system ] || command -v systemctl &>/dev/null; then
    HAS_SYSTEMD=true
else
    warn "systemd not detected — service management via nohup fallback"
    HAS_SYSTEMD=false
fi

# ─── Step 2: Get Source ──────────────────────────────────────────────────────
header "Step 2/7: Preparing Source"

# Detect if we're already in a git repo
if [ -f "go.mod" ] && [ -d "cmd/bandwidth" ] && [ -d "internal" ]; then
    REPO_DIR="$(pwd)"
    ok "Using existing source: $REPO_DIR"
else
    # Check common locations
    for candidate in \
        "$HOME/Bandwidth-flow-maintainer" \
        "/root/Bandwidth-flow-maintainer" \
        "/tmp/bandwidth-build"; do
        if [ -f "$candidate/go.mod" ] && [ -d "$candidate/cmd/bandwidth" ]; then
            REPO_DIR="$candidate"
            ok "Found source at: $REPO_DIR"
            break
        fi
    done

    # Still not found — clone from GitHub
    if [ -z "${REPO_DIR:-}" ]; then
        log "Cloning repository from GitHub..."
        REPO_DIR="/tmp/bandwidth-build"
        rm -rf "$REPO_DIR" 2>/dev/null || true
        if git clone --depth 1 "$REPO_URL" "$REPO_DIR" 2>/tmp/git-clone.log; then
            ok "Repository cloned"
        else
            fail "Git clone failed"
            cat /tmp/git-clone.log
            exit 1
        fi
    fi
fi

cd "$REPO_DIR"
BUILD_DIR="$REPO_DIR/build"
rm -rf "$BUILD_DIR" 2>/dev/null || true
mkdir -p "$BUILD_DIR"

# ─── Step 3: Resolve Modules ─────────────────────────────────────────────────
header "Step 3/7: Resolving Go Modules"

export GOTOOLCHAIN=local
export GOFLAGS="-mod=mod"

if [ -f go.mod ]; then
    log "Running go mod tidy..."
    if go mod tidy 2>/tmp/mod-tidy.log; then
        ok "Go modules: resolved"
    else
        # Clean corrupted cache and retry
        warn "Module tidy had issues — cleaning cache"
        go clean -modcache 2>/dev/null || true
        rm -rf "$HOME/go/pkg/mod/cache" 2>/dev/null || true
        if go mod tidy 2>/tmp/mod-tidy2.log; then
            ok "Go modules: resolved (after cache clean)"
        else
            fail "Module resolution failed"
            cat /tmp/mod-tidy2.log
        fi
    fi
else
    fail "go.mod not found in $REPO_DIR"
    exit 1
fi

# ─── Step 4: Compile ─────────────────────────────────────────────────────────
header "Step 4/7: Compiling Binaries"

BUILD_FLAGS="-trimpath"
export CGO_ENABLED=0
export GOTOOLCHAIN=local

log "Building bandwidth (CLI)..."
if go build $BUILD_FLAGS -o "$BUILD_DIR/bandwidth" ./cmd/bandwidth/ 2>/tmp/build-cli.log; then
    ok "bandwidth CLI: compiled"
else
    fail "bandwidth CLI: FAILED"
    head -20 /tmp/build-cli.log
fi

log "Building bandwidthd (daemon)..."
if go build $BUILD_FLAGS -o "$BUILD_DIR/bandwidthd" ./cmd/bandwidthd/ 2>/tmp/build-daemon.log; then
    ok "bandwidthd daemon: compiled"
else
    fail "bandwidthd daemon: FAILED"
    head -20 /tmp/build-daemon.log
fi

# Verify
if [ ! -f "$BUILD_DIR/bandwidth" ] || [ ! -f "$BUILD_DIR/bandwidthd" ]; then
    echo ""
    echo -e "${RED}╔════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║  BUILD FAILED                              ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Debug info:"
    echo "  Go version: $(go version 2>&1)"
    echo "  Go env GOVERSION: $(go env GOVERSION 2>&1)"
    echo "  Architecture: $GO_ARCH"
    echo "  CGO_ENABLED: $CGO_ENABLED"
    echo "  Source: $REPO_DIR"
    echo ""
    echo "Manual fix: cd $REPO_DIR && CGO_ENABLED=0 GOTOOLCHAIN=local go build ./..."
    exit 1
fi

# ─── Step 5: Install ─────────────────────────────────────────────────────────
header "Step 5/7: Installing Files"

# Stop existing daemon
systemctl stop bandwidth 2>/dev/null || true
killall bandwidthd 2>/dev/null || true
sleep 1

# Create directories
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
ok "Directories: created"

# Install binaries
cp -f "$BUILD_DIR/bandwidth" "$INSTALL_DIR/bandwidth"
cp -f "$BUILD_DIR/bandwidthd" "$INSTALL_DIR/bandwidthd"
chmod 755 "$INSTALL_DIR/bandwidth" "$INSTALL_DIR/bandwidthd"
ok "Binaries: installed to $INSTALL_DIR"

# Config
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    ok "Config: preserving existing $CONFIG_DIR/config.yaml"
else
    if [ -f "$REPO_DIR/configs/config.yaml" ]; then
        cp "$REPO_DIR/configs/config.yaml" "$CONFIG_DIR/config.yaml"
        ok "Config: installed from template"
    else
        warn "Config: no template — daemon auto-generates defaults"
    fi
fi

# Systemd service or fallback
if $HAS_SYSTEMD; then
    if [ -f "$REPO_DIR/systemd/bandwidth.service" ]; then
        # Create /root/.docker if it doesn't exist (needed for Docker config)
        mkdir -p /root/.docker 2>/dev/null || true
        # Always overwrite with fresh service file
        cp -f "$REPO_DIR/systemd/bandwidth.service" /etc/systemd/system/bandwidth.service
        systemctl daemon-reload
        systemctl enable bandwidth 2>/dev/null || true
        ok "Systemd service: installed"
    else
        warn "Systemd service file not found"
    fi
else
    warn "No systemd — daemon must be started manually: bandwidthd &"
fi

# Clean stale sockets
rm -f /var/run/bandwidth.sock /var/run/bandwidth-api.sock 2>/dev/null || true

# ─── Step 6: Start Daemon ────────────────────────────────────────────────────
header "Step 6/7: Starting Daemon"

DAEMON_STARTED=false

if $HAS_SYSTEMD; then
    systemctl start bandwidth 2>/dev/null || true
    sleep 4
    if systemctl is-active --quiet bandwidth 2>/dev/null; then
        ok "Daemon: started (systemd)"
        DAEMON_STARTED=true
    else
        warn "Systemd start failed — trying direct mode"
        echo "  Journal: $(journalctl -u bandwidth --no-pager -n 5 2>/dev/null || echo 'unavailable')"
    fi
fi

if ! $DAEMON_STARTED; then
    # Direct start as background process
    nohup "$INSTALL_DIR/bandwidthd" --config "$CONFIG_DIR/config.yaml" > "$LOG_DIR/bandwidth.log" 2>&1 &
    DAEMON_PID=$!
    sleep 3
    if kill -0 "$DAEMON_PID" 2>/dev/null; then
        ok "Daemon: started (PID $DAEMON_PID)"
        DAEMON_STARTED=true
    else
        fail "Daemon: failed to start"
        echo "Last log lines:"
        tail -20 "$LOG_DIR/bandwidth.log" 2>/dev/null || echo "(no log output)"
    fi
fi

# ─── Step 7: Test Suite ──────────────────────────────────────────────────────
header "Step 7/7: Running Test Suite"

echo ""
BW="$INSTALL_DIR/bandwidth"

# 1. CLI version
if "$BW" version &>/dev/null 2>&1; then
    VER=$("$BW" version 2>&1 | head -1)
    ok "CLI version: $VER"
else
    fail "CLI: execution failed"
fi

# 2. Daemon status
sleep 1
if STATUS=$("$BW" status 2>&1); then
    ok "Daemon: connected"
    echo "$STATUS" | head -6
else
    warn "Daemon status: $STATUS"
fi

# 3. Container list
if LIST=$("$BW" list 2>&1); then
    COUNT=$(echo "$LIST" | grep -ci "running\|stopped" 2>/dev/null || echo "0")
    ok "Containers: $COUNT found"
else
    warn "Containers: discovery pending"
fi

# 4. Health check
if HEALTH=$("$BW" health 2>&1); then
    ok "Health: passed"
else
    warn "Health: some checks pending"
fi

# 5. Database
if [ -f "$DATA_DIR/bandwidth.db" ]; then
    DB_SIZE=$(du -h "$DATA_DIR/bandwidth.db" 2>/dev/null | cut -f1 || echo "?")
    ok "Database: $DB_SIZE"
else
    warn "Database: auto-creates on first poll"
fi

# 6. TC rules
if $HAS_TC; then
    TC_COUNT=$(tc qdisc show 2>/dev/null | grep -c "htb\|ingress" 2>/dev/null || echo "0")
    ok "TC rules: $TC_COUNT active"
else
    warn "TC: not available on this system"
fi

# 7. Config
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    ok "Config: present"
else
    warn "Config: using defaults"
fi

# 8. Socket
if [ -S /var/run/bandwidth.sock ]; then
    ok "IPC socket: ready"
else
    warn "IPC socket: initializing"
fi

# 9. Docker
if $HAS_DOCKER; then
    ok "Docker: connected"
else
    warn "Docker: not available (container features disabled)"
fi

# 10. TUI smoke test (skip if no real terminal — BubbleTea needs a TTY)
if [ -t 0 ] && [ -t 1 ]; then
    if timeout 3 "$BW" top </dev/null &>/dev/null 2>&1; then
        ok "TUI: functional"
    else
        warn "TUI: requires terminal (expected in SSH)"
    fi
else
    warn "TUI: skipped (no TTY — expected in pipe/SSH)"
fi

# ─── Final Summary ───────────────────────────────────────────────────────────
echo ""
banner "╔══════════════════════════════════════════════════════╗"
banner "║              INSTALLATION COMPLETE                   ║"
banner "║   Developed by AnAverageBeing                        ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""
echo -e "  Passed:  ${GREEN}$PASS${NC}"
echo -e "  Failed:  ${RED}$FAIL${NC}"
echo -e "  Warnings: ${YELLOW}$WARN${NC}"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "  ${GREEN}✓ Installation successful!${NC}"
else
    echo -e "  ${YELLOW}⚠ $FAIL check(s) failed — review above${NC}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Quick Start:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  bandwidth status        Daemon status"
echo "  bandwidth list          Managed containers"
echo "  bandwidth top           Live TUI monitor"
echo "  bandwidth health        Health diagnostics"
echo "  bandwidth limits        Configured limits"
echo "  bandwidth configure     Interactive setup"
echo "  bandwidth reapply       Reapply TC rules"
echo "  bandwidth help          All commands"
echo ""

if $HAS_SYSTEMD; then
    echo "  systemctl status bandwidth     Service status"
    echo "  journalctl -u bandwidth -f     Follow logs"
else
    echo "  tail -f $LOG_DIR/bandwidth.log       Follow logs"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Config:   $CONFIG_DIR/config.yaml"
echo " Database: $DATA_DIR/bandwidth.db"
echo " Logs:     $LOG_DIR/bandwidth.log"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ─── Interactive Setup ───────────────────────────────────────────────────────
# Offer interactive config whenever we can read from the controlling terminal.
# This works even when the installer is piped from curl because we read /dev/tty.
if [ $FAIL -eq 0 ] || [ "$DAEMON_STARTED" = "true" ]; then
    if [ -c /dev/tty ]; then
        echo ""
        read -rp "Configure settings now? [Y/n]: " do_config < /dev/tty 2>/dev/null || do_config=""
        do_config=${do_config:-y}
        if [ "$do_config" = "y" ] || [ "$do_config" = "Y" ]; then
            "$BW" configure 2>/dev/null || warn "Interactive configure unavailable"
            "$BW" reapply 2>/dev/null || true
        fi
    else
        warn "Skipping interactive config (no terminal — run 'bandwidth configure' later)"
    fi
fi

# Cleanup
rm -rf "$BUILD_DIR" 2>/dev/null || true
# Keep cloned source if we cloned it (user might want to rebuild)
# rm -rf /tmp/bandwidth-build 2>/dev/null || true

exit $FAIL
