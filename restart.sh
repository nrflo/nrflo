#!/bin/bash
# Restart nrworkflow server (single-process, serves API + embedded UI)
# Kills existing process, rebuilds, and starts as background task

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PORT=${NRWORKFLOW_PORT:-6587}
LOG_DIR="/tmp/nrworkflow/logs"
BE_LOG="$LOG_DIR/be.log"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}    nrworkflow Server Restart Script    ${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo ""

# Create logs directory
mkdir -p "$LOG_DIR"

# ─────────────────────────────────────────
# Step 1: Stop existing server
# ─────────────────────────────────────────
echo -e "${YELLOW}[1/3] Stopping existing server...${NC}"

if lsof -i :$PORT > /dev/null 2>&1; then
    echo "  Killing server on port $PORT..."
    lsof -ti :$PORT | xargs kill -9 2>/dev/null || true
    sleep 1
else
    echo "  No server running on port $PORT"
fi

echo -e "${GREEN}  Done${NC}"
echo ""

# ─────────────────────────────────────────
# Step 2: Build
# ─────────────────────────────────────────
echo -e "${YELLOW}[2/3] Building server (includes UI)...${NC}"

cd "$SCRIPT_DIR/be"
if make build; then
    echo -e "${GREEN}  Server built successfully${NC}"
else
    echo -e "${RED}  Build failed!${NC}"
    exit 1
fi

sudo ln -sf "$SCRIPT_DIR/be/nrworkflow" /usr/local/bin/nrworkflow
echo -e "${GREEN}  CLI symlinked to /usr/local/bin/nrworkflow${NC}"
echo ""

# ─────────────────────────────────────────
# Step 3: Start server in background
# ─────────────────────────────────────────
echo -e "${YELLOW}[3/3] Starting server...${NC}"

cd "$SCRIPT_DIR"
echo "  Starting server on port $PORT..."
nohup "$SCRIPT_DIR/be/nrworkflow_server" serve --port=$PORT > "$BE_LOG" 2>&1 &
BE_PID=$!
echo "  Server PID: $BE_PID"

# Give server a moment to start
sleep 1

# Check if server started successfully
if ! kill -0 $BE_PID 2>/dev/null; then
    echo -e "${RED}  Server failed to start! Check $BE_LOG${NC}"
    exit 1
fi

echo -e "${GREEN}  Done${NC}"
echo ""

# ─────────────────────────────────────────
# Summary
# ─────────────────────────────────────────
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${GREEN}Server started successfully!${NC}"
echo ""
echo -e "  URL: ${YELLOW}http://localhost:$PORT${NC} (PID: $BE_PID)"
echo ""
echo "Log: $BE_LOG"
echo ""
echo "To stop:"
echo "  kill $BE_PID"
echo "  # or: ./stop.sh"
echo -e "${BLUE}═══════════════════════════════════════${NC}"
