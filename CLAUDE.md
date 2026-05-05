# Claude Code Instructions for nrflo

## Overview

nrflo is a multi-workflow state management system for ticket and project-level implementation with spawned AI agents. Supports multiple workflows per ticket, project-scoped workflows (no ticket required), parallel agents (Claude, OpenAI), and real-time WebSocket updates.

The server runs as `nrflo_server serve` and provides an HTTP API + WebSocket for the web UI, plus a Unix socket for agent communication. Spawned agents use the `nrflo` CLI binary (`agent fail/continue`, `findings add/append/get/delete`) to report results.

## New features
Do not keep old / deprecated / backward compat / legacy code
Remove it right away.

## Mandatory Rules

### 1. Update Documentation on Any Change

When making changes, you MUST update all affected documentation:

#### 1a. Root CLAUDE.md (This File)

- Architecture, rules, workflows, state format → update this file

#### 1b. Backend CLAUDE.md

Update [be/CLAUDE.md](be/CLAUDE.md) when modifying:
- Go packages, DB schema, migrations, spawner, CLI adapters, HTTP API, socket methods, tests, build config

#### 1c. UI CLAUDE.md

Update [ui/CLAUDE.md](ui/CLAUDE.md) when modifying:
- Frontend components, pages, WebSocket protocol, API client code, TypeScript types

#### 1d. Agent Manual

The agent manual (`agent_manual.md`) is **user-facing documentation** rendered in the web UI. Keep content focused on what users need to know — no DB table names, internal Go implementation details, session status values, or env var internals.

**Canonical location is the repo root.** The Go embed copy at `be/internal/static/agent_manual.md` is a gitignored build artifact created by `make build` (see `Makefile:39`). Do not edit or commit the embed copy — edit the root file. In a fresh checkout the embed copy will not exist until `make build` runs; this is expected.

Update [agent_manual.md](agent_manual.md) when modifying:
- Template variables in `be/internal/spawner/template.go`
- Findings patterns in `be/internal/spawner/template_findings.go` or `template_project_findings.go`
- Agent CLI commands in `be/internal/cli/agent.go` or `be/internal/cli/findings.go`
- Agent definition schema in `be/internal/model/agent_definition.go`
- Workflow phase format or layer execution rules

Do not document in [agent_manual.md](agent_manual.md):
- API-mode agent configuration (`execution_mode='api'`, `tools` CSV, `api_max_iterations`, `customer_config_dir` manifest tools) — covered by root CLAUDE.md principles 35/36/37/40 and [be/internal/spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md)
- API Credentials (`secret_ref`, `bearer_secret_ref`) — operator/admin territory, not user-facing agent authoring

### 2. Layer-Based Phase Execution

Agents are grouped by `layer` number. All agents in the same layer run concurrently; layers execute in ascending order. The spawner validates:
- All agents in prior layers are completed or skipped before the current layer starts
- Fan-in: if a layer has multiple agents, the next layer must have exactly 1 agent
- At least one agent in a layer must pass for the workflow to proceed (all-skipped continues)

### 3. State is Stored in Database Tables

Workflow runtime state is stored in normalized database tables:
- **`workflow_instances`** — one row per workflow run (multiple per ticket+workflow allowed), stores workflow-level findings, retry count
- **`agent_sessions`** — one row per agent execution, stores result, pid, findings, timestamps
- Phase statuses, phase order, and current phase are derived from `agent_sessions` + agent_definitions (layer field) at read time
- Active agents = `agent_sessions WHERE status = 'running'`
- Agent history = `agent_sessions WHERE status != 'running'`

### 4. CRITICAL: Backend Test Suite Must Run in Under 60 Seconds

The full backend test suite (`make test`) must complete in **≤60 seconds wall time**. Enforced by the Makefile target.

**Never introduce:**
- `time.Sleep` in tests — use `clock.TestClock.Advance()` for time-dependent logic, or poll with a tight loop+timeout for async conditions
- Unnecessary waits after hub `Register`/`Subscribe` — these are synchronous via mutex
- Sleeps waiting for log output — logging before goroutine launch is synchronous
- Real CLI binary execution in tests — always mock adapters/commands (e.g., `exec.Command("echo", "ok")` via injectable functions). Real binaries may hang waiting for input and stall the entire suite

**Patterns that are allowed:**
- `waitForCondition(t, 2*time.Second, 5*time.Millisecond, fn)` — tight polling with short timeout for genuinely async operations
- `clock.TestClock.Advance(d)` — advance fake clock instead of sleeping
- `env.Clock.Advance(d)` — in integration tests

**After editing or creating a test**, run that single test file immediately (`make test-pkg PKG=<package>` for BE, `make test-ui ARGS="path/to/file.test.tsx"` for FE) and verify it completes quickly (under 5s for a single test). If it stalls or is slow, fix it before proceeding.

If the test suite exceeds 60 seconds, **identify and eliminate the cause before merging**.

### 4b. CRITICAL: Frontend Test Suite Must Run in Under 60 Seconds

The full frontend test suite (`make test-ui`) must complete in **≤60 seconds wall time**. Enforced by the Makefile target. Pool is set to `threads` in `vitest.config.ts` for speed.

**Never introduce:**
- `setTimeout` in test bodies or mock implementations — use `new Promise(() => {})` (never-resolving) to keep a mutation in-flight for `isPending` tests
- Real delays inside mock API implementations — mocks should resolve immediately or stay pending

