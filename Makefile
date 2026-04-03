.PHONY: all build build-cli build-server build-server-only build-ui \
       build-release build-release-cli build-release-server \
       install clean test test-ui test-integration test-pkg test-verbose \
       test-coverage test-race tidy help

# --- Configurable variables ---
PREFIX     ?= /usr/local
BINDIR     ?= $(PREFIX)/bin
GO         ?= go
NPM        ?= npm
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -s -w -X be/internal/cli.version=$(VERSION)
CGO_CLI     ?= 0
CGO_SERVER  ?= 1
GOOS       ?= $(shell $(GO) env GOOS)
GOARCH     ?= $(shell $(GO) env GOARCH)

# --- Directories ---
BE_DIR     := be
UI_DIR     := ui
STATIC_DIR := $(BE_DIR)/internal/static/dist

# --- Primary targets ---

all: build

## build: Build both binaries (dev, includes UI)
build: build-cli build-server

## build-cli: Build CLI binary only (no CGO, no tray)
build-cli:
	cd $(BE_DIR) && CGO_ENABLED=0 $(GO) build -o nrflow ./cmd/nrflow

## build-ui: Build UI and copy dist to embed directory
build-ui:
	cd $(UI_DIR) && $(NPM) run build
	rm -rf $(STATIC_DIR)
	cp -r $(UI_DIR)/dist $(STATIC_DIR)
	cp agent_manual.md $(BE_DIR)/internal/static/agent_manual.md

## build-server: Build server binary with tray (includes UI build)
build-server: build-ui
	cd $(BE_DIR) && $(GO) build -tags tray -o nrflow_server ./cmd/server

## build-server-only: Go-only server rebuild (skip UI build)
build-server-only:
	cd $(BE_DIR) && $(GO) build -tags tray -o nrflow_server ./cmd/server

# --- Release builds ---

## build-release: Optimized release build (both binaries, includes UI)
build-release: build-release-cli build-release-server

## build-release-cli: Release build CLI only (pure Go, no CGO)
build-release-cli:
	cd $(BE_DIR) && CGO_ENABLED=$(CGO_CLI) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -ldflags="$(LDFLAGS)" -o nrflow ./cmd/nrflow

## build-release-server: Release build server only (CGO for systray)
build-release-server: build-ui
	cd $(BE_DIR) && CGO_ENABLED=$(CGO_SERVER) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -tags tray -ldflags="$(LDFLAGS)" -o nrflow_server ./cmd/server

# --- Install ---

## install: Install both binaries to PREFIX (default /usr/local)
install: build-release
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(BE_DIR)/nrflow $(DESTDIR)$(BINDIR)/nrflow
	install -m 755 $(BE_DIR)/nrflow_server $(DESTDIR)$(BINDIR)/nrflow_server

# --- Test ---

## test: Run backend tests (30s wall-time constraint)
test:
	@START=$$(date +%s); \
	cd $(BE_DIR) && $(GO) test ./internal/... -count=1; \
	ELAPSED=$$(( $$(date +%s) - $$START )); \
	if [ "$$ELAPSED" -gt 30 ]; then \
		echo ""; \
		echo "CRITICAL: TEST SUITE TOOK $${ELAPSED}s, MUST BE UNDER 30 SECONDS. ANALYZE AND FIX."; \
		exit 1; \
	fi

## test-ui: Run frontend tests (30s wall-time constraint). Use ARGS= for path filter.
test-ui:
	@START=$$(date +%s); \
	cd $(UI_DIR) && npx vitest run $(ARGS); \
	ELAPSED=$$(( $$(date +%s) - $$START )); \
	if [ "$$ELAPSED" -gt 30 ]; then \
		echo ""; \
		echo "CRITICAL: TEST SUITE TOOK $${ELAPSED}s, MUST BE UNDER 30 SECONDS. ANALYZE AND FIX."; \
		exit 1; \
	fi

## test-integration: Run integration tests (verbose)
test-integration:
	cd $(BE_DIR) && $(GO) test -v ./internal/integration/...

## test-pkg: Run tests for a specific package (usage: make test-pkg PKG=orchestrator)
test-pkg:
	cd $(BE_DIR) && $(GO) test -v ./internal/$(PKG)/...

## test-verbose: Run all backend tests (verbose)
test-verbose:
	cd $(BE_DIR) && $(GO) test -v ./internal/... -count=1

## test-coverage: Run backend tests with coverage report
test-coverage:
	cd $(BE_DIR) && $(GO) test -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/... ./internal/... -count=1
	@cd $(BE_DIR) && $(GO) tool cover -func=coverage.out | tail -1
	@echo "Full report: cd be && go tool cover -html=coverage.out"

## test-race: Run backend tests with race detector
test-race:
	cd $(BE_DIR) && $(GO) test -race ./internal/... -count=1

# --- Housekeeping ---

## tidy: Tidy Go module dependencies
tidy:
	cd $(BE_DIR) && $(GO) mod tidy

## clean: Remove build artifacts
clean:
	rm -f $(BE_DIR)/nrflow $(BE_DIR)/nrflow_server
	rm -rf $(STATIC_DIR)
	rm -f $(BE_DIR)/internal/static/agent_manual.md
	mkdir -p $(STATIC_DIR) && touch $(STATIC_DIR)/.gitkeep

## help: Show available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | column -t -s ':'
