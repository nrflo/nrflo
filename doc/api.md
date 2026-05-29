# API Mode (`execution_mode=api`)

For shared concepts (template variables, findings patterns, lifecycle,
workflow config, resilience, examples), see the **Common** tab.

This file covers what is specific to the `api` execution mode: the in-process
provider runner, enabling it, model/provider selection, credentials, tool
registry, multimodal tool results, and behavioral notes.

---

## What It Is

API mode runs agents as an in-process tool-use loop rather than spawning a
CLI process. The concrete provider (Anthropic or OpenAI) is selected
**per-agent** from the agent's `api_models` row — the `model` field on the
agent definition is an `api_models.id`; the spawner looks up the row, reads
its `provider` column, and calls `BuildAPIProvider(provider, projectID)` at
spawn time. Each agent turn calls the provider's streaming API, dispatches
tool invocations to registered handlers, and loops until `end_turn` or a
terminal signal.

---

## Enabling

API mode is controlled by the `api_mode_enabled` global setting. Toggle it at
**Settings → Administration**. When off, any spawn request for an api-mode
agent returns an error immediately.

**api-via-cli hybrid** (`api_via_cli_enabled`): when on, Anthropic api-models
(`provider=anthropic`) are routed through the Claude CLI billed on your
subscription instead of direct HTTP calls. The tool registry is identical;
tools are served over the MCP bridge (`nrflo_server agent mcp`). OpenAI api-models
are unaffected and continue to use the in-process runner.

---

## Model and Provider Selection

The `model` field on an agent definition must be the `id` of a row in the
`api_models` table. The row's `provider` column selects the backend:

| `provider` | Backend |
|------------|---------|
| `anthropic` | Anthropic Messages API (streaming) |
| `openai` | OpenAI Responses API (streaming) |

**Seeded Anthropic rows** (read-only):

| id | mapped_model | context |
|----|--------------|---------|
| `opus_4_8` | `claude-opus-4-8` | 200k |
| `opus_4_8_1m` | `claude-opus-4-8` | 1M |
| `opus_4_7` | `claude-opus-4-7` | 200k |
| `opus_4_7_1m` | `claude-opus-4-7` | 1M |
| `opus_4_6` | `claude-opus-4-6` | 200k |
| `opus_4_6_1m` | `claude-opus-4-6` | 1M |
| `sonnet` | `claude-sonnet-4-6` | 200k |
| `haiku` | `claude-haiku-4-5` | 200k |

**Seeded OpenAI rows** (read-only):

| id | mapped_model | reasoning_effort |
|----|--------------|-----------------|
| `gpt54_high` | `gpt-5.4` | high |
| `gpt54_medium` | `gpt-5.4` | medium |
| `gpt54_low` | `gpt-5.4` | low |
| `gpt53_codex_high` | `gpt-5.3-codex` | high |
| `gpt53_codex_medium` | `gpt-5.3-codex` | medium |
| `gpt53_codex_low` | `gpt-5.3-codex` | low |

The `reasoning_effort` column is threaded into the OpenAI Responses request
(`be/internal/spawner/apirun/provider/openai/translate.go:28`). Custom rows
can be added via **Settings → Administration → API Models**.

---

## Credentials

Credentials are resolved per-provider at spawn time:

- **Anthropic**: API key or OAuth bearer token (auto-detected from token
  shape — `sk-ant-oat01-` prefix → OAuth bearer, otherwise API key).
  Resolution order: per-project env `ANTHROPIC_API_KEY` →
  `ANTHROPIC_OAUTH_TOKEN` → server-process env (same order). Resolved by
  `be/internal/spawner/apirun/provider/anthropic/credentials.go`.

- **OpenAI**: API key only (no OAuth). Resolution order: per-project env
  `OPENAI_API_KEY` → per-project `CODEX_API_KEY` → server-process env
  `OPENAI_API_KEY` → server-process `CODEX_API_KEY`. Resolved by
  `be/internal/spawner/apirun/provider/openai/credentials.go`.

---

## Tool Registry