**Patterns that are allowed:**
- `vi.useFakeTimers()` + `vi.advanceTimersByTime(ms)` — for timer-dependent components
- `new Promise(() => {})` — keeps mutation `isPending: true` without any real delay
- `waitFor(() => expect(...))` — RTL polling for genuinely async React state

**After editing or creating a test**, run that single test file immediately (`make test-ui ARGS="path/to/file.test.tsx"`) and verify it completes quickly (under 5s for a single test). If it stalls or is slow, fix it before proceeding.

If the test suite exceeds 60 seconds, **identify and eliminate the cause before merging**.

### 5. Keep Source Files Under 300 Lines

Source files should be kept under 300 lines when possible. When a file grows beyond this limit, split it into logical sub-files. This applies to both code and documentation files.

### 5. Documentation Hierarchy

Root `CLAUDE.md` contains only project-level information (architecture principles, mandatory rules, CLI commands, workflows, brief summaries). Detailed implementation docs belong in subdirectory `CLAUDE.md` files:
- Backend package details → `be/internal/<package>/CLAUDE.md`
- Frontend module details → `ui/src/<module>/CLAUDE.md`
- DB schema, full API listings, spawner internals, etc. must NOT be duplicated in root — use cross-references instead.

### 6. Polymorphism lives in the implementation, not the call site

When you find yourself writing `if x.Name() == "foo"` (or any equivalent type/name-string switch) at a call site that already holds a polymorphic interface, **push the divergence into the interface** — don't accumulate name-checks at the call site.

- One `if name == "x"` is a paper cut.
- Three is a structural problem — the interface is missing a method.
- Four guarantees the next contributor adds a fifth.

When you spot the second name-string branch on the same dispatcher, stop and extend the interface (or extract a sub-interface) so each implementation owns its divergence in its own file. Don't ship "I'll clean it up later"; later becomes a fifth branch.

Applies to all polymorphic seams: CLI adapters, execution backends, providers, repos, services. Generic code must not reach back into a name-tag check; the interface decides.

Concrete prior case: codex-only setup leaked into `backend_interactive.go` as three `b.adapter.Name() == "codex"` branches; resolved by extending `CLIAdapter` with `PrepareInteractive` / `DeliversPromptInline` / `NeedsTerminalQueryReplies` so codex specifics live entirely in `cli_adapter_codex*.go`.

## Key Files

| File | Purpose |
|------|---------|
| `be/` | Go backend source code (see [be/CLAUDE.md](be/CLAUDE.md)) |
| `ui/` | React web interface (see [ui/CLAUDE.md](ui/CLAUDE.md)) |
| `Makefile` | Build, install, test targets (`make help`) |
| `agent_manual.md` | User-facing agent definition guide (template vars, findings, CLI) |

## Architecture Principles

