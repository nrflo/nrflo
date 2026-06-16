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

---

## 3. Detect "agent is waiting for user input" instead of treating it as idle

### Motivation
For `cli_interactive` agents, nrflo cannot distinguish *"the agent is blocked waiting for a human to type something"* from *"the agent is just slow/idle."* Both look identical: no output for N seconds. The only response today is the time-based idle/nudge loop (`idle_nudge.go`) — after `idleStartTimeout`/`idleAfterMessageTimeout` it writes the `finish-reminder` injectable to PTY stdin, and after `nudgeMax` nudges + another idle window it auto-fails the session as `unresponsive_after_nudges`.

That is a blind heuristic. An agent that genuinely paused to ask a question gets a generic "wrap up and call `nrflo agent continue/fail`" prompt shoved at it, and if it keeps waiting it's killed — rather than being surfaced to a human who could answer it. We have signals that would let us detect the wait precisely; we currently throw them away.

### Current state (verified)
- Claude hook set is registered in `BuildInteractiveSettingsJSON` (`hooks_settings.go:43-54`): `PreToolUse, PostToolUse, UserPromptSubmit, Notification, SubagentStop, PreCompact, SessionStart`. The set is deliberately conservative — adding hook keys the installed CLI doesn't recognize made the CLI reject `--settings` on bootstrap and broke prompt delivery (`hooks_settings.go:38-42`).
- **`Stop` IS registered for Claude** (`hooks_settings.go`) and drives end-of-turn completion enforcement (`handler_record_event.go` `handleStopHook`): an autonomous turn ending without a completion tool gets a `decision:block` + finish-reminder, then a fail after the block budget. The block/continue *mechanism* (old section B) shipped; the remaining gap is input-wait **detection** — block-and-remind is blunt and cannot tell "asked a question" from "not done yet".
- **`Notification` IS registered** (`hooks_settings.go:48`) but `handler_record_event.go:80` just records `event["message"]` as a generic `"text"` agent_message. It never branches on it.
- Claude is launched with `--dangerously-skip-permissions` (`cli_adapter_claude.go`), so permission-driven Notifications never fire.

### Design

Two candidate signals, in order of cost/benefit:

**A. Branch on the `Notification` hook (lowest effort, highest signal).**
Claude Code emits `Notification` when (a) it needs tool permission, and (b) the prompt has been idle waiting for input ~60s. With `--dangerously-skip-permissions`, only the idle-prompt notification fires — which is exactly "waiting for input." We already receive it. Change `handler_record_event.go:80` to pattern-match the message; on the input-wait variant:
- mark the session `awaiting_input` (new agent_session status or a flag column),
- broadcast a new `agent.awaiting_input` WS event,
- suppress / reset the blind idle-nudge timer for that session so we don't auto-fail an agent that's correctly waiting,
- optionally dispatch a notification (Slack/Telegram) so a human can take-control and answer.

No new hook registration, no `--settings` bootstrap risk.

**B. Stop-hook block/continue — SHIPPED (completion enforcement).** The `Stop` hook is registered and the server returns `{"decision":"block","reason":...}` to force-continue an autonomous turn that ended without a completion tool, capped at `stopBlockCap` then failed (`handleStopHook`). What this does **not** yet do — and the remaining open work for this item — is distinguish "stopped to ask the user a question" from "stopped because not done": Stop fires on every turn boundary and the payload carries no intent, so an agent that legitimately pauses to ask gets the same blunt finish-reminder. Telling them apart needs transcript inspection (last assistant message: no tool call + a question?) and, on a genuine ask, escalation to a human instead of a block. That escalation is the same surface as option A below.

### Surface area
- `handler_record_event.go` (Notification branch; optional Stop branch + transcript read).
- `hooks_settings.go` (only if adding `Stop`; gate behind CLI-version verification).
- New agent_session status/flag + `agent.awaiting_input` WS event (`ws/`).
- `idle_nudge.go` — skip/extend the nudge timer while a session is `awaiting_input`.
- UI — show an "awaiting input" badge on the run view; tie into the existing take-control / PTY relay so a human can answer.
- Notify (`notify/`) — optional dispatch when a session enters `awaiting_input`.

### Open questions
- **Notification message stability.** The idle-prompt notification text is CLI-version-dependent; matching on it is fragile. Pin to a substring and verify per CLI version, or accept best-effort?
- **Other providers.** This is Claude-specific (hook-driven). Codex/Gemini/OpenCode have no equivalent "waiting for input" event — fall back to the idle heuristic, or derive it from their JSONL/SSE streams? Out of scope initially.
- **Auto-continue vs. escalate.** When `awaiting_input` is detected, default to escalating to a human (notification + take-control), or attempt a Stop-block auto-continue first and only escalate if it recurs?
- **Interaction with `unresponsive_after_nudges`.** Should `awaiting_input` fully disable auto-fail, or just extend the window? A truly stuck agent that emitted one input-wait notification shouldn't live forever.

### Out of scope
- Per-provider input-wait detection for non-Claude CLIs (no native signal).
- Replacing the idle/nudge loop — this augments it (suppress nudges while genuinely awaiting input), not removes it.

---

## 4. Per-cli-model thinking toggle (mirror api-mode `reasoning_effort=''`)

