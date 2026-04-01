.PHONY: build build-cli build-server build-server-only build-ui build-release build-cli-release build-server-release build-cli-linux docker-build clean test test-integration test-pkg test-verbose install tidy

GO := /opt/homebrew/bin/go

# Build both binaries (development)
build: build-cli build-server

# Development build (CLI binary)
build-cli:
	$(GO) build -o nrworkflow ./cmd/nrworkflow

# Build UI and copy dist to embed directory
build-ui:
	cd ../ui && npm run build
	rm -rf internal/static/dist
	cp -r ../ui/dist internal/static/dist

# Development build (server binary, includes UI build)
build-server: build-ui
	$(GO) build -o nrworkflow_server ./cmd/server

# Fast Go-only rebuild (skips UI build, uses last-copied dist)
build-server-only:
	$(GO) build -o nrworkflow_server ./cmd/server

# Release builds (both binaries)
build-release: build-cli-release build-server-release

# Release build for CLI (Apple Silicon, no CGO, stripped)
build-cli-release:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o nrworkflow ./cmd/nrworkflow

# Release build for server (Apple Silicon, no CGO, stripped)
build-server-release: build-ui
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o nrworkflow_server ./cmd/server

# Cross-compile CLI for linux/arm64 (Docker image)
build-cli-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -ldflags="-s -w" -o ../docker/nrworkflow ./cmd/nrworkflow

# Build Docker image (cross-compiles CLI first)
docker-build: build-cli-linux
	docker build --platform linux/arm64 -t nrworkflow-agent ../docker/
	rm -f ../docker/nrworkflow

# Clean build artifacts
clean:
	rm -f nrworkflow nrworkflow_server
	rm -f nrworkflow.data
	rm -rf internal/static/dist
	mkdir -p internal/static/dist && touch internal/static/dist/.gitkeep

# Run tests
test:
	$(GO) test ./...

# Run integration tests only
test-integration:
	$(GO) test -v ./internal/integration/...

# Run tests for a specific package (usage: make test-pkg PKG=orchestrator)
test-pkg:
	$(GO) test -v ./internal/$(PKG)/...

# Run all tests verbose
test-verbose:
	$(GO) test -v ./...

# Install both to /usr/local/bin
install: build-release
	sudo cp nrworkflow /usr/local/bin/
	sudo cp nrworkflow_server /usr/local/bin/

# Tidy dependencies
tidy:
	$(GO) mod tidy
