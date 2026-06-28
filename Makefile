.PHONY: all build clean install test lint run-cli run-daemon tui

APP_NAME := bandwidth
DAEMON_NAME := bandwidthd
BUILD_DIR := build
GO := go
GOFLAGS := -ldflags="-s -w"
PKG := ./...

all: build

# ─── Build ────────────────────────────────────────────────────────────────────

build: build-cli build-daemon

build-cli:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)/

build-daemon:
	@echo "Building $(DAEMON_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(DAEMON_NAME) ./cmd/$(DAEMON_NAME)/

# ─── Install ──────────────────────────────────────────────────────────────────

install: build
	@echo "Installing to /usr/local/bin..."
	sudo cp -f $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)
	sudo cp -f $(BUILD_DIR)/$(DAEMON_NAME) /usr/local/bin/$(DAEMON_NAME)
	sudo chmod 755 /usr/local/bin/$(APP_NAME) /usr/local/bin/$(DAEMON_NAME)
	@echo "Installation complete."

# ─── Run ──────────────────────────────────────────────────────────────────────

run-daemon: build-daemon
	sudo $(BUILD_DIR)/$(DAEMON_NAME) --config configs/config.yaml

run-cli: build-cli
	$(BUILD_DIR)/$(APP_NAME) $(ARGS)

tui:
	@echo "TUI runs as: $(APP_NAME) tui  (or run with bubbletea program)"

# ─── Test ─────────────────────────────────────────────────────────────────────

test:
	$(GO) test $(PKG) -v -count=1 -timeout 60s

test-race:
	$(GO) test $(PKG) -v -race -count=1 -timeout 120s

test-cover:
	$(GO) test $(PKG) -coverprofile=coverage.out -covermode=atomic
	$(GO) tool cover -html=coverage.out -o coverage.html

# ─── Lint ─────────────────────────────────────────────────────────────────────

lint:
	golangci-lint run $(PKG) 2>/dev/null || echo "golangci-lint not installed — run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"

vet:
	$(GO) vet $(PKG)

fmt:
	$(GO) fmt $(PKG)

# ─── Clean ────────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	$(GO) clean -cache

# ─── Dependencies ─────────────────────────────────────────────────────────────

deps:
	$(GO) mod tidy
	$(GO) mod download

# ─── Docker (optional) ────────────────────────────────────────────────────────

docker-build:
	docker build -t bandwidth-manager .

# ─── Help ─────────────────────────────────────────────────────────────────────

help:
	@echo "Bandwidth Manager — Build System"
	@echo ""
	@echo "Targets:"
	@echo "  make build        Build both binaries"
	@echo "  make build-cli    Build CLI only"
	@echo "  make build-daemon Build daemon only"
	@echo "  make install      Build and install to /usr/local/bin"
	@echo "  make run-daemon   Run daemon (with sudo)"
	@echo "  make run-cli      Run CLI"
	@echo "  make test         Run unit tests"
	@echo "  make test-race    Run tests with race detector"
	@echo "  make test-cover   Run tests with coverage"
	@echo "  make lint         Lint code"
	@echo "  make vet          Go vet"
	@echo "  make fmt          Format code"
	@echo "  make clean        Remove build artifacts"
	@echo "  make deps         Download dependencies"