### Motivation
The same `cli_models` row behaves differently across execution modes. In `api` mode, `reasoning_effort=''` maps to a thinking budget of 0 — thinking is fully off (`spawner/apirun/provider/anthropic/translate.go`, `thinkingBudget`). In `cli` mode, `''` just omits the `--effort` flag (`cli_adapter_claude.go:54-56`), leaving Claude Code's default — which on Opus 4.8 (lean prompt + high-effort default) means thinking is **on**. So an operator who sets empty effort to save tokens gets thinking-off via the api lane and thinking-on via the cli lane.

### Design
When `reasoning_effort==''` for a `claude` cli model, set `MAX_THINKING_TOKENS=0` in the spawned agent's env (or pass `--thinking disabled`) so the cli lane mirrors the api lane. Non-empty effort keeps `--effort=<value>` (thinking on at that tier). Requires Claude Code ≥ 2.1.166 (bundled image is 2.1.178; BYO hosts may be older — see item 5).

### Surface area
- `cli_adapter_claude.go` (argv/env construction).
- Possibly a one-line note in `spawner/CLAUDE.md`.

### Open questions
- Interaction of `--effort` and `MAX_THINKING_TOKENS`: confirm CC honors `MAX_THINKING_TOKENS=0` on a default-thinking model when no `--effort` is passed.
- Implicit (empty effort ⇒ off) vs. an explicit per-model toggle column. Lean implicit to avoid a schema change.

---

## 5. Claude CLI version-floor preflight

### Motivation
Several incorporated features depend on a minimum Claude Code version: Stop `additionalContext` (≥2.1.163), `--fallback-model` in interactive sessions (≥2.1.166), thinking-disable (≥2.1.166). A too-old `claude` on a bring-your-own host degrades **silently** — unknown flags ignored, unrecognized hook keys can break `--settings` bootstrap. The Docker image is build-pinned to 2.1.178, so this is the non-Docker case.

### Design
At server startup (or first claude spawn), run `claude --version`, parse the semver, compare against a floor constant, and refuse/warn with a clear message. This is the lightweight alternative to CC's `requiredMinimumVersion` managed setting (which would require nrflo to start writing a managed-settings policy file to a system path — nrflo writes none today; everything is inline `--settings`, and the managed file is redundant in Docker where the version is build-pinned + autoupdater disabled).

### Surface area
- `spawner/` preflight + a floor constant (or global setting).
- Startup log / health surface.

### Open questions
- Hard-fail vs. warn-and-continue.
- Per-provider floors (codex/opencode have independent version contracts).

---

## 6. Relocate Claude CLI temp/session state under `NRFLO_HOME` (`CLAUDE_CODE_TMPDIR`)

### Motivation
Claude Code's own session state / temp may live in ephemeral `/tmp`. In the Docker image a container restart could drop anything CC persists there. nrflo's MCP socket is already under `NRFLO_HOME` (`socket/server.go:21-27`), so this is strictly about CC-owned state, not nrflo's.

### Design
Set `CLAUDE_CODE_TMPDIR` to a dir under `NRFLO_HOME` (e.g. `$NRFLO_HOME/cc-tmp`) in the Docker image and/or the spawn env. `CLAUDE_CODE_TMPDIR` predates the current pin so the bundled CC honors it (CC docs note it is only *partially* respected).

### Precondition (validate before building)
Only worth doing if `--resume` depends on CC's on-disk session transcripts. If nrflo reconstructs resume context itself (the `to_resume` finding / `${PREVIOUS_DATA}` template var) and does not read CC's session store, this is pure hygiene and can be skipped. **Gate the whole item on confirming that dependency.**

### Surface area
- `Dockerfile` ENV, spawn env construction.

---

## 7. Register Fable 5 as a selectable cli model

### Motivation
Claude Code 2.1.170 introduced Fable 5 (`claude-fable-5`). nrflo's `cli_models` registry should offer it for selection and as a fallback-chain entry (pairs with the `--fallback-model` work).

### Design
Seed migration adding a `claude` `cli_models` row mapped to `claude-fable-5`, mirroring the Opus 4.8 seed (`000138_opus_4_8_models.up.sql`). Confirm default `reasoning_effort` and whether the `xhigh`-restricted-to-Opus rule (`service/cli_model.go`) needs to include Fable 5.

### Surface area
- DB seed migration; possibly the UI model list (auto-driven from the table).

### Open questions
- Default reasoning effort and `xhigh` eligibility for Fable 5.

---

## 8. Disable Claude Code bundled skills for nrflo agents (`disableBundledSkills`)

### Motivation
Claude Code 2.1.169 added `disableBundledSkills` / `CLAUDE_CODE_DISABLE_BUNDLED_SKILLS`. nrflo agents run on custom system prompts + MCP tools; CC's bundled skills add context/token overhead and can surface behaviors nrflo doesn't drive.

### Design
Set `CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1` in the spawn env (or `disableBundledSkills` in the inline `--settings`), likely behind a global/per-project toggle. Measure the token/behavior delta before defaulting it on.

### Surface area
- `hooks_settings.go` / spawn env; a settings toggle.

### Open questions
- Default on or off; confirm no nrflo flow relies on a bundled skill (unlikely).