1. **Server-only**: `nrflo_server serve` is the only user-facing command; all management via web UI
2. **Agent CLI subset**: Spawned agents use `agent fail/continue`, `findings add/append/get/delete`, and `project_findings add/add-bulk/get/append/append-bulk/delete` via Unix socket
3. **Auto-migrate**: Database migrations run automatically on server startup
4. **Two Go binaries**: `nrflo_server` (serve command only) and `nrflo` (agent/findings/tickets/deps CLI)
5. **Project-scoped**: Project discovered from `NRFLO_PROJECT` env variable
6. **Single database**: `~/.nrflo/nrflo.data` (SQLite, global for all projects; override with `NRFLO_HOME`)
7. **Connection Pool**: DB uses connection pooling (10 max, 5 idle)
8. **Versioned migrations**: Schema managed by golang-migrate with embedded SQL files in `db/migrations/`
9. **Service Layer**: Business logic separated from HTTP handlers and socket handlers
10. **Spawner in-process**: Spawner runs inside the server, broadcasts WebSocket events via direct hub (no socket fallback)
11. **Layer-based execution**: Phases grouped by layer; same-layer agents run concurrently, layers execute sequentially with fan-in (pass_count >= 1)
12. **WebSocket real-time**: UI receives all real-time updates via WebSocket (`/api/v1/ws`), no REST polling
13. **DB-stored workflow definitions**: Workflow definitions stored in `workflows` table (no phases column — phases are derived from agent_definitions at read time), managed via `/api/v1/workflows` API
14. **DB-stored agent definitions**: Agent definitions (model, timeout, prompt template, layer) stored in `agent_definitions` table, managed via `/api/v1/workflows/{wid}/agents` API. Each agent definition has a `layer` field that determines phase execution order — phases are derived from agent_definitions rather than stored on the workflow. The spawner loads templates exclusively from DB. Templates support `${VAR}` variable substitution and `#{FINDINGS:agent}` / `#{PROJECT_FINDINGS:key}` pattern expansion. User instructions, callback instructions, and low-context data are no longer in-place `${VAR}` substitutions — they are auto-prepended injectable blocks loaded from `default_templates` (type=`injectable`), editable on the Default Templates page. In api-mode, agents may also have access to manifest tools loaded from `customer_config_dir` (see principle 40). See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for full template variable reference. See [agent_manual.md](agent_manual.md) for the complete agent definition cheat-sheet.
15. **Server-side orchestration**: Workflows run from the web UI via `POST /api/v1/tickets/:id/workflow/run`. The orchestrator groups phases by layer and runs all agents in each layer concurrently (one goroutine per agent calling `spawner.Spawn()`), with cancellation support via `/workflow/stop`.
16. **Low-context relaunch**: When an agent's context drops below threshold (default ~25% remaining, configurable per agent via `restart_threshold` in agent_definitions), the spawner kills the agent and saves context via one of two methods. Path selection is computed by `Spawner.shouldUseAgentSave(proc)`: the **system-agent path** (spawns a `context-saver` haiku that summarizes message history) is forced when (a) the global `context_save_via_agent` setting is true, OR (b) the backend is API-mode, OR (c) the adapter's `SupportsResume() == false` (covers codex). Otherwise the **resume-based path** is used (resumes the same Claude session with a save prompt; only Claude qualifies). Both methods write progress under the `to_resume` findings key; a fresh agent is then spawned with `${PREVIOUS_DATA}` populated from that key. Old sessions get `status='continued'` and are excluded from agent history.
17. **Manual agent restart**: Users can trigger an agent restart from the UI via `POST /api/v1/tickets/:id/workflow/restart` with `{workflow, session_id}`. This triggers the same context-save-and-relaunch flow as the automatic low-context restart, regardless of current token usage.
18. **Retry failed agent**: Users can retry a failed workflow from the failed layer via `POST /api/v1/tickets/:id/workflow/retry-failed` (or `/api/v1/projects/:id/workflow/retry-failed`) with `{workflow, session_id}`. This resets the workflow instance to active, resets phases in the failed layer to pending, increments retry_count, and re-runs the orchestration starting from the failed layer.
19. **Project-scoped workflows**: Workflows can have `scope_type` of `ticket` (default) or `project`. Project-scoped workflows run at project level without requiring a ticket and do not use git worktrees (always run in the original project root). Multiple concurrent instances of the same project workflow are allowed (each gets a unique instance_id). API: `POST /api/v1/projects/:id/workflow/run`, `GET /api/v1/projects/:id/workflow` (returns `all_workflows` keyed by instance_id), `GET /api/v1/projects/:id/agents`. Stop/restart/retry-failed accept optional `instance_id` to target a specific instance. Project agents cannot use `${TICKET_ID}`, `${TICKET_TITLE}`, or `${TICKET_DESCRIPTION}` template variables.
20. **Agent callbacks**: A later-layer agent (e.g., qa-verifier) can trigger a callback to re-run an earlier layer (e.g., implementor). The orchestrator saves `_callback` metadata (instructions, from_agent, level) to workflow instance findings, resets phases/sessions from the target layer forward, and jumps the execution loop back. The target agent's prompt can include `${CALLBACK_INSTRUCTIONS}` to receive the callback instructions. After the callback target layer completes successfully, `_callback` is cleared from findings. Max 3 callbacks per workflow run.
21. **Clock abstraction**: All DB timestamp generation uses a `clock.Clock` interface (`internal/clock/`). Production uses `clock.Real()` (wall clock). Tests use `clock.TestClock` with `Set()`/`Advance()` for deterministic time control, eliminating `time.Sleep` for timestamp ordering.
22. **Take-control (interactive session)**: Users can take interactive control of a running Claude agent via `POST /api/v1/tickets/:id/workflow/take-control` (or `/api/v1/projects/:id/workflow/take-control`) with `{workflow, session_id}`. This kills the running agent, sets session status to `user_interactive`, and returns the session ID for `claude --resume`. The spawner blocks (does not advance to next phase) until `POST .../exit-interactive` with `{workflow, session_id}` is called, which marks the session `interactive_completed` with `result=pass` and unblocks the spawner. Only works for Claude CLI agents (`SupportsResume() == true`). For finished sessions (completed/failed/timeout), use `POST .../resume-session` with `{session_id}` instead — this sets the session to `user_interactive` directly without requiring a running orchestration, then reuses the same PTY handler and exit flow.
23. **PTY WebSocket endpoint**: `GET /api/v1/pty/{session_id}` upgrades to a 1:1 WebSocket and spawns `claude --resume <session_id>` in a pseudo-terminal. Relays stdin/stdout bidirectionally between browser and PTY. Handles terminal resize via JSON `{"type":"resize","rows":N,"cols":N}` text messages. On process exit, triggers exit-interactive flow (status → `interactive_completed`, unblocks spawner). Separate from the broadcast WS at `/api/v1/ws`. Session must be in `user_interactive` status.
24. **Stall detection and auto-restart**: The spawner monitors time since last agent message. If an agent produces no output within `stall_start_timeout_sec` (default 120s) or stops producing output for `stall_running_timeout_sec` (default 480s), it is killed and relaunched via the continuation mechanism after a 15s delay. No context save is attempted (agent is frozen). Stall restarts are capped at 15 per agent. Timeouts are configurable per agent_definition (NULL = defaults, 0 = disabled). `agent.stall_restart` and `agent.stall_waiting` WS events are broadcast. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for details.
25. **Interactive start & plan mode**: The workflow run API accepts optional `interactive` and `plan_mode` boolean flags. When `interactive: true`, the orchestrator spawns only the L0 agent in interactive mode and returns its `session_id` in `RunWorkflowResponse`. When `plan_mode: true`, a planner agent is spawned interactively. The web UI `RunWorkflowDialog` and `RunWorkflowForm` expose these as mutually exclusive checkboxes; on success the `onInteractiveStart` callback opens the PTY terminal via `AgentTerminalDialog`.
26. **Spawner env vars for direct targeting**: The spawner sets `NRF_WORKFLOW_INSTANCE_ID` and `NRF_SESSION_ID` env vars on every spawned agent. The CLI reads these and passes them in all socket requests (`instance_id`, `session_id` fields). The service uses them directly — no ambiguous DB lookup by `(project, ticket, workflow)`. This is required; agents without these env vars cannot use findings or agent commands. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for the full env var list.
27. **System agent definitions**: Global agent definitions not tied to any project or workflow, stored in `system_agent_definitions` table. Managed via `/api/v1/system-agents` CRUD endpoints (no `X-Project` header required). Used for system-level agents like conflict-resolver.
28. **Automatic merge conflict resolution**: When `MergeAndCleanup()` fails after workflow completion, the orchestrator attempts automatic resolution by spawning a `conflict-resolver` system agent. The agent receives `${BRANCH_NAME}`, `${DEFAULT_BRANCH}`, and `${MERGE_ERROR}` via `ExtraVars`. On success, the feature branch is deleted; on failure or missing resolver, falls through to manual resolution. See [be/internal/orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) for details.
29. **Low consumption mode**: Global setting (`GET/PATCH /api/v1/settings`) stored in the `config` table. When enabled, the spawner overrides the model for agents that have a `low_consumption_model` configured in their agent definition (e.g., `"sonnet"`, `"haiku"`). Only the model is swapped — the agent's own prompt template, timeout, and settings are kept. Session records (agent_type, phase tracking) retain the original agent type. The setting is read once at workflow start — toggling mid-workflow has no effect until the next run. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for details.
30. **Global stall timeouts**: Global settings `stall_start_timeout_sec` and `stall_running_timeout_sec` override the hardcoded stall detection defaults (120s and 480s) when agent definitions don't specify per-agent values. Priority: per-agent def > global config > hardcoded default. Settings are read once at workflow start. `0` = disabled, positive integer = custom seconds, null/empty = use hardcoded default. Managed via `GET/PATCH /api/v1/settings`.
31. **Default templates**: Global prompt templates stored in `default_templates` table with a `type` column (`agent` or `injectable`). Six readonly agent templates are seeded by migration (setup-analyzer, test-writer, implementor, qa-verifier, doc-updater, ticket-creator). Three readonly injectable templates are seeded (low-context, callback, user-instructions) with fixed IDs for spawner lookup. Users can create additional non-readonly templates. Managed via `/api/v1/default-templates` CRUD endpoints (no `X-Project` header required); supports `?type=` filter. Readonly templates allow template text edits but not name/type changes or deletion. A `default_template` column stores the original text for readonly templates; `POST /api/v1/default-templates/:id/restore` resets the template to this original value.
32. **Error tracking**: Actionable errors (agent fail/timeout, workflow failures, merge conflict failures) are stored in the `errors` table and broadcast via `error.created` WS event. Paginated API: `GET /api/v1/errors?type=agent&page=1&per_page=20` (requires `X-Project` header). The spawner and orchestrator record errors via `ErrorService.RecordError()`. See [be/internal/service/CLAUDE.md](be/internal/service/CLAUDE.md) and [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md) for details.
33. **Push after merge**: Per-project setting (`push_after_merge`) stored in the `config` table via project config. When enabled, the orchestrator pushes the default branch to origin after a successful worktree merge or conflict resolution. Best-effort: push failure is logged and broadcast (`workflow.push_failed` WS event) but does not fail the workflow. `workflow.pushed` is broadcast on success. The setting is read once at workflow start.
34. **Endless loop mode (project-scoped)**: Run project-scoped workflows in an auto-repeating loop. The run request accepts `endless_loop: true` (mutually exclusive with `interactive`/`plan_mode` and rejected for ticket-scoped workflows; server clears `instructions` when set). The flag is persisted on `workflow_instances.endless_loop`. After a successful completion the orchestrator re-reads the instance and, if `stop_endless_loop_after_iteration` is false, the caller did not `Stop()`, and no new workflow is already running for the same `(project_id, workflow)`, spawns a fresh `workflow_instances` row and calls `Start()` again with `EndlessLoop=true` (no instructions). Failed iterations and `Stop()` always terminate the loop. Users toggle graceful stop via `POST /api/v1/projects/{id}/workflow/stop-endless-loop` with `{instance_id, stop}`; only valid while the instance is active. All transitions broadcast `EventWorkflowUpdated`.
35. **API-mode execution (in-process Anthropic runner)**: Agent definitions with `execution_mode='api'` are driven by an in-process `apirun.Runner` (`be/internal/spawner/apirun/`) instead of a CLI child process. The runner calls the Anthropic Messages API directly, dispatches tool calls via a per-agent registry (builtins + HTTP tool definitions), and coalesces streaming deltas into WS messages via `runnerSink`. Tool registry is resolved at spawn time from the agent's `tools` CSV field using glob matching against builtins and project-scoped HTTP tool definitions. API-mode agents share the same lifecycle signals as CLI agents (stall detection, low-context save, fail/continue/callback terminal tools) with two exceptions: (a) **take-control is unsupported** — HTTP handlers return HTTP 409 `api_mode_unsupported` and the spawner broadcasts `agent.take_control_rejected` without killing the agent; (b) **low-context save always uses the system-agent path** (forces `useAgentSave=true`) because the resume path is Claude-CLI-only. API-mode low-context save is routed through a `context-saver-api` sibling row in `system_agent_definitions` (selected by `GetForBackend("context-saver", "api")`); when no api variant exists the spawner falls back to the default CLI context-saver. The `AgentConfig` struct (`ExecutionMode`, `Tools`, `APIMaxIterations`) carries execution mode into ephemeral spawners so the context-saver-api variant can run via the in-process runner. Automatic merge conflict resolution (`conflict-resolver`) is still CLI-only — no API variant is defined yet. See [be/internal/spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md) for full architecture, tool dispatch, builtins, HTTP tool handler, and stall/low-context/take-control behavior.
36. **API-mode gating**: `nrflo_server serve --mode=api` (default `cli`) is required to enable in-process Anthropic execution. In `cli` mode (default): `execution_mode='api'` agent definitions cannot be created or updated (HTTP 400 `api_mode_disabled`); the spawner rejects any stale `api`-mode rows with `api_mode_disabled` before touching any provider; tool-definition and api-credential HTTP endpoints return 404 (routes not registered). The `GET /api/v1/settings` response includes `api_mode_enabled` (bool, read-only) reflecting the startup flag.
37. **Interactive CLI backend**: When the per-project `interactive_cli_mode` setting is enabled, the spawner uses `cliInteractiveBackend` instead of `cliBackend` for CLI agents whose adapter returns `SupportsInteractive() == true`. Claude, Codex, and **Opencode** all qualify. The agent process is spawned inside a PTY (no batch flags). Prompt delivery asymmetry: Claude and Opencode receive the rendered body via PTY stdin Write after a ~250ms readiness delay; Codex receives it as the final argv positional (codex TUI wrapping bug at `tui/src/wrapping.rs:52`). Suffix delivery asymmetry: Claude uses `--append-system-prompt-file`; Codex/Opencode prepend to the prompt body in memory. Hook telemetry delivery: Claude uses `--settings` JSON hooks; Codex uses repeated `-c hooks.<event>=…` CLI flags; **Opencode uses the embedded SSE bus** — `opencode <workDir> --port N --hostname 127.0.0.1` starts both a TUI and an HTTP event server; an in-process SSE consumer (`cli_adapter_opencode_events.go`) subscribes to `GET /event?directory=<workDir>` and dispatches tool/text/context/turn-complete signals through the same Sink interface paths as Claude/Codex hooks. Take-control in this mode is a **viewer-attach** — the agent is not killed; `agent.viewer_attached` is broadcast and the viewer connects via the standard `/api/v1/pty/{session_id}` endpoint. System agents (conflict-resolver, context-saver-CLI) inherit the same backend selector. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for full backend selection rules, PTY lifecycle, Opencode SSE consumer details, and output ferry behavior.
38. **Idle/nudge loop**: For `cliInteractiveBackend` agents only, the spawner monitors time since the last output. When an agent is silent for `idleStartTimeout` (default 120s before first message) or `idleAfterMessageTimeout` (default 180s after first message), it writes the `finish-reminder` injectable to PTY stdin and broadcasts `agent.nudged` (attempt/max). After `nudgeMax` (default 5) nudges, if the agent remains silent for another full idle window, the spawner calls `AgentService.Fail(reason="unresponsive_after_nudges")` + `RequestTerminalSignal` and records an error row. The nudge count is persisted in `agent_sessions.nudge_count` (migration 000065) and surfaced in the V4 workflow response for both active agents and history. Configurable via `spawner.Config.IdleAfterMessageTimeoutSec`, `IdleStartTimeoutSec`, `NudgeMax`. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for full details.
39. **Scheduled tasks (cron-driven workflow runs)**: Per-project schedules stored in `scheduled_tasks` and `schedule_runs` tables. Each task has a standard 5-field cron expression and a list of project-scoped workflow IDs **and/or** workflow chain IDs to trigger (at least one of the two must be non-empty). The `internal/scheduler/` package wraps `github.com/robfig/cron/v3`; it loads enabled tasks at `Server.Start`, re-registers on `Reload()` (called after every Create/Update/Delete), and stops gracefully in `Server.Stop`. Dispatching one task fans out: (a) one `orchestrator.Start(ScopeType="project")` call per workflow and (b) one `wfChainRunSvc.CreateRun` + `wfChainRunner.Start` call per chain ID (non-blocking — chains run asynchronously). Outcomes are recorded in `schedule_runs` (`workflows` JSON + `chain_runs` JSON columns) and broadcast via `schedule.triggered`. CRUD API: `GET/POST /api/v1/scheduled-tasks`, `GET/PATCH/DELETE /api/v1/scheduled-tasks/{id}`, `GET /api/v1/scheduled-tasks/{id}/runs`, `POST /api/v1/scheduled-tasks/{id}/run-now`. All endpoints require `X-Project` header. See [be/internal/scheduler/CLAUDE.md](be/internal/scheduler/CLAUDE.md).
41. **Auth foundation (users/sessions/audit_log)**: User accounts stored in `users` table (migrations 000075–000078). Passwords hashed with Argon2id (m=64MiB, t=3, p=2; PHC format). Sessions persisted in `sessions` table via SCS sqlite3store (`nrflo_session` cookie, 24h lifetime, 8h idle). Audit events written to `audit_log` table (FK to users ON DELETE SET NULL). Services: `AuthService` (Login, ChangePassword) and `UserService` (Create, Update, ResetPassword, Delete) with last-admin protection. HTTP handlers and middleware wired in `be/internal/api/` — see principle 42. See [be/internal/auth/CLAUDE.md](be/internal/auth/CLAUDE.md) for Argon2id parameters and SCS defaults.
42. **Authentication middleware and HTTP wiring**: SCS session manager (`*scs.SessionManager`) is constructed in `api.NewServer` via `auth.NewManager(pool.DB, insecureCookies)`. The handler chain applies `LoadAndSave` only for `/api/*` paths (static SPA routes excluded). Per-route auth uses `protected` (requireAuth) or `admin` (requireAdmin) helpers in `registerRoutes`; login is the only public route. `requireAuth` stashes `*model.User` in context; `getUser(r)` / `getUserID(r)` retrieve it in handlers. Admin-gated writes: system-agents, default-templates, cli-models, tool-definitions, api-credentials, scheduled-tasks (write methods), workflow-chains (write methods), `PATCH /settings`, `POST /projects`, `DELETE /projects/{id}`. Login rate limiter: 5 attempts / 5 min per IP+email (HTTP 429 + `Retry-After`). `--insecure-cookies` flag disables Secure cookie (local dev). `GET /api/v1/ws` and `GET /api/v1/pty/{session_id}` are auth-gated before WebSocket upgrade. When `sessionMgr == nil` (tests using partial `*Server`), `requireAuth` passes through transparently.
43. **User & audit administration**: Admin-only HTTP endpoints in `be/internal/api/handlers_users.go` and `handlers_audit.go`. `GET/POST /api/v1/users`, `PATCH/DELETE /api/v1/users/{id}`, `POST /api/v1/users/{id}/reset-password` — all gated via `requireAdmin`. Handlers call `service.UserService` (Create/Update/ResetPassword/Delete) which enforces last-admin protection (`ErrLastAdmin` → HTTP 400 `last_admin`) and self-delete prevention (`ErrSelfDelete` → HTTP 400 `cannot_delete_self`). Email uniqueness violations map to HTTP 409 `email_exists`. New users are created with `must_change_password=1`. `model.User.PasswordHash` carries `json:"-"` so it is never serialized. Each mutating handler appends an audit entry via `appendAudit()` (actions: `user_create`, `user_update`, `user_delete`, `password_reset_by_admin`). `GET /api/v1/audit-log` (admin-only) returns paginated entries: `?page=&per_page=` (default 1/50, max 200) and `?user_id=&action=` filters via `repo.AuditRepo.List`.
44. **Python scripts (project-scoped DB storage)**: Python scripts stored in `python_scripts` table (migration 000085). Each script has `id` (PK), `project_id` (FK → projects, CASCADE), `name`, `description`, `code`, timestamps. `agent_definitions.python_script_id` (TEXT NULL, added by migration 000085) links an agent to a stored script; `execution_mode` now accepts `'script'` in addition to `'cli'` and `'api'`. Spawner wiring (loading and executing the script when `execution_mode='script'`) is T2. CRUD API: `GET/POST /api/v1/python-scripts` (project-scoped via X-Project header; writes admin-only), `GET/PATCH/DELETE /api/v1/python-scripts/{id}`, `POST /api/v1/python-scripts/validate` (syntax check only, no DB write). See [be/internal/service/CLAUDE.md](be/internal/service/CLAUDE.md) and [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md).
45. **Python SDK and script.context socket method**: A single-file Python SDK (`nrflo_sdk.py`, pure stdlib, <300 lines) is embedded in the server binary via `//go:embed` in `be/internal/sdk/python/embed.go` (package `pythonsdk`). On every `nrflo_server serve` startup, `pythonsdk.WriteSDK(sdkDir)` writes it to `$NRFLO_HOME/sdk/nrflo_sdk.py` (best-effort; WARN logged on failure). The spawner sets `NRFLO_SDK_DIR` for script-mode agents so they can `sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])` before importing. SDK entry point: `import nrflo_sdk; c = nrflo_sdk.client()`. Exposes `c.findings.*`, `c.project_findings.*`, `c.agent.{finished,fail,continue_,callback}`, `c.context(refresh=False)` (cached call to `script.context`), `c.user_instructions()`, `c.callback_info()`, `c.previous_data()`, `c.skip(tag)`. The underlying `script.context` Unix-socket method (handler in `be/internal/socket/handler_script_context.go`) takes `{session_id}` in params, resolves `agent_sessions` → `workflow_instances` → `tickets` (ticket-scoped only), and returns a 12-key dict: `session_id`, `instance_id`, `project_id`, `agent_type`, `workflow_id`, `scope_type`, `ticket_id`, `ticket_title`, `ticket_description`, `user_instructions` (string, "" if absent), `callback` (null or `{instructions,from_agent,level}`), `previous_data` (string, "" if no `to_resume` session finding). No project field required at the top level — project is derived from the session row.
40. **Manifest tools (api-mode only)**: When the per-project `customer_config_dir` setting points to a valid directory containing a `tool_manifest.yaml`, the spawner loads it at spawn time (mtime-cached, reloaded only on change) and registers each manifest tool as a third tool source in `apirun.ResolveRegistry` (after builtins and HTTP defs). Manifest tools are `python_script` type — the server invokes the script via `python3` with tool input on stdin and expects JSON output on stdout. Tool dispatch is recorded in the `tool_dispatches` table and broadcast via `tool.dispatched`. Tools flagged `review: true` additionally create a `review_items` row (status=pending) and broadcast `review.created`; reviewers approve or reject via the project Review queue page. Config files referenced by manifest tools are versioned in `customer_config_versions` and editable via the Config Editor page. The insights dashboard aggregates dispatch counts, edit rates, and throughput. All review/config/insights pages and endpoints are api-mode only; in cli-mode (default) or when `customer_config_dir` is empty, manifest tools are silently skipped. See [be/internal/manifest/CLAUDE.md](be/internal/manifest/CLAUDE.md) and [be/internal/spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md) for implementation details.

