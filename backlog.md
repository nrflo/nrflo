# nrflo backlog

Candidate features. Each entry is a self-contained brief: motivation, design, surface area, and open questions. Not approved, not scheduled — review and triage.

---

## 1. Declarative validation commands per layer / agent

### Motivation
Today, "did this agent succeed?" is decided inside the agent prompt: the agent runs `make test`, decides if it passed, and reports pass/fail via `agent finished` / `agent fail`. This couples gate logic to agent reasoning. Two consequences:

- The same check (e.g., `make test`) gets re-described in every agent prompt that wants it.
- The orchestrator has no first-class view of "what shell commands constitute pass for this step" — it can only read the agent's verdict.

Better: each agent/layer declares a list of shell commands, and the orchestrator runs them after the implementing agent finishes. Exit-0 across the board = pass; any non-zero feeds the failing command's output back as retry context.

The wins:

- The gate is **declarative**, inspectable in the workflow definition, and reusable across agents.
- Agents stop having to lie/be-honest about test results — the orchestrator runs them.
- Findings have a **canonical source** (stdout/stderr of the failing command) rather than an agent's summary.

### Design

Add an optional `validation_commands` field on either:
- **per-agent-definition** (preferred — same agent in different workflows can have different gates), or
- **per-layer-policy** (`workflow_layer_policies` row gets a `validation_commands` column).

Schema sketch (per agent):
```sql
ALTER TABLE agent_definitions
  ADD COLUMN validation_commands TEXT;  -- JSON array of strings
```

Each command is a single shell string executed in the workflow's working directory (ticket-scoped: worktree; project-scoped: project root) with the same env that the agent saw (`ProjectEnv` + nrflo-controlled vars).

Execution order, after the agent reports `finished`:
1. Spawner / orchestrator runs each command sequentially, capturing combined stdout+stderr.
2. First non-zero exit → write `validation_failure` finding with `{command, exit_code, output_tail}` and transition the session to `failed` (counts against agent retry budget).
3. All zero → session stays `completed`.

If `validation_commands` is empty/null, behavior is unchanged (agent verdict is final).

### API & UI surface
- Workflow editor: per-agent textarea for newline-separated commands.
- API: extend `POST /api/v1/agent-definitions` and the update endpoint.
- Logs: surface the validation run output as part of the agent session log (new section, not interleaved with agent transcript).

### Open questions
- Should validation run on `cli_interactive` sessions? Probably yes, but only after the user signals "I'm done" (exits interactive).
- Should validation run on `api` and `script` modes? Yes — uniform across `effective_mode`.
- Per-command timeout? Default 5 min, override via `validation_command_timeout_sec` column.
- Should validation failure be retryable distinct from agent failure (different counters)? Likely yes — a flaky test should not consume the agent's restart budget the same way an actual failure does. Punt on this until we see real data.

---

## 2. Rate-limit pattern detection + auto-wait

### Motivation
Today, when a CLI agent hits a provider rate limit (Anthropic 429, OpenAI quota, Codex limit), nrflo treats the resulting non-zero exit as an agent failure. The session counts against the restart cap; if limits persist, the workflow eventually fails with no useful signal.

Better: separate **transient rate limits** from **real failures** via pattern matching on output. On a limit match, pause for a configurable backoff and retry the **same** session **without** consuming the retry budget.

Long-running workflows that span an hourly quota currently burn through `restart cap` for nothing — this fixes that.

### Design

Add a small, per-CLI-adapter classifier:

```go
// In CLIAdapter interface
type RetryClass int
const (
    RetryClassNone     RetryClass = iota
    RetryClassRateLimit
    RetryClassError
)

ClassifyExit(recentText, stderrTail string, exitCode int) RetryClass
```

