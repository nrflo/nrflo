.PHONY: build build-server build-all build-release clean test test-integration test-verbose install tidy

GO := /opt/homebrew/bin/go

# Development build (CLI binary)
build:
	$(GO) build -o nrworkflow ./cmd/nrworkflow

# Development build (server binary)
build-server:
	$(GO) build -o nrworkflow_server ./cmd/server

# Build both binaries
build-all: build build-server

# Release build for Apple Silicon (no CGO, stripped)
build-release:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o nrworkflow ./cmd/nrworkflow
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o nrworkflow_server ./cmd/server

# Clean build artifacts
clean:
	rm -f nrworkflow
	rm -f nrworkflow_server
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

# Install to /usr/local/bin
install: build-release
	sudo cp nrworkflow /usr/local/bin/
	sudo cp nrworkflow_server /usr/local/bin/

# Tidy dependencies
tidy:
	$(GO) mod tidy