## Quick Start

Install via Homebrew: `brew tap nrflo/tap && brew install nrflo`. Upgrade: `brew update && brew upgrade nrflo`.

Or build from source: `make build && make install`.

Or run the Linux container (api-mode only, no agent CLIs in image):
```bash
docker run -d -p 6587:6587 -v nrflo-data:/data \
  -e ANTHROPIC_API_KEY=sk-ant-... \
  ghcr.io/nrflo/nrflo-server:latest
```

Then `nrflo_server serve` and open `http://localhost:6587`.

By default the server binds to `127.0.0.1` (localhost only). To make it accessible on the local network: `nrflo_server serve --host 0.0.0.0`

### nrflo_server subcommands

| Command | Description |
|---------|-------------|
| `serve` | Start the HTTP/WebSocket server (default when no subcommand given) |
| `version` | Print version information |
| `init-customer` | Scaffold a starter customer config directory for manifest-driven api-mode workflows. See [be/internal/manifest/CLAUDE.md](be/internal/manifest/CLAUDE.md). |

```bash
nrflo_server init-customer --out /path/to/my-customer --name MyCustomer --git
```

## Agent CLI Commands

Spawned agents use these commands to report results (via Unix socket to the server). Exit 0 = implicit pass; `agent finished` is the explicit equivalent. `agent continue` is reserved for context-exhaustion relaunch (NOT a success signal).

