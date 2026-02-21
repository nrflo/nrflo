#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

START_TIME=$(date +%s)
node_modules/.bin/vitest run "$@"
END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

if [ "$ELAPSED" -gt 15 ]; then
    echo ""
    echo "CRITICAL: TEST SUITE TOOK ${ELAPSED}s, IT SHOULD BE LESS THAN 15 SECONDS TOTAL, ANALYZE AND FIX IT"
    echo "  Hints: eliminate setTimeout/sleep in tests, use fake timers (vi.useFakeTimers), use never-resolving promises for isPending tests"
fi
