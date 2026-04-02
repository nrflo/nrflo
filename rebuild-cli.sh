#!/bin/bash
# Rebuild and re-symlink the nrflow CLI binary
# Use this after code changes to update the CLI without restarting the server

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}Building nrflow CLI...${NC}"

cd "$SCRIPT_DIR/be"
if make build-cli; then
    echo -e "${GREEN}CLI built successfully${NC}"
else
    echo -e "${RED}CLI build failed!${NC}"
    exit 1
fi

sudo ln -sf "$SCRIPT_DIR/be/nrflow" /usr/local/bin/nrflow
echo -e "${GREEN}Symlinked to /usr/local/bin/nrflow${NC}"

echo "Done."