```bash
# All context derived from NRF_SESSION_ID + NRF_WORKFLOW_INSTANCE_ID env vars (set by spawner)
nrflo agent finished                 # explicit success — proceed to next phase
nrflo agent fail [--reason <text>]
nrflo agent continue                 # context exhausted — relaunch with fresh context
nrflo agent callback --level <N>

nrflo skip <tag>  # Add skip tag to running workflow instance (reads NRF_WORKFLOW_INSTANCE_ID from env)

# Own-session findings (write to current agent's session)
nrflo findings add <key> <value>
nrflo findings add key1:val1 [key2:val2...]
nrflo findings append <key> <value>
nrflo findings append key1:val1 [key2:val2...]
nrflo findings get [key] [-k <key>...]
nrflo findings delete <key1> [key2...]

# Cross-agent read (provide target agent-type; uses NRF_WORKFLOW_INSTANCE_ID to scope)
nrflo findings get <agent-type> [key] [-k <key>...]

# Project-level findings (scoped to NRFLO_PROJECT)
nrflo findings project-add <key> <value>
nrflo findings project-add key1:val1 [key2:val2...]
nrflo findings project-get [key] [-k <key>...]
nrflo findings project-append <key> <value>
nrflo findings project-append key1:val1 [key2:val2...]
nrflo findings project-delete <key1> [key2...]
```

