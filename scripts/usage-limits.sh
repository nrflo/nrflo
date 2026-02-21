#!/usr/bin/env bash
# scripts/usage-limits.sh - Extract Claude Code and Codex CLI usage limits
# Spawns each tool in a PTY via expect, sends /usage or /status, parses output.
set -euo pipefail

DEBUG=false
[[ "${1:-}" == "--debug" ]] && DEBUG=true

TMPDIR_BASE=$(mktemp -d /tmp/usage-limits.XXXXXX)
if $DEBUG; then
  echo "debug: temp dir $TMPDIR_BASE (preserved)" >&2
  trap '' EXIT
else
  trap 'rm -rf "$TMPDIR_BASE"' EXIT
fi

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

strip_ansi() {
  # Replace cursor-right moves (ESC[NC) with spaces, then strip remaining ANSI
  sed $'s/\x1b\[[0-9]*C/ /g' \
    | sed $'s/\x1b\[[0-9;]*[a-zA-Z]//g; s/\x1b\][^\x07]*\x07//g; s/\x1b[()][0-9A-B]//g; s/\x0f//g; s/\r//g' \
    | sed $'s/\x1b\[[?][0-9;]*[a-zA-Z]//g'
}

has_cmd() { command -v "$1" &>/dev/null; }

# Run a command with a hard timeout (macOS-compatible, no coreutils needed)
run_with_timeout() {
  local secs=$1; shift
  "$@" &
  local pid=$!
  (sleep "$secs" && kill "$pid" 2>/dev/null) &
  local watchdog=$!
  wait "$pid" 2>/dev/null
  local rc=$?
  kill "$watchdog" 2>/dev/null
  wait "$watchdog" 2>/dev/null
  return $rc
}

json_null='{"available":false,"five_hour":null,"weekly":null}'

# ---------------------------------------------------------------------------
# Claude Code  (/usage returns JSON)
# ---------------------------------------------------------------------------

scrape_claude() {
  local raw="$TMPDIR_BASE/claude_raw.txt"
  local expect_script="$TMPDIR_BASE/claude.exp"

  cat > "$expect_script" <<'EXPECT'
log_file -noappend RAWFILE
if {[info exists env(CLAUDECODE)]} { unset env(CLAUDECODE) }
spawn -noecho claude

# Wait for "Ctx:" in status line = prompt is ready (up to 10s)
set timeout 10
expect {
    timeout { }
    "Ctx:" { }
}

# Type /usage, wait for autocomplete to settle, then press Enter
send "/usage"
after 1500
send "\r"

# Wait for usage data to render (up to 10s)
set timeout 10
expect {
    timeout { }
    "resets" {
        sleep 2
    }
}

# Exit
send "/exit\r"
set timeout 3
expect {
    timeout { }
    eof { }
}
catch { close }
catch { wait }
EXPECT

  sed -i '' "s|RAWFILE|$raw|" "$expect_script"

  run_with_timeout 30 expect "$expect_script" >/dev/null 2>&1 || true

  if [[ ! -s "$raw" ]]; then
    $DEBUG && echo "debug: claude raw file empty or missing" >&2
    return 1
  fi
  $DEBUG && echo "debug: claude raw ($(wc -c < "$raw") bytes):" >&2
  $DEBUG && cat -v "$raw" >&2

  # Strip ANSI, extract JSON block containing five_hour
  local cleaned
  cleaned=$(strip_ansi < "$raw")

  # Parse /usage output
  # After ANSI stripping, "Resets" may appear as "Rese s" (cursor move → space)
  # Session data may be on one line: "48% used Rese s 9pm (Asia/Bangkok)"
  local parsed
  parsed=$(echo "$cleaned" | python3 -c '
import sys, re, json

text = sys.stdin.read()
# Normalize: fix "Rese s" → "Resets", collapse whitespace around block chars
text = re.sub(r"Rese\s+s\b", "Resets", text)
result = {}

# Current session: NN% used ... Resets TIME
m = re.search(r"Current\s+session.*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?=Current|\n\s*\n|$)", text, re.IGNORECASE | re.DOTALL)
if m:
    result["session_pct"] = float(m.group(1))
    result["session_reset"] = re.sub(r"\s+", " ", m.group(2)).strip()

# Current week (all models): NN% used ... Resets TIME
m = re.search(r"Current\s+week\s*\(all\s+models?\).*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?=Current|\n\s*\n|$)", text, re.IGNORECASE | re.DOTALL)
if m:
    result["weekly_pct"] = float(m.group(1))
    result["weekly_reset"] = re.sub(r"\s+", " ", m.group(2)).strip()

if result:
    print(json.dumps(result))
else:
    sys.exit(1)
' 2>/dev/null) || return 1

  if has_cmd jq; then
    echo "$parsed" | jq '{
      available: true,
      session: { used_pct: .session_pct, resets_at: .session_reset },
      weekly: { used_pct: .weekly_pct, resets_at: .weekly_reset }
    }'
  else
    echo "$parsed" | python3 -c '
import sys, json
d = json.load(sys.stdin)
print(json.dumps({
    "available": True,
    "session": {"used_pct": d.get("session_pct"), "resets_at": d.get("session_reset")},
    "weekly": {"used_pct": d.get("weekly_pct"), "resets_at": d.get("weekly_reset")}
}))
'
  fi
}

