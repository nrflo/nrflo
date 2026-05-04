.PHONY: all build build-cli build-server build-server-only build-ui \
       build-release build-release-cli build-release-server \
       install clean test test-ui test-integration test-pkg test-verbose \
       test-coverage test-race tidy release-check release-dry-run help \
       embed-assets docker-build docker-buildx docker-login

# --- Configurable variables ---
PREFIX     ?= /usr/local
BINDIR     ?= $(PREFIX)/bin
GO         ?= go
NPM        ?= npm
VERSION    ?= $(shell if [ -f VERSION ]; then printf 'v'; cat VERSION; else git describe --tags --always --dirty 2>/dev/null || echo "dev"; fi)
LDFLAGS    := -s -w -X be/internal/cli.version=$(VERSION)
CGO_CLI     ?= 0
CGO_SERVER  ?= 1
GOOS       ?= $(shell $(GO) env GOOS)
GOARCH     ?= $(shell $(GO) env GOARCH)

# --- Directories ---
BE_DIR     := be
UI_DIR     := ui
STATIC_DIR := $(BE_DIR)/internal/static/dist
EMBED_MANUAL := $(BE_DIR)/internal/static/agent_manual.md
EMBED_GITKEEP := $(STATIC_DIR)/.gitkeep

# --- Primary targets ---

all: build

## embed-assets: Ensure go:embed prerequisites exist (agent_manual.md + dist/.gitkeep). Required before any go build/test.
embed-assets: $(EMBED_MANUAL) $(EMBED_GITKEEP)

$(EMBED_MANUAL): agent_manual.md
	cp $< $@

$(EMBED_GITKEEP):
	@mkdir -p $(STATIC_DIR) && touch $@

## build: Build both binaries (dev, includes UI)
build: build-cli build-server

## build-cli: Build CLI binary only (no CGO, no tray)
build-cli: embed-assets
	cd $(BE_DIR) && CGO_ENABLED=0 $(GO) build -o nrflo ./cmd/nrflo

## build-ui: Build UI and copy dist to embed directory
build-ui:
	cd $(UI_DIR) && $(NPM) run build
	rm -rf $(STATIC_DIR)
	cp -r $(UI_DIR)/dist $(STATIC_DIR)
	cp agent_manual.md $(BE_DIR)/internal/static/agent_manual.md

## build-server: Build server binary with tray (includes UI build)
build-server: build-ui
	cd $(BE_DIR) && $(GO) build -tags tray -o nrflo_server ./cmd/server

## build-server-only: Go-only server rebuild (skip UI build)
build-server-only: embed-assets
	cd $(BE_DIR) && $(GO) build -tags tray -o nrflo_server ./cmd/server

# --- Release builds ---

## build-release: Optimized release build (both binaries, includes UI)
build-release: build-release-cli build-release-server

## build-release-cli: Release build CLI only (pure Go, no CGO)
build-release-cli: embed-assets
	cd $(BE_DIR) && CGO_ENABLED=$(CGO_CLI) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -ldflags="$(LDFLAGS)" -o nrflo ./cmd/nrflo

## build-release-server: Release build server only (CGO for systray)
build-release-server: build-ui
	cd $(BE_DIR) && CGO_ENABLED=$(CGO_SERVER) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -tags tray -ldflags="$(LDFLAGS)" -o nrflo_server ./cmd/server

# --- Install ---

## install: Install both binaries to PREFIX (default /usr/local)
install: build-release
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(BE_DIR)/nrflo $(DESTDIR)$(BINDIR)/nrflo
	install -m 755 $(BE_DIR)/nrflo_server $(DESTDIR)$(BINDIR)/nrflo_server

# --- Test ---
# Separate locks for BE/FE prevent concurrent runs within the same toolchain.
# Per-worktree via path hash so parallel worktrees don't block each other.
_LOCK_PFX := /tmp/nrflo-test-$(shell echo "$(CURDIR)" | shasum | cut -c1-8)
BE_LOCK := $(_LOCK_PFX)-be.lock
UI_LOCK := $(_LOCK_PFX)-ui.lock

define acquire_be_lock
	@if ! mkdir $(BE_LOCK) 2>/dev/null; then \
		echo "ERROR: Another BE test run is in progress ($(BE_LOCK))."; \
		echo "If stale, remove with: rmdir $(BE_LOCK)"; \
		exit 1; \
	fi
endef

define acquire_ui_lock
	@if ! mkdir $(UI_LOCK) 2>/dev/null; then \
		echo "ERROR: Another UI test run is in progress ($(UI_LOCK))."; \
		echo "If stale, remove with: rmdir $(UI_LOCK)"; \
		exit 1; \
	fi
endef

## test: Run backend tests (60s wall-time constraint, -p 4 avoids build cache contention)
test: embed-assets
	$(acquire_be_lock)
	@START=$$(date +%s); \
	cd $(BE_DIR) && $(GO) test -p 6 ./internal/... -count=1; \
	RC=$$?; \
	rmdir $(BE_LOCK) 2>/dev/null || true; \
	ELAPSED=$$(( $$(date +%s) - $$START )); \
	if [ "$$ELAPSED" -gt 60 ]; then \
		echo ""; \
		echo "CRITICAL: TEST SUITE TOOK $${ELAPSED}s, MUST BE UNDER 60 SECONDS. ANALYZE AND FIX."; \
		exit 1; \
	fi; \
	exit $$RC