## Ticket CLI Commands

Manage tickets and dependencies via the HTTP API (requires server running):

```bash
nrflo tickets list [--status <status>] [--type <type>] [--parent <id>] [--json]
nrflo tickets get <id> [--json]
nrflo tickets create --title <title> [--id <id>] [--description <text>] [--type <type>] [--priority <1-4>] [--parent <id>] [--created-by <name>] [--json]
nrflo tickets update <id> [--title <title>] [--description <text>] [--type <type>] [--priority <1-4>] [--parent <id>]
nrflo tickets close <id> [--reason <text>]
nrflo tickets reopen <id>
nrflo tickets delete <id>

nrflo deps list <ticket-id> [--json]
nrflo deps add <ticket-id> <blocker-id>
nrflo deps remove <ticket-id> <blocker-id>
```

All ticket/deps commands use `--server` (default `NRFLO_API_URL` or `http://localhost:6587`) and require `NRFLO_PROJECT` env variable.

## Workflows

| Workflow | Phases (by layer) | Use Case |
|----------|-------------------|----------|
| `feature` | L0: setup-analyzer -> L1: test-writer -> L2: implementor -> L3: qa-verifier -> L4: doc-updater | New features (full TDD) |
| `bugfix` | L0: setup-analyzer -> L1: implementor -> L2: qa-verifier | Bug fixes |
| `hotfix` | L0: implementor | Urgent fixes |
| `docs` | L0: setup-analyzer -> L1: doc-updater | Documentation only |
| `refactor` | L0: setup-analyzer -> L1: implementor -> L2: qa-verifier | Code refactoring |