# ---------------------------------------------------------------------------
# Codex CLI  (/status returns human-readable text)
# ---------------------------------------------------------------------------

scrape_codex() {
  local raw="$TMPDIR_BASE/codex_raw.txt"
  local expect_script="$TMPDIR_BASE/codex.exp"

  cat > "$expect_script" <<'EXPECT'
log_file -noappend RAWFILE
spawn -noecho codex

# Wait for "context left" in status = prompt is ready (up to 10s)
set timeout 10
expect {
    timeout { }
    "context left" { }
}

# Type /status, wait for autocomplete to settle, then press Enter
send "/status"
after 1500
send "\r"

# Wait for limits data to load (Status tab shows "% left" or "% used")
set timeout 12
expect {
    timeout { }
    "% left" { sleep 1 }
    "% used" { sleep 1 }
}

# Exit via Ctrl+C (codex crashes on /exit due to a Rust wrapping bug)
send "\x03"
set timeout 3
expect {
    timeout { }
    eof { }
}
catch { close }
catch { wait }
EXPECT

  sed -i '' "s|RAWFILE|$raw|" "$expect_script"

  run_with_timeout 30 expect "$expect_script" >/dev/null 2>&1 || true

  if [[ ! -s "$raw" ]]; then
    $DEBUG && echo "debug: codex raw file empty or missing" >&2
    return 1
  fi
  $DEBUG && echo "debug: codex raw ($(wc -c < "$raw") bytes):" >&2
  $DEBUG && cat -v "$raw" >&2

  local cleaned
  cleaned=$(strip_ansi < "$raw")

  # Parse codex /status output
  # Status tab format: "5h limit: [...] 80% left (resets 22:26)"
  #                    "Weekly limit: [...] 71% left (resets 23:26 on 26 Feb)"
  # Fix "Rese s" → "Resets" (cursor-move artifact)
  local parsed
  parsed=$(echo "$cleaned" | python3 -c '
import sys, re, json

text = sys.stdin.read()
text = re.sub(r"Rese\s+s\b", "Resets", text)
result = {}

# Status tab: "5h limit: ... NN% left (resets HH:MM)"
m = re.search(r"5h\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*left.*?\(resets?\s+([^)]+)\)", text, re.IGNORECASE | re.DOTALL)
if m:
    result["session_pct"] = 100.0 - float(m.group(1))
    result["session_reset"] = m.group(2).strip()

m = re.search(r"weekly\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*left.*?\(resets?\s+([^)]+)\)", text, re.IGNORECASE | re.DOTALL)
if m:
    result["weekly_pct"] = 100.0 - float(m.group(1))
    result["weekly_reset"] = m.group(2).strip()

# Fallback: Usage tab format "Current session...NN% used...Resets TIME"
if not result:
    m = re.search(r"Current\s+session.*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?=Current|\n\s*\n|$)", text, re.IGNORECASE | re.DOTALL)
    if m:
        result["session_pct"] = float(m.group(1))
        result["session_reset"] = re.sub(r"\s+", " ", m.group(2)).strip()
    m = re.search(r"Current\s+week.*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?=Current|\n\s*\n|$)", text, re.IGNORECASE | re.DOTALL)
    if m:
        result["weekly_pct"] = float(m.group(1))
        result["weekly_reset"] = re.sub(r"\s+", " ", m.group(2)).strip()

if result:
    print(json.dumps(result))
else:
    sys.exit(1)
' 2>/dev/null) || return 1

  if has_cmd jq; then
    echo "$parsed" | jq '{
      available: true,
      session: { used_pct: .session_pct, resets_at: .session_reset },
      weekly: { used_pct: .weekly_pct, resets_at: .weekly_reset }
    }'
  else
    echo "$parsed" | python3 -c '
import sys, json
d = json.load(sys.stdin)
print(json.dumps({
    "available": True,
    "session": {"used_pct": d.get("session_pct"), "resets_at": d.get("session_reset")},
    "weekly": {"used_pct": d.get("weekly_pct"), "resets_at": d.get("weekly_reset")}
}))
'
  fi
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

# Check expect
if ! has_cmd expect; then
  echo "error: expect is required but not found" >&2
  exit 1
fi

# Scrape Claude
claude_json="$json_null"
if has_cmd claude; then
  if result=$(scrape_claude); then
    claude_json="$result"
  else
    claude_json='{"available":true,"five_hour":null,"weekly":null,"error":"failed to parse /usage output"}'
  fi
fi

# Scrape Codex
codex_json="$json_null"
if has_cmd codex; then
  if result=$(scrape_codex); then
    codex_json="$result"
  else
    codex_json='{"available":true,"session":null,"weekly":null,"error":"failed to parse /status output"}'
  fi
fi

# Timestamp
ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Combine
if has_cmd jq; then
  jq -n \
    --argjson claude "$claude_json" \
    --argjson codex "$codex_json" \
    --arg timestamp "$ts" \
    '{ claude: $claude, codex: $codex, timestamp: $timestamp }'
else
  printf '{"claude":%s,"codex":%s,"timestamp":"%s"}\n' "$claude_json" "$codex_json" "$ts"
fi