An agent's available tools are determined by the `tools` field on the agent
definition — a comma-separated list of tool-name patterns (`*` = all, `prefix*`
= prefix match, or an exact name) resolved at spawn time. For CLI agents
(`cli_interactive` and api-via-cli) the `agent_*` lifecycle tools are always
granted regardless of the CSV, and an empty field means "all tools" (backward
compatible). For in-process `api` agents an empty field means no tools
(text-only).

**Builtin tools** (always available when matched by glob):

| Tool | Description |
|------|-------------|
| `agent_finished` | Signal task success |
| `agent_fail` | Signal task failure |
| `agent_continue` | Signal context exhaustion |
| `agent_callback` | Trigger callback to re-run earlier layers |
| `findings_add` | Add/update own findings |
| `emit_findings` | Validate a finding against the workflow's configured schema for the key, then store it (rejects with the required-structure example on mismatch). Input: `{key, value}` |
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
| `consult` | Ask a named consultant agent a question and receive an inline answer (api-mode only). Input: `{consultant, question}` |

**`read_document`** materializes a named input artifact and returns its bytes
as an image or document content block so the model can read it natively (OCR
scanned PDFs, photos). PDF → document block; PNG/JPEG → image block. Capped
at 32 MiB.

**`emit_findings`** validates a finding before storing it. Each workflow definition may register finding schemas (`key` + JSON Schema Draft 2020 + a known-good example) under Settings. The tool looks up the schema for `key`, validates the supplied `value`, and on success stores it as a session finding. On a schema mismatch — or when the key has no schema — the call is rejected (nothing is stored) and the error result includes the validation message plus the example, so the agent can correct the value and call again. Use `findings_add` for free-form findings that have no schema.

**`consult`** synchronously spawns a named consultant agent (agent definition with `consultant=true` and `execution_mode=api`) under the same workflow instance. The caller's recent message transcript and the question are passed as `${CALLER_TRANSCRIPT}` and `${CONSULT_QUESTION}` template variables. The consultant must write a `_consult_answer` finding (string) and then call `agent_finished`. The answer is returned inline to the calling agent; the `_consult` phase is hidden from the read model. Consultant agents cannot call `consult` themselves (recursion guard). WebSocket events: `consult.started`, `consult.answered`, `consult.failed`.

**Python tools** — `python_scripts` rows with `kind=tool`. Each invocation
writes the script to a temp file and execs `pythonPath` with JSON input on
stdin. Input is validated against the script's declared JSON Schema (Draft 2020)
before execution. Like script agents, a tool's source can be inline `code` or an
optional absolute `.py` `file_path` (file contents override `code`); set it via
the file-path picker on the Python Tool form.

### Tool Name Collision Rules

- Python tool name collides with a builtin → spawn fails
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

---

## Consultants

A consultant is a named api-mode agent that a caller invokes inline via the `consult` builtin tool to get a focused answer without spinning up a full workflow layer.

**Requirements:**
- `execution_mode` must be `api` — enforced by the service layer at create and update time; saving a consultant def with any other mode is rejected.
- The **Consultant** toggle in the agent editor sets the `consultant=true` flag on the definition.

**Context the consultant receives:**
- All caller ticket data, findings, and artifacts (shared `workflow_instance_id`).
- `${CALLER_TRANSCRIPT}` — recent message history of the calling agent (truncated to fit context).
- `${CONSULT_QUESTION}` — the question string passed by the caller to the `consult` tool.

**Required lifecycle:**
1. Do the work using available tools.
2. Write the answer to the `_consult_answer` finding (string value).
3. Call `agent_finished` — this unblocks the caller and delivers the answer inline.

**Constraints:**
- Consultant agents cannot call `consult` themselves (recursion guard enforced in `prepareSpawn`).
- The `_consult` phase is hidden from the v4 read model; the caller's run timeline is uninterrupted.

**Caller usage:** pass `{consultant: "<agent_id>", question: "<text>"}` to the `consult` builtin tool. The call blocks until the consultant finishes and returns the `_consult_answer` value as the tool result.
