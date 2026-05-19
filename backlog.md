# nrflo backlog

Candidate features. Each entry is a self-contained brief: motivation, design, surface area, and open questions. Not approved, not scheduled — review and triage.

---

## 1. Post-success "finalize" step that is allowed to fail

### Motivation
Today, once a workflow's last layer completes, the orchestrator either:
- closes the ticket (if `close_ticket_on_complete=true`),
- spawns `NextWorkflowOnSuccess` (if set and `workflow_final_result` non-empty), and
- dispatches notifications.

There is no place to run "do this cleanup/push/deploy/rebase work, but if it fails don't fail the workflow."

Concrete use cases:
- Push the branch + open a draft PR (currently has to be inside the implementor agent, which makes that agent's prompt do double duty).
- Run a deploy script.
- Post a custom Slack message that needs the workflow's findings (the existing notification system fires before any post-step would run).
- Rebase the worktree onto `main` and clean up the worktree.
- Move a config file, archive a plan, ping an external API.

The "allowed to fail" semantics is the key feature — these tasks are housekeeping, not validation, and a failure shouldn't flip the workflow status.

### Design

Add an optional `finalize_agent_id` field on the workflow definition pointing to an agent definition. Or — cheaper — add a `finalize` block directly on the workflow:

```sql
ALTER TABLE workflows
  ADD COLUMN finalize_command TEXT,            -- single shell command, optional
  ADD COLUMN finalize_agent_definition_id INTEGER REFERENCES agent_definitions(id),
  ADD COLUMN finalize_required BOOLEAN NOT NULL DEFAULT 0;
```

Semantics:
1. Runs **after** `markCompleted` succeeds, **before** `maybeStartNextOnSuccess` and notifications. (Order matters: finalize may add a commit; downstream chain steps should see the final tree.)
2. If `finalize_agent_definition_id` is set, spawn it as a Layer-N+1 agent with effective_mode and findings inherited from the workflow. Otherwise run `finalize_command` directly in the worktree.
3. Failure handling:
   - `finalize_required=false` (default): log failure, write a `finalize_failure` row in `errors`, **keep workflow status = completed**. Emit `workflow.finalize_failed` WS event.
   - `finalize_required=true`: failure flips workflow to `failed`. Provided as an escape hatch for users who want push-or-bust.
4. Notifications fire *after* finalize completes (success or fail), so they can include finalize result in payload.

### Why not just chain a workflow?
`next_workflow_on_success` runs as a **new workflow instance** — heavier (new instance row, new agent sessions, separate UI surface). The use cases above are single-step, want to share the worktree, and want the failure to be visible *on the parent run* rather than a child run.

### UI surface
- Workflow editor: optional "Finalize step" section with command OR agent picker, plus a "required" toggle.
- Workflow run view: dedicated finalize panel showing command, exit, output tail.

### Open questions
- Should finalize have access to the workflow's `workflow_final_result` finding via env? Yes — inject as `NRF_WORKFLOW_FINAL_RESULT`.
- Does finalize run for project-scoped workflows? Yes, with no worktree (project root).
- Does finalize run on `retry_failed`? Only if the retry succeeds — same as today's success path.
- Interaction with chain runs: if a chain step has finalize and finalize fails (non-required), does the chain continue? Yes — chain sees the step as completed.

---

## 2. ACP execution mode — uniform adapter for ~14 extra providers

### Motivation
Today nrflo ships a hand-written `CLIAdapter` per vendor (Claude, Codex, OpenCode). Adding a new provider (Gemini, Copilot, Cursor, Qwen, Amp, Auggie, Droid, Kimi, Kiro, Qoder, Trae, iFlow, Pi, Kilocode) means a new adapter file, new prompt-delivery quirks, new stdout parser. The cost per vendor is real and the long tail is large.

The [Agent Client Protocol (ACP)](https://agentclientprotocol.com) is a JSON-RPC 2.0 stdio dialect that most modern coding CLIs now speak — either natively (Copilot `--acp`, Gemini `--acp`, OpenCode `acp`, Cursor `cursor-agent acp`, Qwen, Droid, Kimi, Kiro, Qoder, Trae, iFlow, Pi, Kilocode) or via a thin adapter (`npx -y @agentclientprotocol/claude-agent-acp`, `npx -y @zed-industries/codex-acp`, `npx -y amp-acp`, etc.). The adapter or native mode **still spawns the real CLI underneath** — auto-compact, MCP servers, credentials, model selection all preserved. Reference: [kdlbs/kandev](https://github.com/kdlbs/kandev) ships ~17 providers behind one ACP factory exactly this way (`apps/backend/internal/agent/agents/*_acp.go`).

One ACP adapter in nrflo subsumes the entire long tail.

### Design

Add a fifth peer to `execution_mode`:

```
execution_mode ∈ {cli_interactive, api, script, acp}
                                                    ↑ new
```

Per CLAUDE.md rule #6, the divergence lives in one new file alongside the existing per-vendor adapters:

```
be/internal/spawner/
  cli_adapter_claude.go        ← keep (depth: native stream-json + usage)
  cli_adapter_codex.go         ← keep
  cli_adapter_opencode.go      ← keep
  cli_adapter_acp.go           ← NEW (breadth: uniform JSON-RPC for everything else)
```

The ACP adapter:
1. Spawns the configured launch command per provider profile (e.g., `npx -y @google/gemini-cli --acp`). Provider catalog stored in a new `acp_providers` table or as `cli_models` rows with a `launch_command` column.
2. Speaks ACP: `initialize` → `session/new` → `session/prompt` → consumes `session/update` notifications until `stop_reason`.
3. Maps `session/update` variants to nrflo events:
   - `ContentChunk` (agent_message_chunk / agent_thought_chunk) → agent log lines.
   - `ToolCall` / `ToolCallUpdate` → existing tool-event surface (parity with apirun).
   - `Plan` → optional UI hook (could feed phase status).
   - `CurrentModeUpdate` → discard or expose.
   - `session/request_permission` → auto-approve by default; future: surface to UI for HITL.
4. Carries nrflo agent identity into the child via env (`NRF_SESSION_ID`, `NRF_WORKFLOW_INSTANCE_ID`, `NRFLO_AGENT_TOKEN`, `ProjectEnv`) — same envelope as today's adapters. The ACP child inherits and the real CLI underneath sees it, so `nrflo agent findings`, `agent.finished`, `skip`, etc. all keep working.

Everything **above** `execution_mode` is unchanged: layer execution, pass policies, callbacks, findings, chains, low-context relaunch, stall detection, restart cap. Those sit on the orchestrator and don't care which lane an agent picks.

### Hybrid model (multiple lanes coexisting)

These are real and useful — and follow naturally from `execution_mode` being per-agent:

1. **Per workflow.** A layered workflow can mix lanes: L0 setup-analyzer on `acp` (Gemini), L1 implementor on `cli_interactive` (Claude native, for usage capture + PTY), L2 qa-verifier on `cli_interactive` (human review), L3 doc-updater on `api`.
2. **Per provider.** Keep `cli_interactive` for Claude/Codex/OpenCode (depth path — stream-json, usage, cost); use `acp` only for providers without a native nrflo adapter.
3. **Per session — mode swap on take-control.** Start an agent in `acp`; when user clicks take-control, kill the ACP adapter and re-spawn the same vendor CLI in `cli_interactive` (PTY) with the CLI's native `--resume <session>` flag. Functionally gives users "ACP by default, PTY when needed." Session boundary, not co-existence.

What you genuinely **cannot** do (single-process stdio constraint):
- Run the `cli_interactive` adapter **and** ACP on the same process. Single stdio owner.
- Attach a human PTY **and** ACP to the same vendor CLI. The adapter sits between human and CLI — no terminal to attach.
- Drive `cli_interactive`'s idle/nudge loop from ACP "for free." You'd redefine idle as "no `session/update` for N seconds" and any nudge becomes a synthetic `session/prompt`, not a keypress. Doable but distinct logic.

### What ACP covers vs what it doesn't

**ACP gives you cheaply (one adapter, ~14 providers):**
- Streaming `agent_message_chunk` / `tool_call` / `tool_call_update` uniformly.
- Permission-gating surface (`session/request_permission`) if HITL-approve-in-UI is ever wanted.
- `Plan` updates (phase UI hook).
- Optional `session/load` for vendor-side resume.
- File ops (`fs/read_text_file`, `fs/write_text_file`) and terminal ops (`terminal/create|output|release|wait_for_exit|kill`) — if we want to back them with nrflo logic.

**ACP does NOT carry — has to live above the protocol (already does in nrflo):**
- Token usage / context size / context-window remaining. ACP's `session/update` schema has no usage field. Per-message token counts are blind in the `acp` lane unless the underlying CLI writes them elsewhere. **This is the main reason to keep native `cli_interactive` adapters for Claude/Codex/OpenCode** — Claude exposes stream-json with usage; Codex uses the rollout JSONL tail; OpenCode writes `tokens.{input,output,reasoning,cache.read}` to its SQLite DB (`$XDG_DATA_HOME/opencode/opencode.db`) which the sqlite tail reads. The ACP lane is the breadth lane, not the depth lane.
- Context exhaustion signal / compaction events. No equivalent. `to_resume` finding + `${PREVIOUS_DATA}` template var stay nrflo-owned.
- Workflow concepts: findings, callbacks, layer fan-in, pass policy, chains, next_workflow_on_success, endless loop, stall detection, restart cap, low-context relaunch. All orchestrator-level; unaffected.
- Cost / pricing.

### API & UI surface
- New `cli_models` rows (or new `acp_providers` table) with `launch_command`, optional `--model` template, `auth_env` (e.g. `GEMINI_API_KEY`), display logo. Seeded list mirrors kandev's catalog.
- Agent-definition editor: when `execution_mode='acp'`, model picker is sourced from the chosen provider's catalog row.
- Logs: surface ACP `session/update` stream in the existing agent session log; tool events go through the same path as apirun.
- No new WS event types — map onto existing `agent.*` events.

### Open questions
- **Per-message usage in ACP lane.** Accept the blind spot (document it), or wrap each adapter's stderr and grep for usage lines (fragile, per-vendor)? Default: accept it; nudge users to `cli_interactive` mode when they need cost telemetry.
- **Auto-approve vs UI-approve for `session/request_permission`.** Auto-approve matches kandev's default and current nrflo behavior. UI-approve is a future option; gate behind a per-agent flag.
- **Provider catalog management.** Hard-coded Go seed (kandev's approach), `cli_models` rows (extensible, fits existing surface), or a new admin-CRUD table? Lean toward `cli_models` extension to avoid a new table.
- **`fs/*` and `terminal/*` client methods.** Implement nrflo-side, or refuse (let the agent fall back to shell)? Refuse initially; implement only if a provider misbehaves without them.
- **Take-control swap.** Does the adapter-spawned child expose its underlying CLI's session id well enough to resume in PTY? Vendor-specific — verify per provider before promising the UX.
- **Manifest tools / api-mode parity.** ACP tools are agent-side and named by the CLI vendor; manifest tools (principle 40) are nrflo-side and api-mode only. Keep these orthogonal — don't try to surface manifest tools through ACP.
- **Stall detection.** Redefine "stalled" as `time.Since(lastUpdate) > N` where `lastUpdate` is the last `session/update`. Simpler than today's stdout-silence heuristic.

### Out of scope
- Replacing native `cli_interactive` adapters for Claude/Codex/OpenCode. ACP is additive, not a replacement.
- ACP for `cli_interactive`. PTY users want a real terminal; ACP has no terminal.
- ACP for `api` mode. In-process Anthropic Messages is orthogonal.
