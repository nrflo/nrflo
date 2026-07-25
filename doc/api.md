# API Mode (`execution_mode=api`)

For shared concepts (template variables, findings patterns, lifecycle,
workflow config, resilience, examples), see the **Common** tab.

This file covers what is specific to the `api` execution mode: the in-process
provider runner, enabling it, model/provider selection, credentials, tool
registry, multimodal tool results, and behavioral notes.

---

## What It Is

API mode runs agents as an in-process tool-use loop rather than spawning a
CLI process. The concrete provider (Anthropic, OpenAI, or OpenRouter) is selected
**per-agent** from the agent's unified `models` row — the `model` field on the
agent definition is a `models.id`, and api mode requires that row's
`api_model` to be non-empty. The spawner reads `provider`, `api_model`,
`api_context`, and `api_efforts`, then calls
`BuildAPIProvider(provider, projectID)` at spawn time. Each agent turn calls
the provider's streaming API, dispatches tool invocations to registered
handlers, and loops until `end_turn` or a terminal signal.

---

## Enabling

API mode is controlled by the `api_mode_enabled` global setting. Toggle it at
**Settings → Administration**. When off, any spawn request for an api-mode
agent returns an error immediately.

**api-via-cli hybrid** (`api_via_cli_enabled`): when on, Anthropic api-mode
models are routed through the Claude CLI billed on your subscription instead
of direct HTTP calls. The hybrid deliberately retains the row's API model,
context, and effort fields even though it launches a CLI process. The tool
registry is identical; tools are served over the MCP bridge
(`nrflo_server agent mcp`). OpenAI models are unaffected and continue to use
the in-process runner.

---

## Model and Provider Selection

One `models` row represents a provider/model pair and may support CLI mode,
API mode, or both. For an api-mode agent, `model` must name an enabled row
whose `api_model` is non-empty. The row's `provider` selects the backend:

| `provider` | Backend |
|------------|---------|
| `anthropic` | Anthropic Messages API (streaming) |
| `openai` | OpenAI Responses API (streaming) |
| `openrouter` | OpenAI Responses API via OpenRouter (thin wrapper over the openai decoder) |

**Seeded API-capable rows** (read-only):

| provider | id | api_model | api_context | api_efforts |
|----------|----|-----------|-------------|-------------|
| anthropic | `fable-5` | `claude-fable-5` | 1M | low, medium, high, xhigh, max |
| anthropic | `sonnet-5` | `claude-sonnet-5` | 1M | low, medium, high, xhigh, max |
| anthropic | `haiku-4-5` | `claude-haiku-4-5` | 200k | low, medium, high |
| anthropic | `opus-5` | `claude-opus-5` | 1M | low, medium, high, xhigh, max |
| anthropic | `opus-5-1m` | `claude-opus-5[1m]` | 1M | low, medium, high, xhigh, max |
| anthropic | `opus-4-6` | `claude-opus-4-6` | 1M | low, medium, high, max |
| anthropic | `opus-4-6-1m` | `claude-opus-4-6[1m]` | 1M | low, medium, high, max |
| anthropic | `opus-4-7` | `claude-opus-4-7` | 1M | low, medium, high, xhigh, max |
| anthropic | `opus-4-7-1m` | `claude-opus-4-7[1m]` | 1M | low, medium, high, xhigh, max |
| anthropic | `opus-4-8` | `claude-opus-4-8` | 1M | low, medium, high, xhigh, max |
| anthropic | `opus-4-8-1m` | `claude-opus-4-8[1m]` | 1M | low, medium, high, xhigh, max |
| openai | `gpt-5.2` | `gpt-5.2` | 400k | low, medium, high, xhigh |
| openai | `gpt-5.3-codex` | `gpt-5.3-codex` | 400k | low, medium, high, xhigh |
| openai | `gpt-5.4` | `gpt-5.4` | 1.05M | low, medium, high, xhigh |
| openai | `gpt-5.4-mini` | `gpt-5.4-mini` | 400k | low, medium, high, xhigh |
| openai | `gpt-5.5` | `gpt-5.5` | 1.05M | low, medium, high, xhigh |
| openai | `gpt-5.6-sol` | `gpt-5.6-sol` | 1.05M | low, medium, high, xhigh, max |
| openai | `gpt-5.6-terra` | `gpt-5.6-terra` | 1.05M | low, medium, high, xhigh, max |
| openai | `gpt-5.6-luna` | `gpt-5.6-luna` | 1.05M | low, medium, high, xhigh, max |

