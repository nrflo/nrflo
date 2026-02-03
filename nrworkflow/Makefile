.PHONY: build build-release clean test install

GO := /opt/homebrew/bin/go

# Development build
build:
	$(GO) build -o nrworkflow ./cmd/nrworkflow

# Release build for Apple Silicon (no CGO, stripped)
build-release:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o nrworkflow ./cmd/nrworkflow

# Clean build artifacts
clean:
	rm -f nrworkflow
	rm -f nrworkflow.data

# Run tests
test:
	$(GO) test ./...

# Install to /usr/local/bin
install: build-release
	sudo cp nrworkflow /usr/local/bin/

# Tidy dependencies
tidy:
	$(GO) mod tidy
