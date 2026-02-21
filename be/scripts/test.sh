#!/usr/bin/env bash
set -euo pipefail

GO="${GO:-/opt/homebrew/bin/go}"
cd "$(dirname "$0")/.."

# Defaults
INTEGRATION_ONLY=false
VERBOSE=false
COVERAGE=false
RACE=false
EXTRA_ARGS=()

usage() {
    echo "Usage: $0 [-i] [-v] [-c] [-r] [-h]"
    echo "  -i  Integration tests only"
    echo "  -v  Verbose output"
    echo "  -c  Coverage report"
    echo "  -r  Race detector"
    echo "  -h  Show this help"
    exit 0
}

while getopts "ivcrh" opt; do
    case $opt in
        i) INTEGRATION_ONLY=true ;;
        v) VERBOSE=true ;;
        c) COVERAGE=true ;;
        r) RACE=true ;;
        h) usage ;;
        *) usage ;;
    esac
done

# Build test args
ARGS=()

if $VERBOSE; then
    ARGS+=("-v")
fi

if $RACE; then
    ARGS+=("-race")
fi

if $COVERAGE; then
    ARGS+=("-coverprofile=coverage.out" "-covermode=atomic")
    ARGS+=("-coverpkg=./internal/...")
fi

# Select packages
if $INTEGRATION_ONLY; then
    PKGS="./internal/integration/..."
else
    PKGS="./..."
fi

echo "Running tests: $PKGS"
START_TIME=$(date +%s)
$GO test "${ARGS[@]}" $PKGS
END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

# Coverage summary
if $COVERAGE; then
    echo ""
    echo "=== Coverage Summary ==="
    $GO tool cover -func=coverage.out | tail -1
    echo ""
    echo "Full report: go tool cover -html=coverage.out"
fi

if [ "$ELAPSED" -gt 15 ] && ! $INTEGRATION_ONLY && ! $RACE && ! $COVERAGE; then
    echo ""
    echo "CRITICAL: TEST SUITE TOOK ${ELAPSED}s, IT SHOULD BE LESS THAN 15 SECONDS TOTAL, ANALYZE AND FIX IT"
    echo "  Hints: eliminate time.Sleep, use clock.TestClock.Advance(), use copyTemplateDB() in integration tests"
fi
