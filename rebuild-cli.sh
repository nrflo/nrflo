#!/bin/bash
# Rebuild and re-symlink the nrworkflow CLI binary
# Use this after code changes to update the CLI without restarting the server

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}Building nrworkflow CLI...${NC}"

cd "$SCRIPT_DIR/be"
if make build-cli; then
    echo -e "${GREEN}CLI built successfully${NC}"
else
    echo -e "${RED}CLI build failed!${NC}"
    exit 1
fi

sudo ln -sf "$SCRIPT_DIR/be/nrworkflow" /usr/local/bin/nrworkflow
echo -e "${GREEN}Symlinked to /usr/local/bin/nrworkflow${NC}"

# Rebuild Docker image if it exists
if docker image inspect nrworkflow-agent >/dev/null 2>&1; then
    echo -e "${YELLOW}Rebuilding Docker image with updated CLI...${NC}"
    cd "$SCRIPT_DIR/be"
    make docker-build
    echo -e "${GREEN}Docker image rebuilt${NC}"
fi

echo "Done."