`reasoning_effort` on an agent definition is an optional per-agent override
validated against `api_efforts`; when omitted, `default_effort` from the model
row is used. (`ultra` is a Codex-CLI-only effort and therefore never appears
in `api_efforts`.) Custom rows can be managed under **Settings → Models** or
through the global model routes:

- `GET|POST /api/v1/models`
- `GET|PATCH|DELETE /api/v1/models/{id}`
- `POST /api/v1/models/{id}/test` — CLI-mode probe; API-only rows are rejected

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
| `delegate` | Spawn tier-resolved worker(s) downward (async-with-poll). Input: `{tier: "extractor"\|"executor", brief, context?, artifacts?, wait_sec?, fanout?}` |
| `get_delegation` | Poll a delegation started via `delegate`. Input: `{delegation_id, wait_sec?}` |
| `run_subworkflow` | Start a callable workflow as a detached sub-workflow; returns `{instance_id, status}`. Input: `{workflow, instructions, result_key?, wait_sec?}` |
| `get_subworkflow` | Poll a sub-workflow; terminal statuses include the result finding/failure reason, plan-boundary statuses include `{plan, revision, questions}`. Input: `{instance_id, result_key?, wait_sec?}` |
| `dynamic_workflow` | Start the bundled plan-driven `dynamic` workflow as a sub-workflow; a planner drafts a manifest from `instructions`. Input: `{instructions, mode?: "approve"\|"auto", wait_sec?}` |
| `revise_plan` | Revise a sub-workflow's plan (edited manifest, or planner feedback/answers). Input: `{instance_id, revision, plan?, feedback?, answers?}` |
| `approve_plan` | Approve+materialize a sub-workflow's plan at a revision. Input: `{instance_id, revision}` |

**Native fs tools** (workdir-jailed, offered only when `native_tools=none` on a claude def — bridge — or the `api_native_tools_enabled` global setting is on — in-process/console; see `tools_builtin.FSTools()`):

| Tool | Description |
|------|-------------|
| `read_file` | Read a file (line-numbered `cat -n` text, or an image content block for PNG/JPEG). Input: `{path, offset?, limit?}` |
| `edit_file` | Exact-string replacement on a file already `read_file`'d this session. Input: `{path, old_string, new_string, replace_all?}` |
| `write_file` | Create a file, or overwrite one already `read_file`'d this session. Input: `{path, content}` |
| `glob` | Fast filename pattern matching (`**` supported), mtime-sorted. Input: `{pattern}` |
| `grep` | Regex content search: `files_with_matches`\|`count`\|`content` modes, line numbers + `-A`/`-B`/`-C` context, optional `glob` filter. Input: `{pattern, glob?, output_mode?, -i?, -A?, -B?, -C?}` |
| `bash` | Run `sh -c`; set `run_in_background` for a long-running command. Input: `{command, timeout_ms?, run_in_background?}` |
| `bash_output` | Poll a background shell for new output + status/exit code. Input: `{shell_id, filter?}` |
| `kill_shell` | Kill a background shell. Input: `{shell_id}` |

