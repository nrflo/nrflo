#!/bin/bash
# Nuclear-option Docker image rebuild: stops containers, removes image, rebuilds from scratch
set -e

echo "Stopping stale agent containers..."
docker ps -a --filter name=nrwf- -q | xargs docker rm -f 2>/dev/null || true

echo "Removing old image..."
docker rmi nrworkflow-agent 2>/dev/null || true

echo "Building image (no cache)..."
cd "$(dirname "$0")/be"
make docker-build

echo "Done."