Each adapter (`cli_adapter_claude.go`, `cli_adapter_codex.go`, `cli_adapter_opencode.go`, `apirun`'s in-process equivalent) owns its own pattern list. Anthropic API errors in api-mode get classified at the SDK error level (typed errors), not via stdout regex.

**Pattern scan must be windowed, not full-output.** Keep a ring buffer of the last ~10 text blocks per session and only match against the joined recent window. Matching against full output trips false positives when the agent earlier echoed text like "rate limit" while quoting code or docs. Maintain `recentBlocks` per session, scan only the joined tail.

**Two pattern lists, with priority.** Split into `*_limit_patterns` (wait+retry) and `*_error_patterns` (graceful exit). Same string can appear in both: e.g. `"You've hit your org's monthly usage limit"` lives in both lists, and the limit check runs **first**, so when `rate_limit_enabled=true` it wins; when disabled the string still trips graceful exit instead of generic agent failure. This single mechanism handles both modes cleanly.

**Matching is case-insensitive substring.** Not regex initially — vendors reword these messages frequently and regex tempts over-clever patterns that miss the next variant. Substring is more forgiving and cheaper to debug.

When the spawner sees `RetryClassRateLimit`:
1. Persist the limit hit on the `agent_sessions` row (new column `last_retry_class TEXT` or use an enum).
2. Sleep for a configurable backoff (start 60s, exponential up to `rate_limit_max_wait`, default 1h). Sleep is via `clock.Clock` so tests can fast-forward.
3. Re-spawn the **same** session with the same prompt and finding context. **Do not** decrement the restart counter.
4. Emit a WS event `agent.rate_limited` with `{session_id, wait_seconds, matched_pattern}` so the UI can show "waiting for quota… 47m" instead of "failed."

If `rate_limit_max_wait` is exceeded across consecutive limit hits → fall through to normal failure path (so a permanently-broken account doesn't hang forever).

### Configuration

Per-project settings (new `project_settings` row or `project_env_vars`-adjacent):
- `rate_limit_max_wait_sec` (default 3600)
- `rate_limit_initial_backoff_sec` (default 60)
- `rate_limit_enabled` (default true)

**Pattern lists are user-overridable.** Adapter defaults shipped in code, but exposed in project settings as comma-separated overrides:
- `claude_limit_patterns`, `claude_error_patterns`
- `codex_limit_patterns`, `codex_error_patterns`
- `opencode_limit_patterns`, `opencode_error_patterns`

Rationale: these strings are user-facing English produced by third-party CLIs. When Anthropic/OpenAI rewords (e.g., `"You've hit your limit"` → `"Usage limit reached"`), users shouldn't need to wait for an nrflo release to keep working — they patch the pattern themselves. We ship reasonable defaults; they own the override.

Seed defaults (validated against current CLI output as of 2026-05):
- claude limit: `You've hit your limit`, `Your usage allocation has been disabled by your admin`, `You've hit your org's monthly usage limit`
- claude error: same three, plus `API Error:`, `cannot be launched inside another Claude Code session`, `Not logged in`
- codex limit: `Rate limit exceeded`, `rate limit reached`, `429 Too Many Requests`, `quota exceeded`, `insufficient_quota`, `You've hit your usage limit`

### UI surface
- Live session view shows "Rate-limited, retrying in X" badge.
- `agent_session_logs` includes rate-limit events as a distinct row type so they're filterable.
- Live agent sessions endpoint already surfaces status — extend with `rate_limit_until_ts`.

### Open questions
- Should rate-limit waits respect `endless_loop` cancellation and graceful shutdown? Yes — `shutdownCleanup` must wake sleeping waiters.
- Does the wait survive a server restart? Initially no (sleep is in-memory). If we need it durable, persist `rate_limit_until_ts` on the session and resume on startup. Defer.
- Does the budget for `rate_limit_max_wait` reset on success? Yes — track consecutive limit hits only.
- Ring-buffer size: start at 10 text blocks. For `script` and `cli_interactive` modes, "block" is less well-defined (no JSON event stream). Likely: last 10 stdout lines + last 10 stderr lines, concatenated. Validate during impl.
- `api`-mode (apirun) classification: skip pattern matching entirely — Anthropic SDK surfaces `RateLimitError` / `OverloadedError` as typed errors. Map those directly to `RetryClassRateLimit` without going near string scans. Patterns only exist for the CLI-shell-out modes.

---

## 3. Post-success "finalize" step that is allowed to fail

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

## 4. Spec adoption: import external spec formats into a workflow

### Motivation
Today, work enters nrflo as a ticket created in the UI or via the API. The ticket has a title and description; everything else is built up by the L0 setup-analyzer agent at run time.

A lot of upstream context already lives in structured formats elsewhere:
- GitHub Issues (with labels, linked PRs, comments).
- OpenSpec / spec-kit Markdown specs.
- Linear / Jira tickets.
- Plain feature plan documents in repos (`docs/plans/foo.md`).

We want a one-shot import path that turns an external artifact into a populated ticket + pre-loaded findings + chosen workflow, ready to spawn. The user clicks "import," picks a source, and gets a ticket with the right scope and instructions wired up.

### Design

Two layers:

**Layer A — Import adapters.** A small interface:
```go
type SpecImporter interface {
    Name() string                // "github_issue", "openspec", "spec_kit", "markdown"
    CanHandle(input ImportInput) bool
    Import(ctx context.Context, input ImportInput) (ImportedSpec, error)
}

type ImportInput struct {
    URL          string   // for issue URLs
    Body         string   // for raw paste
    FilePath     string   // for repo-relative paths
    ProjectID    int64
}

type ImportedSpec struct {
    TicketTitle       string
    TicketDescription string
    Instructions      string
    WorkflowName      string                 // suggested
    Findings          map[string]string      // pre-seed `workflow_instances.findings`
    AttachedRefs      []string               // URLs preserved as ticket metadata
}
```

Initial adapters:
- `github_issue` — `gh api` fetch by issue URL, parse body + labels.
- `markdown` — paste-in or file path; treats H2 sections as well-known fields if recognized.
- `openspec` / `spec_kit` — recognize the format from headers, map sections to ticket fields.

The **normalization step itself uses an agent** (system agent, api-mode, low-context) — an LLM does the messy parsing rather than us writing a brittle parser per format.

**Layer B — UI + API.**
- `POST /api/v1/import/spec` with `{source, url|body|file, project_id}` returns an `ImportedSpec` preview.
- UI shows the preview, lets the user edit any field, pick the workflow, then "Create ticket" → standard ticket creation + workflow spawn.
- For project-scoped imports, skip ticket creation; pass `Instructions` directly to a project-scoped workflow start.

### Surface to ship first
- One adapter: `github_issue` (highest ROI, common entry point).
- One UI route: "Import from GitHub issue" on the tickets page.
- The agent-based normalization is the same `apirun` path used elsewhere — no new infra.

Other adapters land behind the same interface as users ask.

### Open questions
- Where do the imported `AttachedRefs` live on the ticket? New `ticket_metadata` table or just stuff them into the description? Probably a `ticket_refs` table — also useful for linking back from PRs created by workflows.
- Authentication for `github_issue` — does the server use a project-level GitHub token (new setting) or the user's? Project-level token in `project_env_vars` (read-only).
- Do we want a "watch this issue and re-import on update" mode? No — out of scope. One-shot only.
- Is the normalizing agent a built-in system agent or a user-editable agent definition? Built-in system agent — users shouldn't have to author this to get import working, but they can override via the agent_definitions table by registering one with the canonical id.

---

## 5. Codex context-left tracking in `cli` (batch) mode

### Motivation
`cli_interactive` mode already tracks codex context-left and surfaces it in the UI / agent session row. In `cli` (batch) mode the same signal is missing — sessions show no remaining-context indicator, and the spawner can't make low-context relaunch decisions for codex batch runs the way it can for Claude.

Claude tracks this uniformly across cli + cli_interactive; codex only does cli_interactive. Close the gap so batch codex runs surface the same telemetry and can participate in low-context relaunch.

### Design
- Locate where codex cli_interactive extracts context-left today (likely `be/internal/socket/handler_codex_context.go` and the codex JSONL event extractor).
- Add the equivalent extractor on the batch `cli` output path in `be/internal/spawner/cli_adapter_codex.go` (or wherever codex stdout is parsed in batch mode).
- Persist via the same `agent_sessions` context-usage columns Claude already writes.
- Hook into low-context relaunch logic so codex/cli sessions become eligible (same threshold/policy as Claude).

### Open questions
- Does codex batch (`--json`?) emit the same `token_count` / context event that interactive does? Verify before designing — if batch output omits it, this is dead in the water and we should mark codex/cli as "no context telemetry" instead.
- Should low-context relaunch be gated per-CLI or uniform once telemetry lands?

---

## 6. ACP execution mode — uniform adapter for ~14 extra providers

### Motivation
Today nrflo ships a hand-written `CLIAdapter` per vendor (Claude, Codex, OpenCode). Adding a new provider (Gemini, Copilot, Cursor, Qwen, Amp, Auggie, Droid, Kimi, Kiro, Qoder, Trae, iFlow, Pi, Kilocode) means a new adapter file, new prompt-delivery quirks, new stdout parser. The cost per vendor is real and the long tail is large.

The [Agent Client Protocol (ACP)](https://agentclientprotocol.com) is a JSON-RPC 2.0 stdio dialect that most modern coding CLIs now speak — either natively (Copilot `--acp`, Gemini `--acp`, OpenCode `acp`, Cursor `cursor-agent acp`, Qwen, Droid, Kimi, Kiro, Qoder, Trae, iFlow, Pi, Kilocode) or via a thin adapter (`npx -y @agentclientprotocol/claude-agent-acp`, `npx -y @zed-industries/codex-acp`, `npx -y amp-acp`, etc.). The adapter or native mode **still spawns the real CLI underneath** — auto-compact, MCP servers, credentials, model selection all preserved. Reference: [kdlbs/kandev](https://github.com/kdlbs/kandev) ships ~17 providers behind one ACP factory exactly this way (`apps/backend/internal/agent/agents/*_acp.go`).

One ACP adapter in nrflo subsumes the entire long tail.

### Design

Add a fifth peer to `execution_mode`:

```
execution_mode ∈ {cli, cli_interactive, api, script, acp}
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

1. **Per workflow.** A layered workflow can mix lanes: L0 setup-analyzer on `acp` (Gemini), L1 implementor on `cli` (Claude native, for usage capture), L2 qa-verifier on `cli_interactive` (human review), L3 doc-updater on `api`.
2. **Per provider.** Keep `cli` for Claude/Codex/OpenCode (depth path — stream-json, usage, cost); use `acp` only for providers without a native nrflo adapter.
3. **Per session — mode swap on take-control.** Start an agent in `acp`; when user clicks take-control, kill the ACP adapter and re-spawn the same vendor CLI in `cli_interactive` (PTY) with the CLI's native `--resume <session>` flag. Functionally gives users "ACP by default, PTY when needed." Session boundary, not co-existence.

What you genuinely **cannot** do (single-process stdio constraint):
- Run `cli` parser **and** ACP on the same process. Single stdio owner.
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
- Token usage / context size / context-window remaining. ACP's `session/update` schema has no usage field. Per-message token counts are blind in the `acp` lane unless the underlying CLI prints them to stderr/log. **This is the main reason to keep native `cli` adapters for Claude/Codex/OpenCode** — they expose stream-json with usage; the ACP lane is the breadth lane, not the depth lane.
- Context exhaustion signal / compaction events. No equivalent. `to_resume` finding + `${PREVIOUS_DATA}` template var stay nrflo-owned.
- Workflow concepts: findings, callbacks, layer fan-in, pass policy, chains, next_workflow_on_success, endless loop, stall detection, restart cap, low-context relaunch. All orchestrator-level; unaffected.
- Cost / pricing.

### API & UI surface
- New `cli_models` rows (or new `acp_providers` table) with `launch_command`, optional `--model` template, `auth_env` (e.g. `GEMINI_API_KEY`), display logo. Seeded list mirrors kandev's catalog.
- Agent-definition editor: when `execution_mode='acp'`, model picker is sourced from the chosen provider's catalog row.
- Logs: surface ACP `session/update` stream in the existing agent session log; tool events go through the same path as apirun.
- No new WS event types — map onto existing `agent.*` events.

### Open questions
- **Per-message usage in ACP lane.** Accept the blind spot (document it), or wrap each adapter's stderr and grep for usage lines (fragile, per-vendor)? Default: accept it; nudge users to native `cli` mode when they need cost telemetry.
- **Auto-approve vs UI-approve for `session/request_permission`.** Auto-approve matches kandev's default and current nrflo behavior. UI-approve is a future option; gate behind a per-agent flag.
- **Provider catalog management.** Hard-coded Go seed (kandev's approach), `cli_models` rows (extensible, fits existing surface), or a new admin-CRUD table? Lean toward `cli_models` extension to avoid a new table.
- **`fs/*` and `terminal/*` client methods.** Implement nrflo-side, or refuse (let the agent fall back to shell)? Refuse initially; implement only if a provider misbehaves without them.
- **Take-control swap.** Does the adapter-spawned child expose its underlying CLI's session id well enough to resume in PTY? Vendor-specific — verify per provider before promising the UX.
- **Manifest tools / api-mode parity.** ACP tools are agent-side and named by the CLI vendor; manifest tools (principle 40) are nrflo-side and api-mode only. Keep these orthogonal — don't try to surface manifest tools through ACP.
- **Stall detection.** Redefine "stalled" as `time.Since(lastUpdate) > N` where `lastUpdate` is the last `session/update`. Simpler than today's stdout-silence heuristic.

### Out of scope
- Replacing native `cli` adapters for Claude/Codex/OpenCode. ACP is additive, not a replacement.
- ACP for `cli_interactive`. PTY users want a real terminal; ACP has no terminal.
- ACP for `api` mode. In-process Anthropic Messages is orthogonal.

---

## Manual-testing scenarios — Tier 3 (specialized backends + side channels)

Candidate scenarios for `manual_testing/`. Each one needs extra plumbing beyond what the Tier 1+2 harness provides. Add only when the underlying feature ships or breaks.

- **Script-mode agent (`execution_mode='script'`)** — create a `python_scripts` row, point an agent definition at it, run the workflow, assert the script-spawn path (`scriptBackend`) is taken and findings written via the embedded Python SDK land in `agent_sessions.findings`. Validates the per-project venv path resolution (`be/internal/venv/`) and the `script.context` socket method.
- **API-mode agent (`execution_mode='api'`)** — needs the server booted with `--mode=api`, plus `api_credentials` and `tool_definitions` populated. Run a tool-using prompt end-to-end through `apirun.Runner`. Probably worth a separate `manual_testing/test_api_mode.py` so it can be skipped on CLI-only hosts.
- **Notification channels (Slack/Telegram)** — spawn a tiny in-harness Python `http.server` that records inbound POSTs, create a channel with that URL as the webhook target, run any workflow that triggers `orchestration.completed`, assert the payload was POSTed within N seconds and matches the rendered template. Also covers the retry/backoff queue when the mock returns 500.
- **Scheduled tasks (cron)** — create a `scheduled_tasks` row with a `* * * * *` (every minute) cron, sleep ~70s, assert at least one `schedule_runs` row exists with `status='triggered'` and a matching `workflow_instances` row. Slow — gate behind a `--slow` flag.
- **Take-control / resume-session / exit-interactive** — kick off a workflow, `POST .../take-control`, assert session status flips to `user_interactive`, then `POST .../exit-interactive`. Cannot easily drive the PTY stream from REST; this is a partial end-to-end test.
- **Plan-before-execute (`plan_mode=true`)** — start a run with plan mode, assert response status `planning` and a `session_id`. Validating the actual plan file requires a PTY client.
- **Multi-instance same ticket** — run the same `ticket+workflow` twice, assert two `workflow_instances` rows exist (no UNIQUE constraint enforced after mig 000040). Low-signal but cheap.
- **Custom `cli_models` row** — `POST /api/v1/cli-models`, use the new ID in an agent definition, assert the spawner resolves it to the right CLI binary via `cliForModel`.
- **Workflow chain `require_ticket_handoff`** — chain with a ticket-scope step downstream of a project-scope step; agent uses `chain-next-ticket` to set the ticket; assert the downstream step ran against that ticket.

---

## Manual-testing scenarios — Tier 4 (skip — covered well by Go tests)

These were considered for the harness and rejected. Recorded so future contributors don't re-litigate.

- Authentication / SCS session lifecycle / login rate limiter (`be/internal/auth`, `be/internal/api/handlers_auth.go`, `auth_ratelimit.go`).
- Role-based access (admin vs viewer) — covered by `requireAuth` / `requireAdmin` tests.
- DB migration application — runs at every server boot already; a regression would surface as the harness server failing to start.
- REST pagination shape / list endpoint envelopes — exercised by handler unit tests.
- Field-level validation on REST request bodies (regex, length caps, reserved-name checks).
- Per-handler error code mapping (404 vs 409 vs 400).
- `agent_messages` cursor / WS replay — covered in `be/internal/ws/replay.go` tests.

If any of these regresses in production, prefer adding/extending a Go test over thickening the manual harness.

---

## Backend issues surfaced by the manual-testing harness (2026-05-12)

The first full provider × mode validation of `manual_testing/` (24 scenarios × 6 combos = claude/codex/opencode × cli/cli-interactive) surfaced three real backend issues plus one test-side issue. Recorded here verbatim so they get triaged independently of the harness PR.

### Codex `cli_interactive` hooks never fire — upstream codex regression (openai/codex#21639)

**Status:** confirmed upstream bug. **Hold all nrflo-side changes.**
Codex CLI versions ≥ `0.129.0-alpha.15` ship with a regression that
breaks hook delivery in interactive sessions regardless of how hooks
are declared (`-c hooks.X=…`, `[[hooks.X]]` blocks in
`config.toml`, project-local or user-level `hooks.json`). Tracked at
[openai/codex#21639](https://github.com/openai/codex/issues/21639);
last known working codex = `0.128.0-alpha.1`. Local repro on `0.130.0`,
2026-05-12.

Manifests in nrflo as: codex/cli_interactive sessions complete
successfully (`result='pass'`, agent calls `nrflo agent finished` via
HTTP) but `agent_messages` rows = 0 for the entire session. The PTY
ferry sees codex's banner ("first PTY bytes received" in server log)
then no hook event ever fires, so the spawner has zero
visibility into the model's tool calls or text output. Workflow
runtime is unaffected; only telemetry/observability is lost.

#### Workaround until upstream ships a fix

Downgrade codex on the host running `nrflo_server`:

```
brew install codex@0.128 # or equivalent
```

**Workaround live at:** `be/internal/spawner/backend_interactive_tui_capture.go` — raw PTY bytes are captured, ANSI-stripped, line-buffered, and emitted as `agent_messages` rows when `(*CodexAdapter).CapturesTUIBytes()` returns `true`. Flip that method to `false` once upstream ships a fix, then after one release delete the file + the `tuiLineBuf` field + the `captureTUI` ferryPTYOutput param + the `CapturesTUIBytes` CLIAdapter method.

#### `effective_mode='cli_interactive'` selection — single code path

While diagnosing this we verified the selection mechanism. Recorded
here so the next investigation doesn't re-litigate it.

There is exactly one branch in `be/internal/spawner/spawner.go:1274`:

```go
if s.config.InteractiveCLIMode && prep.adapter != nil && prep.adapter.SupportsInteractive() {
    backend = newCLIInteractiveBackend(...); effectiveMode = "cli_interactive"
}
```

Gates: (a) `executionMode != "script"/"api"`, (b)
`s.config.InteractiveCLIMode == true` — read once at workflow start
from `project_config["interactive_cli_mode"]` (orchestrator.go:351,
single write site `handlers_projects.go:244`), (c)
`adapter.SupportsInteractive()` — true for claude/codex/opencode. No
per-agent-def override; no per-run override. The per-run
`interactive=true` body flag triggers a separate `user_interactive`
pre-step (`orchestrator.go:416`), orthogonal to `effective_mode`
selection.

_Superseded by per-agent execution_mode=cli_interactive (migration 000101)._

`agent_sessions.effective_mode` is written at spawn time
(`repo/agent_session.go:146`) and frozen — toggling
`interactive_cli_mode` off after a successful run does NOT rewrite
historical session rows. (We chased a phantom "third trigger" earlier
because a working UI session had `effective_mode='cli_interactive'`
while the project config currently showed empty; that was just config
drift, not a hidden code path.)

_Superseded by per-agent execution_mode=cli_interactive (migration 000101)._

#### Empirical hook-firing test results (2026-05-12, codex 0.130.0)

Isolated Python PTY harness — `pty.fork`, winsize 24×80, DSR/DA/kitty/
OSC terminal-query auto-replies (matching nrflo's
`ferryPTYOutput(respondToQueries=true)`):

| Hook declaration | PTY bytes | Hooks fired (of 5) |
|---|---|---|
| Inline `-c hooks.X=[…]`, bare PTY (no responder) | 84 | 0 |
| Inline `-c hooks.X=[…]`, full responder + winsize | 1119 | 0 |
| `[[hooks.X]]` array-of-tables in `CODEX_HOME/config.toml`, `[features] hooks=true` | 1119 | 0 |
| `[[hooks.X]]` blocks, `[features] codex_hooks=true` (docs canonical) | 1119 | 0 |
| `hooks.X = […]` inline-assignment in `config.toml`, `[features] hooks=true` | 397 | 0 |

The responder more than doubles byte volume → PTY plumbing itself is
fine. No declaration mechanism causes a single hook event to fire,
including `SessionStart` (which should fire immediately on TUI init).

Stale-but-non-causal: `codex features list` reports the feature is
now named `hooks` (default true), but official docs at
https://developers.openai.com/codex/hooks still document `codex_hooks`
as the required key. We keep `codex_hooks` in
`cli_adapter_codex_hooks.go` to match docs. The feature toggle is
irrelevant on 0.129+ because the underlying regression nukes hook
delivery regardless.

#### When upstream ships the fix

Re-run `/tmp/codex_pty_full.py` (kept locally, not checked in)
against the patched codex. If it shows non-zero hooks fired, two
follow-ups become worthwhile (independent of the upstream fix):

1. Move hook declaration from inline `-c hooks.X=…` flags
   (undocumented, brittle) into `[[hooks.X]]` blocks in the per-session
   `config.toml` written by `writeCodexProfileForSession`. Drop the
   `-c` flags from `BuildInteractiveCommand`. This matches the
   documented schema and survives future upstream changes better.
2. Decide whether to drop the `[features] codex_hooks = true` block
   entirely if/when codex officially makes hooks default-on. Right
   now it's harmless either way.

Until then, no nrflo-side code change reproduces the original problem
or fixes it.

#### Reproduction (for verification once upstream fix lands)

```
make build
python3 manual_testing/test_codex.py --mode=cli-interactive --parallel=1 --only=s01
```

Inspect `~/.nrflo/nrflo.data`:

```sql
SELECT COUNT(*) FROM agent_messages
WHERE session_id IN (
  SELECT id FROM agent_sessions
  WHERE model_id LIKE 'codex%' AND effective_mode='cli_interactive'
  ORDER BY started_at DESC LIMIT 1
);
```

Bug present → count = 0. Bug fixed → count > 0.

### Opencode `cli_interactive` mode exits immediately on opencode 1.14.48

**Status:** unconfirmed; the "shared root cause with codex" hypothesis
was wrong — codex turned out to be an upstream hook regression, which
doesn't apply here. Opencode uses an HTTP/SSE telemetry channel
(`/event?directory=…`), not hooks. This entry needs its own
investigation.

`effective_mode='cli_interactive'` selection is the single code path
documented in the codex entry above; harness and UI both reach the
same `cliInteractiveBackend`. So the failure is real, not a
harness-vs-UI divergence.

#### Symptoms (harness)
- Every scenario in `opencode/cli-interactive` mode fails within 1-2s.
- `agent_sessions.result='fail'`, `result_reason='exit_code'` — the
  agent process exited non-zero immediately after spawn.
- A handful of scenarios "accidentally" PASS (S11, S14, S18, S22, S24)
  because their assertions only check spawn/DB plumbing, not agent work.
- `opencode/cli` (batch) mode passes 22/24 in the same harness.
- Tested with opencode 1.14.48.

#### Reproduction
```
make build
python3 manual_testing/test_opencode.py --mode=cli-interactive --parallel=1
```

#### Where to look
`be/internal/spawner/cli_adapter_opencode.go` interactive `BuildCommand`:
```
opencode <workDir> --port <N> --hostname 127.0.0.1 --model <MAPPED> [--variant <level>]
```
- Opencode 1.14.48 may have renamed/removed one of those flags
  (check `opencode --help`).
- May require a `--config` file or env var that neither harness nor
  spawner provides.
- The SSE event consumer at `:N/event?directory=<workDir>` may need
  different auth or a different endpoint shape in 1.14.x.
- Capture stderr from the failing opencode process (currently dropped
  by the PTY ferry) to see the actual exit reason.

### Agent-callback prompt not reliably followed by codex/opencode models

#### Symptoms
- `s17_callback` consistently FAILs on `codex/cli` and `opencode/cli`: `L0 did not re-run (a_count=1)`.
- The L1 prompt asks only for `nrflo agent callback --level 0`, which under haiku reliably runs. Under codex's default model and opencode's default model, the agent finishes without calling the callback.

#### Status
This is **not** a backend bug — it's a model instruction-following gap. The scenario has the `MODELS_BY_PROVIDER` override mechanism specifically for this. The fix is either:
- `MODELS_BY_PROVIDER = {"codex": "codex_gpt_high", "opencode": "opencode_gpt54"}` in `s17_callback.py`, OR
- a more directive L1 prompt that all three models follow reliably.

Pick whichever produces a stable green on first try; document the choice in the scenario's docstring.

#### Out of scope
The callback orchestration mechanic itself is fine — it's verified end-to-end on `claude/cli` and `claude/cli-interactive`. This entry is only about the test-side flake.

### Coverage gaps deferred from the 2026-05 batch

Recorded so they don't get forgotten when the next harness iteration lands:

- **WS event subscriber scenario** — open `/api/v1/ws`, run a workflow, assert `agent.completed` and `orchestration.completed` events fire with the expected payload shape. Adds runtime dep on `websockets`.
- **Manual `restart` endpoint** — `POST /api/v1/projects/{id}/workflow/restart` while an agent is still running; assert a fresh `agent_sessions` row is spawned with `ancestor_session_id` linking back. Distinct from `retry-failed`.
- **Ticket concurrency guard** — `POST /tickets/{id}/workflow/run` with one already running and no `force` body field; assert HTTP 409.
- **Notification channels end-to-end** — spawn a tiny in-harness `http.server`, register a Slack channel with that URL, run a workflow, assert the delivery row + the inbound HTTP POST payload.
- **`execution_mode=script`** — create a `python_scripts` row + agent_def pointing at it; assert `scriptBackend` is taken (`effective_mode='script'`) and findings written via the embedded Python SDK land in `agent_sessions.findings`.
- **`execution_mode=api`** — boot server with `--mode=api`, configure an api_credentials row + tool_definitions; assert `apirun.Runner` runs the tool turn loop. Probably a separate `test_api_mode.py` so cli-only hosts can skip cleanly.
- **Multi-skip-tag accumulation** — multiple `nrflo skip <tag>` calls; assert all land in `workflow_instances.skip_tags`.