`bash` runs every command through a server-side script safety gate first (project `tool_safety_script` config key > global `tool_safety_script` > the project's `claude_safety_hook` config > allow) — a check only, not an interactive permission system; a block surfaces as an `isError` tool result, never a turn failure.

**`read_document`** materializes a named input artifact and returns its bytes
as an image or document content block so the model can read it natively (OCR
scanned PDFs, photos). PDF → document block; PNG/JPEG → image block. Capped
at 32 MiB.

**`emit_findings`** validates a finding before storing it. Each workflow definition may register finding schemas (`key` + JSON Schema Draft 2020 + a known-good example) under Settings. The tool looks up the schema for `key`, validates the supplied `value`, and on success stores it as a session finding. On a schema mismatch — or when the key has no schema — the call is rejected (nothing is stored) and the error result includes the validation message plus the example, so the agent can correct the value and call again. Use `findings_add` for free-form findings that have no schema.

**`consult`** synchronously spawns a named consultant agent (agent definition with `consultant=true` and `execution_mode=api`) under the same workflow instance. The caller's recent message transcript and the question are passed as `${CALLER_TRANSCRIPT}` and `${CONSULT_QUESTION}` template variables. The consultant must write a `_consult_answer` finding (string) and then call `agent_finished`. The answer is returned inline to the calling agent; the `_consult` phase is hidden from the read model. Consultant agents cannot call `consult` themselves (recursion guard). WebSocket events: `consult.started`, `consult.answered`, `consult.failed`.

**`run_subworkflow` / `get_subworkflow`** start and poll callable sub-workflows (async-with-poll). Only workflow definitions flagged `callable_as_subworkflow` can be started. `run_subworkflow` returns `{instance_id, status: "running"}` immediately; pass `wait_sec` (max 240) on either tool to block server-side until completion — the caller's stall timer is heartbeated during the wait. Results are read from the child's session finding named `result_key` (default `workflow_final_result`). Guards: nesting capped by `subworkflow_max_depth` (default 3; counts run_subworkflow starts only — next-on-success chains are unaffected; the tool is stripped from agents of runs at the cap), `subworkflow_max_children` concurrent children (default 6), `subworkflow_max_invocations` per run (default 25, persisted — survives pause/retry) — all config-KV overridable per project. Only the run that started a child may poll it; children are stopped when their parent reaches a terminal status (they survive a pause), and sub-runs never fire `next_workflow_on_success`. Disable with `subworkflow_tools_enabled=false`.

**`dynamic_workflow` / `revise_plan` / `approve_plan`** drive an on-demand, plan-driven sub-workflow — the bundled global `dynamic` workflow (fanout_template agents only; a planner drafts its manifest). `dynamic_workflow` starts it as a child sharing `run_subworkflow`'s guards/caps; `mode: "approve"` (default) parks the child at `waiting_approval` for the caller to drive via `revise_plan`/`approve_plan`; `mode: "auto"` auto-approves and materializes without suspending — refused unless the `dynamic_workflow_auto_enabled` config key (project-override, else global; default off) is true. `get_subworkflow` on a child in one of the four plan-boundary statuses (`planning`/`plan_ready`/`waiting_input`/`waiting_approval`) returns `{plan, revision, questions}` instead of a terminal result; `revise_plan`/`approve_plan` are revision-pinned (a stale `revision` errors, naming the current one). Any `callable_as_subworkflow` workflow with no static layers self-drafts at the plan boundary the same way, independent of these tools — `mode`/auto-approve is persisted on the run as `plan_auto_approve`.

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

---

## Delegation

`delegate` spawns one or more downward workers to do execution work a decider/executor agent shouldn't do itself, returning their structured findings — never a transcript.

**Tiers** — `tier` resolves to a fixed system agent definition, not a caller-chosen model:
- `extractor` → `_t2_extractor` (haiku-4-5, low effort, read-only tools). Answers exactly one question; cannot itself call `delegate`.
- `executor` → `_t1_executor` (sonnet-5, medium effort, full tool set). Owns a slice of work end to end and may itself call `delegate` (tier `extractor`) one level further down.

**Inputs:**
- `brief` (required) — what the worker should do; templated identically per fanout item.
- `context` (optional) — inline context, capped at 4KB; larger context belongs in an artifact.
- `artifacts` (optional) — names of artifacts already materialized for this run.
- `wait_sec` (optional, default 0) — block inline up to this many seconds (max 240) for the result; `0` returns immediately with a `delegation_id` to poll via `get_delegation`.
- `fanout` (optional) — spawn one worker per item (same brief, templated once per item) instead of a single worker; capped by `delegate_max_fanout` (default 20, project-override, else global).

**Result:** each worker's structured findings (its `_delegate_findings` finding), aggregated per fanout item — never the worker's transcript. The `_delegate` worker phase is hidden from the v4 read model, same as `_consult`.

**Recursion guard:** `_t2_extractor` never has `delegate` in its tool set. `_t1_executor` keeps it until `delegate_max_depth` (default 2) is reached, tracked per delegation chain (each worker inherits the caller's depth + 1) — a worker spawned past the cap has `delegate` stripped from its registry. A top-level agent and every fresh top-level `delegate` call start a new chain at depth 0.

**Async polling:** `get_delegation` takes `{delegation_id, wait_sec?}` and returns the current aggregated status (`running`/`completed`/`failed`) plus per-worker results; `wait_sec` blocks up to 240s for still-running workers, heartbeated so the caller's stall timer stays quiet.

**Console usage:** `delegate`/`get_delegation` are also available to console (T0) sessions — the one instance-creating tool intentionally exposed there, since a console session has no bound workflow instance for a worker to spawn under.

WebSocket events: `delegate.started`, `delegate.completed`, `delegate.failed`.
