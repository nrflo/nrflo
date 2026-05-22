# CLI Mode (`execution_mode=cli_interactive`)

For shared concepts (template variables, findings, lifecycle commands, workflow
config, resilience patterns, examples), see the **Common** tab.

This file covers what is specific to the `cli_interactive` execution mode:
supported CLIs and model names, the safety hook, rate-limit config keys, and
interactive telemetry.

---

## Supported CLIs

| CLI | Adapter | Model Format | Context Tracking |
|-----|---------|--------------|-----------------|
| `claude` | `ClaudeAdapter` | Versioned IDs (`opus_4_7`, `sonnet`) | `--settings` hooks |
| `opencode` | `OpencodeAdapter` | `provider/model` (auto-mapped) | SQLite tail |
| `codex` | `CodexAdapter` | Model aliases with reasoning levels | app-server JSON-RPC (token usage) |
| `gemini` | `GeminiAdapter` | `gemini_pro\|flash\|flash_lite` | JSONL tail |

---

## Supported Models

### Claude (`claude` CLI)

| `model` value | Maps to |
|---------------|---------|
| `opus_4_6` | `claude-opus-4-6` (200k context) |
| `opus_4_6_1m` | `claude-opus-4-6` (1M context) |
| `opus_4_7` | `claude-opus-4-7` (200k context) |
| `opus_4_7_1m` | `claude-opus-4-7` (1M context) |
| `sonnet` | Claude Sonnet |
| `haiku` | Claude Haiku |

### Opencode (`opencode` CLI)

| `model` value | Maps to |
|---------------|---------|
| `opencode_minimax_m25_free` | `opencode/minimax-m2.5-free` |
| `opencode_qwen36_plus_free` | `opencode/qwen3.6-plus-free` |
| `opencode_gpt54` | `openai/gpt-5.4` (variant `high`) |
| `opencode_gpt54_mini_low` | `openai/gpt-5.4-mini` (variant `low`) |

### Codex (`codex` CLI)

| `model` value | Maps to |
|---------------|---------|
| `codex_gpt_normal` | `gpt-5.3-codex` (effort "high") |
| `codex_gpt_high` | `gpt-5.3-codex` (effort "high") |
| `codex_gpt54_normal` | `gpt-5.4` (effort "medium") |
| `codex_gpt54_high` | `gpt-5.4` (effort "high") |
| `codex_gpt54_mini_low` | `gpt-5.4-mini` (effort "low") |

### Gemini (`gemini` CLI)

| `model` value | Maps to |
|---------------|---------|
| `gemini_pro` | `gemini-2.5-pro` |
| `gemini_flash` | `gemini-2.5-flash` |
| `gemini_flash_lite` | `gemini-2.5-flash-lite` |

---

## Safety Hook (Claude only)

The `claude_safety_hook` project config key injects a `--settings <json>` flag
into every Claude command. Configured via **Project Settings → Configuration**.
The JSON is built once at workflow start via `BuildSafetySettingsJSON()`; mid-run
config changes have no effect. Other adapters ignore this setting.

---

## Rate-Limit Config Keys

These project-scoped config keys (project > global) control rate-limit restart
behavior for `cli_interactive` agents:

| Key | Default | Description |
|-----|---------|-------------|
| `rate_limit_enabled` | `true` | Enable/disable rate-limit restart |
| `rate_limit_initial_backoff_sec` | `60` | First retry wait in seconds |
| `rate_limit_max_wait_sec` | `3600` | Max per-step wait |
| `<adapter>_limit_patterns` | adapter defaults | Extra comma-separated rate-limit patterns |
| `<adapter>_error_patterns` | adapter defaults | Extra comma-separated error patterns |

Replace `<adapter>` with `claude`, `opencode`, `codex`, or `gemini`.

When an agent exits non-zero and its output matches a rate-limit pattern, the
spawner broadcasts `agent.rate_limited`, waits with exponential backoff
(`min(InitialBackoff × 2^(retries-1), MaxWait)`), then relaunches.

---

## Interactive Claude Telemetry

When `execution_mode=cli_interactive`, Claude agents run inside a PTY.
nrflo automatically registers `--settings` hooks for `PreToolUse` and
`PostToolUse` events. These hooks pipe structured tool-call data back to nrflo
via the Unix socket, populating the agent's message timeline and keeping
context-usage tracking accurate.

This is transparent — no agent prompt changes or explicit calls are needed.
Opencode, Codex, and Gemini agents use adapter-specific context tracking
(SQLite/JSONL tails) instead.

---

For implementation depth on adapters, hook injection, and context-save paths,
see [be/internal/spawner/CLAUDE.md](../be/internal/spawner/CLAUDE.md).
