#!/bin/bash
# Stop nrworkflow servers (Backend + UI)

PORT=${NRWORKFLOW_PORT:-6587}
UI_PORT=5175

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Stopping nrworkflow servers...${NC}"

stopped=0

if lsof -i :$PORT > /dev/null 2>&1; then
    echo "  Stopping backend on port $PORT..."
    lsof -ti :$PORT | xargs kill 2>/dev/null || true
    stopped=$((stopped + 1))
else
    echo "  No backend running on port $PORT"
fi

if lsof -i :$UI_PORT > /dev/null 2>&1; then
    echo "  Stopping UI on port $UI_PORT..."
    lsof -ti :$UI_PORT | xargs kill 2>/dev/null || true
    stopped=$((stopped + 1))
else
    echo "  No UI running on port $UI_PORT"
fi

if [ $stopped -gt 0 ]; then
    echo -e "${GREEN}Stopped $stopped server(s)${NC}"
else
    echo -e "${YELLOW}No servers were running${NC}"
fi
