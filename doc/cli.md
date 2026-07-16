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
| `claude` | `ClaudeAdapter` | Unified model IDs mapped to Claude model names | `--settings` hooks |
| `codex` | `CodexAdapter` | Unified model IDs with per-agent reasoning effort | app-server JSON-RPC (token usage) |

---

## Supported Models

The `model` value is the unified registry ID shown under **Settings → Models**.
The seeded CLI-capable rows are:

| provider | id | CLI model | context | default effort |
|----------|----|-----------|---------|----------------|
| anthropic | `fable-5` | `claude-fable-5` | 1M | provider default |
| anthropic | `sonnet-5` | `claude-sonnet-5` | 1M | provider default |
| anthropic | `haiku-4-5` | `claude-haiku-4-5` | 200k | provider default |
| anthropic | `opus-4-6`, `opus-4-7`, `opus-4-8` | matching Claude model | 200k | provider default |
| anthropic | `opus-4-6-1m`, `opus-4-7-1m`, `opus-4-8-1m` | matching Claude model with `[1m]` | 1M | provider default |
| openai | `gpt-5.2` | `gpt-5.2` | 200k | medium |
| openai | `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.5` | matching GPT model | 200k | medium |
| openai | `gpt-5.6-sol` | `gpt-5.6-sol` | 372k | low |
| openai | `gpt-5.6-terra`, `gpt-5.6-luna` | matching GPT model | 372k | medium |

`reasoning_effort` may override the row default when the selected model supports
that level. The API exposes the exact per-mode effort lists; custom enabled rows
appear alongside the seeded rows.

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

Replace `<adapter>` with `claude` or `codex`.

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
Codex agents use adapter-specific context tracking (JSON-RPC token usage) instead.

---

## Local console (`nrflo_server console`)

This manual covers spawned, `cli_interactive` agents. The human console is a
different surface: `nrflo_server console` opens the server-driven resume/model
picker, while `--engine ... [--model ...]` starts directly and `--resume ...`
reattaches a live chat. Ctrl+D detaches without stopping the provider; see the
root [README.md](../README.md#local-console-nrflo_server-console).

---

For implementation depth on adapters, hook injection, and context-save paths,
see [be/internal/spawner/CLAUDE.md](../be/internal/spawner/CLAUDE.md).
