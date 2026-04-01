#!/bin/bash
# Stop nrworkflow server

PORT=${NRWORKFLOW_PORT:-6587}

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Stopping nrworkflow server...${NC}"

if lsof -i :$PORT > /dev/null 2>&1; then
    echo "  Stopping server on port $PORT..."
    lsof -ti :$PORT | xargs kill 2>/dev/null || true
    echo -e "${GREEN}Server stopped${NC}"
else
    echo -e "${YELLOW}No server running on port $PORT${NC}"
fi
