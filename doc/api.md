# API Mode (`execution_mode=api`)

For shared concepts (template variables, findings patterns, lifecycle,
workflow config, resilience, examples), see the **Common** tab.

This file covers what is specific to the `api` execution mode: the in-process
Anthropic runner, enabling it, supported models, tool registry, multimodal
tool results, and behavioral notes.

---

## What It Is

API mode runs agents as an in-process tool-use loop using the Anthropic SDK
rather than spawning a CLI process. Each agent turn calls the Anthropic
Messages API, dispatches tool invocations to registered handlers, and loops
until `end_turn` or a terminal signal.

---

## Enabling

API mode is controlled by the `api_mode_enabled` global setting. Toggle it at
**Settings → Administration**. When off, any spawn request for an api-mode
agent returns an error immediately.

---

## Supported Models

API mode supports Anthropic models only:

| `model` value | Resolved to |
|---------------|-------------|
| `opus_4_6` | `claude-opus-4-6` (200k) |
| `opus_4_6_1m` | `claude-opus-4-6` (1M) |
| `opus_4_7` | `claude-opus-4-7` (200k) |
| `opus_4_7_1m` | `claude-opus-4-7` (1M) |
| `sonnet` | Claude Sonnet |
| `haiku` | Claude Haiku |

Non-Anthropic model values (opencode/codex/gemini prefixes) are invalid for
api-mode agents and will cause spawn to fail.

---

## Tool Registry

An api-mode agent's available tools are determined by the `tools` field on the
agent definition — a comma-separated list of tool name globs resolved at spawn
time. Empty = no tools; `*` = all tools in scope.

**Builtin tools** (always available when matched by glob):

| Tool | Description |
|------|-------------|
| `agent_finished` | Signal task success |
| `agent_fail` | Signal task failure |
| `agent_continue` | Signal context exhaustion |
| `agent_callback` | Trigger callback to re-run earlier layers |
| `findings_add` | Add/update own findings |
| `findings_get` | Read own or cross-agent findings |
| `findings_append` | Append to own findings |
| `findings_delete` | Delete own findings |
| `project_findings_add` | Add/update project-level findings |
| `project_findings_get` | Read project-level findings |
| `project_findings_append` | Append to project-level findings |
| `project_findings_delete` | Delete project-level findings |
| `read_document` | Materialize a named artifact and return it as a native content block |
| `artifact_add` | Upload a file as a named artifact |
| `artifact_list` | List artifacts for this workflow instance |
| `artifact_get` | Get the local path of a materialized artifact |
| `workflow_skip` | Add a skip tag to the workflow instance |
| `workflow_continue` | Resume a paused (waiting) workflow instance. Input: `{instance_id, instructions?}` |
| `workflow_fail` | Fail a workflow instance with a reason. Input: `{instance_id, reason}` |

**`read_document`** materializes a named input artifact and returns its bytes
as an image or document content block so the model can read it natively (OCR
scanned PDFs, photos). PDF → document block; PNG/JPEG → image block. Capped
at 32 MiB.

**HTTP tool definitions** — custom tools whose invocations are forwarded via
HTTP POST to a configured endpoint. Defined per-project and scoped by project
and optional workflow. Auth methods: none, `bearer_env`, `bearer_secret_ref`.

**Python tools** — `python_scripts` rows with `kind=tool`. Each invocation
writes the script to a temp file and execs `pythonPath` with JSON input on
stdin. Input is validated against the script's declared JSON Schema (Draft 2020)
before execution. Like script agents, a tool's source can be inline `code` or an
optional absolute `.py` `file_path` (file contents override `code`); set it via
the file-path picker on the Python Tool form.

### Tool Name Collision Rules

- Python tool name collides with a builtin → spawn fails
- HTTP tool name collides with a builtin or python tool → spawn fails
- Glob produces no matches → spawn fails with `no tools matched`

---

## Rate-Limit Behavior

API-mode rate-limit detection uses typed SDK errors only — no string matching.
`sdk.Error` with `ErrorTypeRateLimitError` / `ErrorTypeOverloadedError`, or
HTTP status 429/529, triggers rate-limit restart. The same config keys apply as
for CLI mode (see the **CLI** tab): `rate_limit_enabled`,
`rate_limit_initial_backoff_sec`, `rate_limit_max_wait_sec`.
The adapter name for api-mode pattern config is `"api"`.

---

## Low-Context Behavior

API-mode agents use the agent-save path exclusively (no resume-based save —
that path is Claude-CLI-only). On context threshold breach: runner is cancelled,
a `context-saver` haiku agent summarizes history, then the agent is relaunched
with `${PREVIOUS_DATA}` via the standard low-context injectable block (see
**Common** → Low-Context Continuation).

---

## Lifecycle as Builtin Tools

API-mode agents call lifecycle via tool invocations rather than CLI commands:
`agent_finished`, `agent_fail`, `agent_continue`, `agent_callback`. These
produce identical DB rows and WebSocket broadcasts as their CLI equivalents.
They are registered in the tool registry and must be matched by the agent's
`tools` glob (or `*`) to be available.

---

For implementation depth on the turn loop, provider error classification, and
per-agent registry resolution, see
[be/internal/spawner/apirun/CLAUDE.md](../be/internal/spawner/apirun/CLAUDE.md).

---

## REST Continue/Fail Endpoints

External callers (service tokens, web UI) can continue or fail a paused workflow
via REST:

- `POST /api/v1/tickets/{id}/workflow/continue` — body `{workflow, instructions?}`
- `POST /api/v1/tickets/{id}/workflow/fail` — body `{workflow, reason}`
- `POST /api/v1/projects/{id}/workflow/continue` — body `{instance_id, instructions?}`
- `POST /api/v1/projects/{id}/workflow/fail` — body `{instance_id, reason}`

All routes accept SCS sessions, spawn tokens, and service tokens (same as
`retry-failed`). See [be/internal/api/CLAUDE.md](../be/internal/api/CLAUDE.md).