**Note:** These are example workflow configurations. Workflows are stored in the database and must be created via the `/api/v1/workflows` API or the Workflows page in the web UI.

Workflow definitions support a `close_ticket_on_complete` boolean (default true). When false, the orchestrator skips ticket auto-closing after successful completion. Only applies to ticket-scoped workflows. The flag is read at workflow start time.

### Phase Definition Format

Phases are derived from agent definitions at read time. Each agent definition has an `id` and a `layer` (integer >= 0) field. Agents are grouped by layer for concurrent execution; layers run in ascending order. There is no `phases` column on the `workflows` table — the phase list is built from the workflow's agent_definitions ordered by `layer ASC, id ASC`. Supported models are loaded from the `cli_models` DB table (seeded with `opus_4_6`, `opus_4_6_1m`, `opus_4_7`, `opus_4_7_1m`, `sonnet`, `haiku`, `opencode_minimax_m25_free`, `opencode_qwen36_plus_free`, `opencode_gpt54`, `codex_gpt_normal`, `codex_gpt_high`, `codex_gpt54_normal`, `codex_gpt54_high`). Custom models can be added via `POST /api/v1/cli-models` and are immediately valid for agent definitions. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for model mapping details.

## State Storage

Workflow runtime state is stored in two main tables: `workflow_instances` (one row per workflow run, stores findings, retry count) and `agent_sessions` (one row per agent execution, stores result, pid, findings, context usage, timestamps). The `errors` table stores actionable errors from agent failures, workflow failures, and system errors (e.g., merge conflicts). Phase statuses, phase order, and current phase are derived at read time from `agent_sessions` rows + agent_definitions (which define the phases via their `layer` field). Both ticket-scoped and project-scoped workflows allow multiple instances; concurrent runs of the same ticket+workflow are prevented by the orchestrator's `IsRunning` check (not DB constraint). Completion statistics (`completed_at`, `total_duration_sec`, `total_tokens_used`) are computed from agent session data. See [be/internal/db/CLAUDE.md](be/internal/db/CLAUDE.md) for full schema.

