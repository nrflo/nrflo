#!/bin/bash
set -e

HOME_DIR="/Users/anderfred"

# Linux Claude CLI reads $HOME/.claude.json, macOS stores it at $HOME/.claude/.claude.json
# Symlink so the mounted macOS config is found by the Linux binary
ln -sf "$HOME_DIR/.claude/.claude.json" "$HOME_DIR/.claude.json"

# Ensure onboarding is marked complete
if [ -f "$HOME_DIR/.claude/.claude.json" ]; then
    tmp=$(jq '.hasCompletedOnboarding = true' "$HOME_DIR/.claude/.claude.json") && echo "$tmp" > "$HOME_DIR/.claude/.claude.json"
fi

if [ -n "$HOST_UID" ] && [ -n "$HOST_GID" ]; then
    # Remove any existing user/group with conflicting IDs
    existing_user=$(getent passwd "$HOST_UID" 2>/dev/null | cut -d: -f1 || true)
    if [ -n "$existing_user" ]; then
        deluser "$existing_user" 2>/dev/null || true
    fi
    existing_group=$(getent group "$HOST_GID" 2>/dev/null | cut -d: -f1 || true)
    if [ -n "$existing_group" ]; then
        delgroup "$existing_group" 2>/dev/null || true
    fi

    addgroup -g "$HOST_GID" agentuser
    adduser -D -u "$HOST_UID" -G agentuser -h "$HOME_DIR" -s /bin/bash agentuser
    mkdir -p "$HOME_DIR/.local/state" "$HOME_DIR/.config"
    # chown specific dirs — skip $HOME_DIR recursively to avoid :ro mounts like .ai_common/safety.json
    chown "$HOST_UID:$HOST_GID" "$HOME_DIR"
    chown -R "$HOST_UID:$HOST_GID" "$HOME_DIR/.local" "$HOME_DIR/.config"
    # .claude is selectively mounted (individual files/dirs, not whole dir) — chown what exists
    chown -R "$HOST_UID:$HOST_GID" "$HOME_DIR/.claude" 2>/dev/null || true
    chown -h "$HOST_UID:$HOST_GID" "$HOME_DIR/.claude.json" 2>/dev/null || true
    exec su-exec agentuser "$@"
fi

exec "$@"