## test-ui: Run frontend tests (60s wall-time constraint). Use ARGS= for path filter.
test-ui:
	$(acquire_ui_lock)
	@START=$$(date +%s); \
	cd $(UI_DIR) && npx vitest run $(ARGS); \
	RC=$$?; \
	rmdir $(UI_LOCK) 2>/dev/null || true; \
	ELAPSED=$$(( $$(date +%s) - $$START )); \
	if [ "$$ELAPSED" -gt 60 ]; then \
		echo ""; \
		echo "CRITICAL: TEST SUITE TOOK $${ELAPSED}s, MUST BE UNDER 60 SECONDS. ANALYZE AND FIX."; \
		exit 1; \
	fi; \
	exit $$RC

## test-integration: Run integration tests (verbose)
test-integration: embed-assets
	$(acquire_be_lock)
	@cd $(BE_DIR) && $(GO) test -v ./internal/integration/...; RC=$$?; rmdir $(BE_LOCK) 2>/dev/null || true; exit $$RC

## test-pkg: Run tests for a specific package (usage: make test-pkg PKG=orchestrator)
test-pkg: embed-assets
	$(acquire_be_lock)
	@cd $(BE_DIR) && $(GO) test -v ./internal/$(PKG)/...; RC=$$?; rmdir $(BE_LOCK) 2>/dev/null || true; exit $$RC

## test-verbose: Run all backend tests (verbose)
test-verbose: embed-assets
	$(acquire_be_lock)
	@cd $(BE_DIR) && $(GO) test -v ./internal/... -count=1; RC=$$?; rmdir $(BE_LOCK) 2>/dev/null || true; exit $$RC

## test-coverage: Run backend tests with coverage report
test-coverage: embed-assets
	$(acquire_be_lock)
	@cd $(BE_DIR) && $(GO) test -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/... ./internal/... -count=1; \
	RC=$$?; rmdir $(BE_LOCK) 2>/dev/null || true; \
	if [ $$RC -eq 0 ]; then \
		cd $(BE_DIR) && $(GO) tool cover -func=coverage.out | tail -1; \
		echo "Full report: cd be && go tool cover -html=coverage.out"; \
	fi; \
	exit $$RC

## test-race: Run backend tests with race detector
test-race: embed-assets
	$(acquire_be_lock)
	@cd $(BE_DIR) && $(GO) test -race ./internal/... -count=1; RC=$$?; rmdir $(BE_LOCK) 2>/dev/null || true; exit $$RC

# --- Housekeeping ---

## tidy: Tidy Go module dependencies
tidy:
	cd $(BE_DIR) && $(GO) mod tidy

## clean: Remove build artifacts
clean:
	rm -f $(BE_DIR)/nrflo $(BE_DIR)/nrflo_server
	rm -rf $(STATIC_DIR)
	rm -f $(BE_DIR)/internal/static/agent_manual.md
	mkdir -p $(STATIC_DIR) && touch $(STATIC_DIR)/.gitkeep

## release-check: Validate GoReleaser config
release-check:
	goreleaser check

## release-dry-run: Test GoReleaser locally (no publish)
release-dry-run:
	goreleaser release --snapshot --clean

# --- Docker (linux/amd64+arm64, api-mode only, pushes to GHCR) ---

IMAGE_REGISTRY ?= ghcr.io
IMAGE_OWNER    ?= nrflo
IMAGE_NAME     ?= nrflo-server
# Strip leading 'v' from VERSION for OCI-style tag (v1.2.3 -> 1.2.3).
IMAGE_TAG      ?= $(VERSION:v%=%)
IMAGE_REF      := $(IMAGE_REGISTRY)/$(IMAGE_OWNER)/$(IMAGE_NAME)
PLATFORMS      ?= linux/amd64,linux/arm64

## docker-build: Build single-arch image locally (host arch) for sanity testing
docker-build:
	docker build \
	  --build-arg VERSION=$(IMAGE_TAG) \
	  -t $(IMAGE_REF):$(IMAGE_TAG) \
	  -t $(IMAGE_REF):latest \
	  .

## docker-buildx: Build & push multi-arch image (linux/amd64,arm64) to $IMAGE_REGISTRY
docker-buildx:
	@docker buildx inspect nrflo-builder >/dev/null 2>&1 \
	  || docker buildx create --name nrflo-builder --use
	docker buildx build \
	  --platform $(PLATFORMS) \
	  --build-arg VERSION=$(IMAGE_TAG) \
	  -t $(IMAGE_REF):$(IMAGE_TAG) \
	  -t $(IMAGE_REF):latest \
	  --push \
	  .

## docker-login: Log in to $IMAGE_REGISTRY using $CR_PAT or $GITHUB_TOKEN
docker-login:
	@if [ -z "$${CR_PAT}$${GITHUB_TOKEN}" ]; then \
		echo "ERROR: set CR_PAT (a GitHub PAT with write:packages) or GITHUB_TOKEN"; exit 1; \
	fi
	@printf '%s' "$${CR_PAT:-$$GITHUB_TOKEN}" | docker login $(IMAGE_REGISTRY) -u $(IMAGE_OWNER) --password-stdin

## help: Show available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | column -t -s ':'
