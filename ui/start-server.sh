#!/bin/bash
# Start the nrworkflow server in foreground (single-process, serves API + embedded UI)

set -e

PORT=${NRWORKFLOW_PORT:-6587}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting nrworkflow Server${NC}"
echo "================================"

# Check if nrworkflow_server is installed
if ! command -v nrworkflow_server &> /dev/null; then
    echo -e "${RED}Error: nrworkflow_server command not found${NC}"
    echo "Please install nrworkflow_server first:"
    echo "  cd be && make build && sudo make install"
    exit 1
fi

echo -e "Using: ${YELLOW}$(which nrworkflow_server)${NC}"

# Kill existing server if running
if lsof -i :$PORT > /dev/null 2>&1; then
    echo -e "${YELLOW}Killing existing process on port $PORT...${NC}"
    lsof -ti :$PORT | xargs kill -9 2>/dev/null || true
    sleep 1
fi

echo ""
echo -e "Server will start on ${YELLOW}http://localhost:$PORT${NC}"
echo "Press Ctrl+C to stop"
echo ""

# Start the server in foreground
nrworkflow_server serve --port=$PORT &
SERVER_PID=$!

# Handle cleanup on exit
cleanup() {
    echo ""
    echo -e "${YELLOW}Shutting down...${NC}"
    kill $SERVER_PID 2>/dev/null
    exit 0
}

trap cleanup SIGINT SIGTERM

# Wait for process
wait
