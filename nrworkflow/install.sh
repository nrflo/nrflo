#!/bin/bash
set -e

echo "Building wfw..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 /opt/homebrew/bin/go build -ldflags="-s -w" -o wfw ./cmd/wfw

echo "Installing to /usr/local/bin/..."
sudo cp wfw /usr/local/bin/

echo "Done. Installed wfw to /usr/local/bin/wfw"
