#!/bin/bash
# Start the nrworkflow API server

set -e

PORT=${NRWORKFLOW_PORT:-6587}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting nrworkflow API Server${NC}"
echo "================================"

# Check if nrworkflow_server is installed
if ! command -v nrworkflow_server &> /dev/null; then
    echo -e "${RED}Error: nrworkflow_server command not found${NC}"
    echo "Please install nrworkflow_server first:"
    echo "  cd ~/.nrworkflow/be && make build && sudo make install"
    exit 1
fi

echo -e "Using: ${YELLOW}$(which nrworkflow_server)${NC}"

# Kill existing server if running on API port
if lsof -i :$PORT > /dev/null 2>&1; then
    echo -e "${YELLOW}Killing existing process on port $PORT...${NC}"
    lsof -ti :$PORT | xargs kill -9 2>/dev/null || true
    sleep 1
fi

# Kill existing server if running on UI port (5175)
if lsof -i :5175 > /dev/null 2>&1; then
    echo -e "${YELLOW}Killing existing process on port 5175...${NC}"
    lsof -ti :5175 | xargs kill -9 2>/dev/null || true
    sleep 1
fi

echo ""
echo -e "API server will start on ${YELLOW}http://localhost:$PORT${NC}"
echo -e "UI will start on ${YELLOW}http://localhost:5175${NC}"
echo "Press Ctrl+C to stop"
echo ""

# Start the API server in background
nrworkflow_server serve --port=$PORT &
API_PID=$!

# Give the API server a moment to start
sleep 1

# Start the UI dev server
cd "$(dirname "$0")"
npm run dev &
UI_PID=$!

# Handle cleanup on exit
cleanup() {
    echo ""
    echo -e "${YELLOW}Shutting down...${NC}"
    kill $UI_PID 2>/dev/null
    kill $API_PID 2>/dev/null
    exit 0
}

trap cleanup SIGINT SIGTERM

# Wait for both processes
wait
