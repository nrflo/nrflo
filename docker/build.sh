#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Cross-compiling nrflow CLI for linux/arm64..."
cd "$PROJECT_DIR/be"
make build-cli-linux

echo "==> Building Docker image..."
docker build --platform linux/arm64 -t nrflow-agent "$SCRIPT_DIR/"

echo "==> Cleaning up cross-compiled binary..."
rm -f "$SCRIPT_DIR/nrflow"

echo "==> Done. Image: nrflow-agent"
