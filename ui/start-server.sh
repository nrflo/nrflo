#!/bin/bash
# Start the nrflow server in foreground (single-process, serves API + embedded UI)

set -e

PORT=${NRFLOW_PORT:-6587}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting nrflow Server${NC}"
echo "================================"

# Check if nrflow_server is installed
if ! command -v nrflow_server &> /dev/null; then
    echo -e "${RED}Error: nrflow_server command not found${NC}"
    echo "Please install nrflow_server first:"
    echo "  cd be && make build && sudo make install"
    exit 1
fi

echo -e "Using: ${YELLOW}$(which nrflow_server)${NC}"

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
nrflow_server serve --port=$PORT &
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
