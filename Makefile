.PHONY: all build build-server build-server-only build-ui \
       build-release build-release-server \
       build-server-notray build-release-server-notray \
       install clean test test-ui test-integration test-pkg test-verbose \
       test-coverage test-race tidy release-check release-dry-run help \
       embed-assets docker-build docker-buildx docker-login \
       lint lint-fix lint-pkg deadcode filesize filesize-update cleanup

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
EMBED_DOC_DIR := $(BE_DIR)/internal/static/doc
DOC_SRCS      := $(wildcard doc/*.md)
EMBED_DOCS    := $(patsubst doc/%.md,$(EMBED_DOC_DIR)/%.md,$(DOC_SRCS))
EMBED_GITKEEP := $(STATIC_DIR)/.gitkeep

# --- Primary targets ---

all: build

## embed-assets: Ensure go:embed prerequisites exist (doc/*.md + dist/.gitkeep). Required before any go build/test.
embed-assets: $(EMBED_DOCS) $(EMBED_GITKEEP)

$(EMBED_DOC_DIR)/%.md: doc/%.md
	@mkdir -p $(EMBED_DOC_DIR)
	cp $< $@

$(EMBED_GITKEEP):
	@mkdir -p $(STATIC_DIR) && touch $@

## build: Build the server binary (dev, includes UI)
build: build-server

## build-ui: Build UI and copy dist to embed directory
build-ui:
	cd $(UI_DIR) && $(NPM) run build
	rm -rf $(STATIC_DIR)
	cp -r $(UI_DIR)/dist $(STATIC_DIR)
	mkdir -p $(EMBED_DOC_DIR) && cp doc/*.md $(EMBED_DOC_DIR)/

## build-server: Build server binary with tray (includes UI build)
build-server: build-ui
	cd $(BE_DIR) && $(GO) build -tags tray -o nrflo_server ./cmd/server

## build-server-only: Go-only server rebuild (skip UI build)
build-server-only: embed-assets
	cd $(BE_DIR) && $(GO) build -tags tray -o nrflo_server ./cmd/server

# --- Release builds ---

## build-release: Optimized release build (server binary, includes UI)
build-release: build-release-server

## build-release-server: Release build server only (CGO for systray)
build-release-server: build-ui
	cd $(BE_DIR) && CGO_ENABLED=$(CGO_SERVER) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -tags tray -ldflags="$(LDFLAGS)" -o nrflo_server ./cmd/server

## build-server-notray: Build server binary without tray (CGO-free, for Linux)
build-server-notray: build-ui
	cd $(BE_DIR) && CGO_ENABLED=0 $(GO) build -o nrflo_server ./cmd/server

## build-release-server-notray: Release build server without tray (CGO-free cross-compile)
build-release-server-notray: build-ui
	cd $(BE_DIR) && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -ldflags="$(LDFLAGS)" -o nrflo_server ./cmd/server

# --- Install ---

## install: Install the server binary to PREFIX (default /usr/local)
install: build-release
	install -d $(DESTDIR)$(BINDIR)
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

# --- Lint / dead-code (cleanup pipeline) ---
# Pinned tooling is bootstrapped into be/bin (gitignored). golangci-lint's large
# dependency tree is deliberately kept out of be/go.mod.
GOLANGCI_VERSION ?= v2.12.2
BE_BIN     := $(BE_DIR)/bin
GOLANGCI   := $(BE_BIN)/golangci-lint
DEADCODE   := $(BE_BIN)/deadcode

$(GOLANGCI):
	@mkdir -p $(BE_BIN)
	@echo "Installing golangci-lint $(GOLANGCI_VERSION) -> $(BE_BIN)"
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
		| sh -s -- -b $(CURDIR)/$(BE_BIN) $(GOLANGCI_VERSION)

$(DEADCODE):
	@mkdir -p $(BE_BIN)
	@cd $(BE_DIR) && GOBIN=$(CURDIR)/$(BE_BIN) $(GO) install golang.org/x/tools/cmd/deadcode@latest

## lint: Run golangci-lint over the backend (cleanup gate; enforces gofmt)
lint: embed-assets $(GOLANGCI)
	cd $(BE_DIR) && $(CURDIR)/$(GOLANGCI) run ./...

## lint-fix: golangci-lint formatters + autofix
lint-fix: embed-assets $(GOLANGCI)
	cd $(BE_DIR) && $(CURDIR)/$(GOLANGCI) fmt ./... && $(CURDIR)/$(GOLANGCI) run --fix ./...

## lint-pkg: Lint one package (usage: make lint-pkg PKG=orchestrator)
lint-pkg: embed-assets $(GOLANGCI)
	cd $(BE_DIR) && $(CURDIR)/$(GOLANGCI) run ./internal/$(PKG)/...

## deadcode: Report unreachable funcs; fails on NEW dead code vs deadcode.baseline
deadcode: embed-assets $(DEADCODE)
	@cd $(BE_DIR) && $(CURDIR)/$(DEADCODE) -f '{{range .Funcs}}{{println $$.Path .Name}}{{end}}' ./cmd/server 2>/dev/null | sort -u > /tmp/nrflo-deadcode-now.txt
	@grep -v '^#' $(BE_DIR)/deadcode.baseline | sort -u > /tmp/nrflo-deadcode-base.txt
	@new=$$(comm -23 /tmp/nrflo-deadcode-now.txt /tmp/nrflo-deadcode-base.txt); \
	if [ -n "$$new" ]; then \
		echo "NEW unreachable funcs (not in be/deadcode.baseline):"; \
		echo "$$new" | sed 's/^/  /'; \
		echo "Delete the dead code, or (if reached via socket/reflection/tests) add the line(s) to be/deadcode.baseline."; \
		exit 1; \
	fi; \
	echo "deadcode: no new unreachable funcs"

## filesize: Fail on tracked .go/.ts/.tsx files over 300 lines vs filesize.baseline
filesize:
	@scripts/filesize.sh check

## filesize-update: Re-snapshot filesize.baseline (accept the current oversized files)
filesize-update:
	@scripts/filesize.sh update

## cleanup: Full cleanup gate — golangci-lint + dead-code + file-size check
cleanup: lint deadcode filesize

# --- Housekeeping ---

## tidy: Tidy Go module dependencies
tidy:
	cd $(BE_DIR) && $(GO) mod tidy

## clean: Remove build artifacts
clean:
	rm -f $(BE_DIR)/nrflo_server
	rm -rf $(STATIC_DIR)
	rm -rf $(EMBED_DOC_DIR)
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
