#!/usr/bin/env bash
# context-check.sh — Claude Code hook to track context usage per session.
#
# Install as a Claude Code PreToolUse hook in .claude/settings.json:
#   {
#     "hooks": {
#       "PreToolUse": [
#         {
#           "matcher": "",
#           "command": "/path/to/context-check.sh"
#         }
#       ]
#     }
#   }
#
# The hook reads the JSON event from stdin (Claude Code passes it),
# extracts session_id and context usage, then writes to /tmp/usable_context.json.
# The spawner reads this file to track how much context each agent has left.
#
# Only runs when NRWF_SPAWNED=1 (set by spawner on spawned agents).

set -euo pipefail

# Only active for spawned agents
[ "${NRWF_SPAWNED:-}" = "1" ] || exit 0

# Read hook input from stdin
INPUT=$(cat)

# Extract session_id from CLAUDE_SESSION_ID env var (set by claude --session-id)
SESSION_ID="${CLAUDE_SESSION_ID:-}"
[ -n "$SESSION_ID" ] || exit 0

# Extract context usage from the hook input JSON
# Claude Code passes conversation_id, session_id, and token usage info
USED_PCT=$(echo "$INPUT" | jq -r '.session.usage.cache_creation_input_tokens // empty' 2>/dev/null || true)

# Try to get percentage from the stats field if available
if [ -z "$USED_PCT" ]; then
    USED_PCT=$(echo "$INPUT" | jq -r '.session.context_window.used_percentage // empty' 2>/dev/null || true)
fi

# If we got a usage percentage, write it to the shared context file
if [ -n "$USED_PCT" ]; then
    CONTEXT_FILE="/tmp/usable_context.json"

    # Read existing data or start fresh
    if [ -f "$CONTEXT_FILE" ]; then
        EXISTING=$(cat "$CONTEXT_FILE" 2>/dev/null || echo '{}')
    else
        EXISTING='{}'
    fi

    # Compute remaining percentage
    REMAINING=$((100 - USED_PCT))

    # Update the JSON with this session's context info
    echo "$EXISTING" | jq --arg sid "$SESSION_ID" \
        --argjson used "$USED_PCT" \
        --argjson remaining "$REMAINING" \
        '.[$sid] = {"used_percentage": $used, "remaining_percentage": $remaining}' \
        > "${CONTEXT_FILE}.tmp" 2>/dev/null && mv "${CONTEXT_FILE}.tmp" "$CONTEXT_FILE" || true
fi

# Check threshold and signal continuation if needed
THRESHOLD="${NRWF_CONTEXT_THRESHOLD:-15}"
if [ -n "$USED_PCT" ] && [ "$USED_PCT" -ge $((100 - THRESHOLD)) ] 2>/dev/null; then
    # Context is running low — the spawner will detect this via the context file
    # and handle continuation. We just ensure the data is written.
    :
fi

exit 0
