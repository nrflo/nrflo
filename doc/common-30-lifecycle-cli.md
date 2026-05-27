# Agent Lifecycle & Findings Tools

Spawned agents report results via **MCP tools** served by nrflo. The tools appear
in your tool list, namespaced by your CLI — `mcp__nrflo__<tool>` (Claude) or
`nrflo/<tool>` (codex). Call them by their base name (e.g. `agent_finished`).

**Exiting with code 0 is an implicit pass** — the `agent_finished` tool is the
explicit equivalent. Only call `agent_fail` when the task cannot be completed.
Context is provided automatically by the system.

> **Note:** Script-mode agents (`execution_mode=script`) interact with lifecycle
> via the Python SDK instead of these tools. See [python.md](python.md).

## Lifecycle tools

| Tool | Input | When to use |
|------|-------|-------------|
| `agent_finished` | — | Task completed successfully; orchestrator advances to the next phase |
| `agent_fail` | `{reason?}` | Task cannot be completed; `reason` is optional but recommended |
| `agent_continue` | — | Context window exhausted; save progress to the `to_resume` finding first. Triggers a fresh relaunch — not a success signal |
| `agent_callback` | `{level}` \| `{target_agent}` \| `{chain}` | Issue found requiring a re-run; supply exactly one of `level` (whole layer N), `target_agent` (single agent), or `chain` (list of named agents) |
| `workflow_skip` | `{tag}` | Skip a workflow group in subsequent layers; `tag` must be in the workflow's `groups` |
| `chain_next_instructions` | `{instructions}` | Pass instructions to the next chain step; call before `agent_finished` |
| `chain_next_ticket` | `{ticket_id}` | Set the ticket ID for the next ticket-scope chain step; call before `agent_finished` |
| `consult` | `{consultant, question}` | Synchronous expert consult; blocks until the consultant (an api-mode consultant defined in the same workflow) answers and returns the answer as the tool result |

**Completion semantics:** Exit 0 or `agent_finished` = pass. Non-zero exit or
`agent_fail` = fail. `agent_continue` triggers a fresh relaunch for
context-exhausted agents.

---

## Artifact tools

`$NRF_ARTIFACTS_DIR` points to the pre-materialized stage directory for input
artifacts. The `read_document` tool returns an input artifact's absolute path so
you can read it with your native file tools.

| Tool | Input | Purpose |
|------|-------|---------|
| `artifact_add` | `{file/name}` | Upload a file as a named artifact (max 32 MiB) |
| `artifact_list` | — | List all artifacts for this workflow instance |
| `artifact_get` | `{name}` | Materialize an artifact and return its local absolute path |

---

## Findings tools

### Agent-level (own session)

| Tool | Input |
|------|-------|
| `findings_add` | `{key, value}` |
| `findings_add_bulk` | `{key_values: {k: v, …}}` |
| `emit_findings` | `{key, value}` — validate `value` against the workflow's configured schema for `key` before storing; rejected (with the required-structure example) on mismatch or unknown key |
| `findings_append` | `{key, value}` (creates an array if needed) |
| `findings_append_bulk` | `{key_values: {…}}` |
| `findings_get` | `{}` (own) · `{key}` · `{keys: [...]}` · `{agent_type}` (cross-agent read) · `{layer}` (every agent at a layer) |
| `findings_delete` | `{keys: [...]}` |

### Project-level

Same shapes with the `project_findings_` prefix: `project_findings_add`,
`project_findings_add_bulk`, `project_findings_append`,
`project_findings_append_bulk`, `project_findings_get`, `project_findings_delete`.

Cross-agent and cross-layer reads use `agent_type` / `layer` on `findings_get`;
project findings are visible to every agent in the project.
