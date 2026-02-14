.PHONY: build build-cli build-server build-release build-cli-release build-server-release clean test test-integration test-verbose install tidy

GO := /opt/homebrew/bin/go

# Build both binaries (development)
build: build-cli build-server

# Development build (CLI binary)
build-cli:
	$(GO) build -o nrworkflow ./cmd/nrworkflow

# Development build (server binary)
build-server:
	$(GO) build -o nrworkflow_server ./cmd/server

# Release builds (both binaries)
build-release: build-cli-release build-server-release

# Release build for CLI (Apple Silicon, no CGO, stripped)
build-cli-release:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o nrworkflow ./cmd/nrworkflow

# Release build for server (Apple Silicon, no CGO, stripped)
build-server-release:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o nrworkflow_server ./cmd/server

# Clean build artifacts
clean:
	rm -f nrworkflow nrworkflow_server
	rm -f nrworkflow.data

# Run tests
test:
	$(GO) test ./...

# Run integration tests only
test-integration:
	$(GO) test -v ./internal/integration/...

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