## API Response Format

`GET /api/v1/tickets/:id/workflow` returns a v4 format wrapper with `ticket_id`, `has_workflow`, `workflows` list (deduplicated workflow names), and `all_workflows` map keyed by instance_id (same pattern as project workflows). Each workflow state includes version, status, instance_id, workflow name, current phase, phase order, phase layers, phases map (derived from agent_definitions), active agents, agent history, and findings. A separate `workflow_findings` field contains workflow-level findings (from `workflow_instances.findings`) filtered to exclude internal keys (starting with `_`); omitted when empty. When any agent writes a `workflow_final_result` finding, the last-written value (by session `ended_at`) is included as a top-level `workflow_final_result` field; this value is also forwarded to `orchestration.completed` notifications when notification channels are configured. Supports `?instance_id=` and `?workflow=` query params for selection. See [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md) for full endpoint listing and [be/internal/service/CLAUDE.md](be/internal/service/CLAUDE.md) for response construction.

## Chain Execution

Chains allow sequential execution of multiple tickets with a single workflow. Tickets are expanded with transitive dependency blockers, topologically sorted, and locked to prevent overlapping runs. Items execute one at a time via the orchestrator. Zombie running chains are marked failed on server startup (crash recovery). See [be/internal/orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) for chain runner details and [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md) for chain API endpoints.

## Workflow Chain Definitions and Runs

Named sequences of workflow+step configurations stored in `workflow_chains` / `workflow_chain_steps` tables. Each chain is a project-scoped ordered list of steps; step 0 must be `scope_type=project`, steps may be `ticket` or `project` scoped, and positions are dense `0..N-1`. `require_ticket_handoff` is only valid for ticket-scoped steps. CRUD API at `/api/v1/workflow-chains` (reads protected, writes admin-only); WS events `chain_def.created/updated/deleted`. UI: `WorkflowChainsPage` and `WorkflowChainEditorPage`.

**Execution engine** (`be/internal/chainrunner/`): Runs persisted in `workflow_chain_runs` / `workflow_chain_run_steps` tables. `chainrunner.Runner` executes steps sequentially via the orchestrator; each step spawns a `workflow_instances` row. WS events: `chain.run_started/step_started/step_completed/run_completed/run_failed`. API: `POST /api/v1/workflow-chains/{id}/runs` starts; `GET /runs` / `GET /runs/{runId}` list/get; `POST /runs/{runId}/cancel` (admin).

**Agent handoff**: Agent in step N sets data for step N+1 before finishing:
- `nrflo agent chain-next-instructions --instructions "..."` → next step's `instructions_used`
- `nrflo agent chain-next-ticket --ticket-id "..."` → next step's `ticket_id` (for ticket-scope steps)

See [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md) and [ui/CLAUDE.md](ui/CLAUDE.md).

## Building & Installing

```bash
make build          # Build both binaries (dev, includes UI)
make build-release  # Optimized release build
make install        # Install to /usr/local/bin (or PREFIX=...)
make test           # Run backend tests
make help           # Show all targets
```

### Docker image (linux/amd64+arm64, api-mode only)

Distributed as `ghcr.io/nrflo/nrflo-server` (built by [`Dockerfile`](Dockerfile) and [`.github/workflows/docker.yml`](.github/workflows/docker.yml)). The image:
- Hard-bakes `serve --mode=api --host 0.0.0.0` in its ENTRYPOINT — only api-mode workflows run.
- Ships `python3`, `git`, `ca-certificates`, `tini`, and the static `nrflo_server` binary. No `claude`, `codex`, or `opencode` CLIs (api-mode runs the Anthropic Messages API in-process via `be/internal/spawner/apirun`). Mount a customer config directory via `-v /path/to/customer:/customer-config` and set `customer_config_dir=/customer-config` in project settings to enable manifest tools (principle 40).
- Runs as non-root user `nrflo` (uid 65532) with `/data` as the `NRFLO_HOME` volume.
- Built with `CGO_ENABLED=0` and no `tray` build tag (uses `be/internal/cli/serve_notray.go`).

Make targets: `make docker-build` (single-arch local), `make docker-buildx` (multi-arch + push), `make docker-login`. Override `IMAGE_OWNER`, `IMAGE_NAME`, `IMAGE_TAG`, `PLATFORMS` on the command line. The `docker.yml` workflow auto-builds and pushes on every `v*` tag.

Logs are written to `~/.nrflo/logs/be.log` (or `$NRFLO_HOME/logs/be.log`).
