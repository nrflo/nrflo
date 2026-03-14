#!/bin/bash
# Restart nrworkflow servers (Backend + UI)
# Kills existing processes, rebuilds, and starts as background tasks

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PORT=${NRWORKFLOW_PORT:-6587}
UI_PORT=5175
LOG_DIR="/tmp/nrworkflow/logs"
BE_LOG="$LOG_DIR/be.log"
UI_LOG="$LOG_DIR/fe.log"

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
# Step 1: Kill existing servers
# ─────────────────────────────────────────
echo -e "${YELLOW}[1/4] Stopping existing servers...${NC}"

if lsof -i :$PORT > /dev/null 2>&1; then
    echo "  Killing backend on port $PORT..."
    lsof -ti :$PORT | xargs kill -9 2>/dev/null || true
    sleep 1
else
    echo "  No backend running on port $PORT"
fi

if lsof -i :$UI_PORT > /dev/null 2>&1; then
    echo "  Killing UI on port $UI_PORT..."
    lsof -ti :$UI_PORT | xargs kill -9 2>/dev/null || true
    sleep 1
else
    echo "  No UI running on port $UI_PORT"
fi

echo -e "${GREEN}  Done${NC}"
echo ""

# ─────────────────────────────────────────
# Step 2: Rebuild Backend
# ─────────────────────────────────────────
echo -e "${YELLOW}[2/4] Building backend...${NC}"

cd "$SCRIPT_DIR/be"
if make build; then
    echo -e "${GREEN}  Backend built successfully${NC}"
else
    echo -e "${RED}  Backend build failed!${NC}"
    exit 1
fi

sudo ln -sf "$SCRIPT_DIR/be/nrworkflow" /usr/local/bin/nrworkflow
echo -e "${GREEN}  CLI symlinked to /usr/local/bin/nrworkflow${NC}"
echo ""

# ─────────────────────────────────────────
# Step 3: Rebuild UI
# ─────────────────────────────────────────
echo -e "${YELLOW}[3/4] Building UI dependencies...${NC}"

cd "$SCRIPT_DIR/ui"
if npm install --silent 2>/dev/null; then
    echo -e "${GREEN}  UI dependencies installed${NC}"
else
    echo -e "${RED}  npm install failed!${NC}"
    exit 1
fi
echo ""

# ─────────────────────────────────────────
# Step 4: Start servers in background
# ─────────────────────────────────────────
echo -e "${YELLOW}[4/4] Starting servers...${NC}"

# Start backend
cd "$SCRIPT_DIR"
echo "  Starting backend on port $PORT..."
nohup "$SCRIPT_DIR/be/nrworkflow_server" serve --port=$PORT > "$BE_LOG" 2>&1 &
BE_PID=$!
echo "  Backend PID: $BE_PID"

# Give backend a moment to start
sleep 1

# Check if backend started successfully
if ! kill -0 $BE_PID 2>/dev/null; then
    echo -e "${RED}  Backend failed to start! Check $BE_LOG${NC}"
    exit 1
fi

# Start UI
cd "$SCRIPT_DIR/ui"
echo "  Starting UI on port $UI_PORT..."
nohup npm run dev > "$UI_LOG" 2>&1 &
UI_PID=$!
echo "  UI PID: $UI_PID"

# Give UI a moment to start
sleep 2

# Check if UI started successfully
if ! kill -0 $UI_PID 2>/dev/null; then
    echo -e "${RED}  UI failed to start! Check $UI_LOG${NC}"
    exit 1
fi

echo -e "${GREEN}  Done${NC}"
echo ""

# ─────────────────────────────────────────
# Summary
# ─────────────────────────────────────────
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${GREEN}Servers started successfully!${NC}"
echo ""
echo -e "  Backend: ${YELLOW}http://localhost:$PORT${NC} (PID: $BE_PID)"
echo -e "  UI:      ${YELLOW}http://localhost:$UI_PORT${NC} (PID: $UI_PID)"
echo ""
echo "Logs:"
echo "  Backend: $BE_LOG"
echo "  UI:      $UI_LOG"
echo ""
echo "To stop servers:"
echo "  kill $BE_PID $UI_PID"
echo "  # or: lsof -ti :$PORT :$UI_PORT | xargs kill"
echo -e "${BLUE}═══════════════════════════════════════${NC}"
