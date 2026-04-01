# Claude Code Instructions for nrworkflow

## Overview

nrworkflow is a multi-workflow state management system for ticket and project-level implementation with spawned AI agents. Supports multiple workflows per ticket, project-scoped workflows (no ticket required), parallel agents (Claude, OpenAI), and real-time WebSocket updates.

The server runs as `nrworkflow_server serve` and provides an HTTP API + WebSocket for the web UI, plus a Unix socket (and optional TCP socket for Docker agents) for agent communication. Spawned agents use the `nrworkflow` CLI binary (`agent fail/continue`, `findings add/append/get/delete`) to report results.

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

Update [agent_manual.md](agent_manual.md) when modifying:
- Template variables in `be/internal/spawner/template.go`
- Findings patterns in `be/internal/spawner/template_findings.go` or `template_project_findings.go`
- Agent CLI commands in `be/internal/cli/agent.go` or `be/internal/cli/findings.go`
- Agent definition schema in `be/internal/model/agent_definition.go`
- Workflow phase format or layer execution rules

### 2. Layer-Based Phase Execution

Agents are grouped by `layer` number. All agents in the same layer run concurrently; layers execute in ascending order. The spawner validates:
- All agents in prior layers are completed or skipped before the current layer starts
- Fan-in: if a layer has multiple agents, the next layer must have exactly 1 agent
- At least one agent in a layer must pass for the workflow to proceed (all-skipped continues)

### 3. State is Stored in Database Tables

Workflow runtime state is stored in normalized database tables:
- **`workflow_instances`** — one row per workflow run (multiple per ticket+workflow allowed), stores workflow-level findings, retry count
- **`agent_sessions`** — one row per agent execution, stores result, pid, findings, timestamps
- Phase statuses, phase order, and current phase are derived from `agent_sessions` + workflow definition at read time
- Active agents = `agent_sessions WHERE status = 'running'`
- Agent history = `agent_sessions WHERE status != 'running'`

### 4. CRITICAL: Backend Test Suite Must Run in Under 15 Seconds

The full backend test suite (`cd be && go test ./internal/... -count=1`) must complete in **≤15 seconds wall time**. This is a hard constraint enforced by the test script.

**Never introduce:**
- `time.Sleep` in tests — use `clock.TestClock.Advance()` for time-dependent logic, or poll with a tight loop+timeout for async conditions
- Unnecessary waits after hub `Register`/`Subscribe` — these are synchronous via mutex
- Sleeps waiting for log output — logging before goroutine launch is synchronous

**Patterns that are allowed:**
- `waitForCondition(t, 2*time.Second, 5*time.Millisecond, fn)` — tight polling with short timeout for genuinely async operations
- `clock.TestClock.Advance(d)` — advance fake clock instead of sleeping
- `env.Clock.Advance(d)` — in integration tests

If the test suite exceeds 15 seconds, **identify and eliminate the cause before merging**.

### 4b. CRITICAL: Frontend Test Suite Must Run in Under 15 Seconds

The full frontend test suite (`cd ui && npx vitest run`) must complete in **≤15 seconds wall time**. Enforced by `ui/scripts/test.sh` (mirrors the BE constraint). Pool is set to `threads` in `vitest.config.ts` for speed.

**Never introduce:**
- `setTimeout` in test bodies or mock implementations — use `new Promise(() => {})` (never-resolving) to keep a mutation in-flight for `isPending` tests
- Real delays inside mock API implementations — mocks should resolve immediately or stay pending

**Patterns that are allowed:**
- `vi.useFakeTimers()` + `vi.advanceTimersByTime(ms)` — for timer-dependent components
- `new Promise(() => {})` — keeps mutation `isPending: true` without any real delay
- `waitFor(() => expect(...))` — RTL polling for genuinely async React state

If the test suite exceeds 15 seconds, **identify and eliminate the cause before merging**.

### 5. Keep Source Files Under 300 Lines

Source files should be kept under 300 lines when possible. When a file grows beyond this limit, split it into logical sub-files. This applies to both code and documentation files.

### 5. Documentation Hierarchy

Root `CLAUDE.md` contains only project-level information (architecture principles, mandatory rules, CLI commands, workflows, brief summaries). Detailed implementation docs belong in subdirectory `CLAUDE.md` files:
- Backend package details → `be/internal/<package>/CLAUDE.md`
- Frontend module details → `ui/src/<module>/CLAUDE.md`
- DB schema, full API listings, spawner internals, etc. must NOT be duplicated in root — use cross-references instead.

## Key Files

| File | Purpose |
|------|---------|
| `be/` | Go backend source code (see [be/CLAUDE.md](be/CLAUDE.md)) |
| `ui/` | React web interface (see [ui/CLAUDE.md](ui/CLAUDE.md)) |
| `guidelines/agent-protocol.md` | Agent conventions |
| `nrworkflow.data` | SQLite database (tickets, projects, sessions) |
| `restart.sh` | Rebuild and restart BE + UI servers (background) |
| `stop.sh` | Stop running servers |
| `rebuild-cli.sh` | Rebuild and re-symlink CLI binary (also rebuilds Docker image if it exists) |
| `rebuild-docker.sh` | Nuclear-option Docker image rebuild: stops containers, removes image, rebuilds from scratch |
| `agent_manual.md` | User-facing agent definition guide (template vars, findings, CLI) |

## Architecture Principles

1. **Server-only**: `nrworkflow_server serve` is the only user-facing command; all management via web UI
2. **Agent CLI subset**: Spawned agents use `agent fail/continue`, `findings add/append/get/delete`, and `project_findings add/add-bulk/get/append/append-bulk/delete` via Unix socket
3. **Auto-migrate**: Database migrations run automatically on server startup
4. **Two Go binaries**: `nrworkflow_server` (serve command only) and `nrworkflow` (agent/findings/tickets/deps CLI)
5. **Project-scoped**: Project discovered from `NRWORKFLOW_PROJECT` env variable
6. **Single database**: `~/projects/2026/nrworkflow/nrworkflow.data` (SQLite, global for all projects)
7. **Connection Pool**: DB uses connection pooling (10 max, 5 idle)
8. **Versioned migrations**: Schema managed by golang-migrate with embedded SQL files in `db/migrations/`
9. **Service Layer**: Business logic separated from HTTP handlers and socket handlers
10. **Spawner in-process**: Spawner runs inside the server, broadcasts WebSocket events via direct hub (no socket fallback)
11. **Layer-based execution**: Phases grouped by layer; same-layer agents run concurrently, layers execute sequentially with fan-in (pass_count >= 1)
12. **WebSocket real-time**: UI receives all real-time updates via WebSocket (`/api/v1/ws`), no REST polling
13. **DB-stored workflow definitions**: Workflow definitions (phases) stored in `workflows` table, managed via `/api/v1/workflows` API
14. **DB-stored agent definitions**: Agent definitions (model, timeout, prompt template) stored in `agent_definitions` table, managed via `/api/v1/workflows/{wid}/agents` API. The spawner loads templates exclusively from DB. Templates support `${VAR}` variable substitution and `#{FINDINGS:agent}` / `#{PROJECT_FINDINGS:key}` pattern expansion. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for full template variable reference. See [agent_manual.md](agent_manual.md) for the complete agent definition cheat-sheet.
15. **Server-side orchestration**: Workflows run from the web UI via `POST /api/v1/tickets/:id/workflow/run`. The orchestrator groups phases by layer and runs all agents in each layer concurrently (one goroutine per agent calling `spawner.Spawn()`), with cancellation support via `/workflow/stop`.
16. **Low-context relaunch**: When an agent's context drops below threshold (default ~25% remaining, configurable per agent via `restart_threshold` in agent_definitions), the spawner kills the agent, resumes with `claude --resume` instructing it to save progress under the `to_resume` findings key, then spawns a fresh agent with `${PREVIOUS_DATA}` populated from that `to_resume` key. Old sessions get `status='continued'` and are excluded from agent history.
17. **Manual agent restart**: Users can trigger an agent restart from the UI via `POST /api/v1/tickets/:id/workflow/restart` with `{workflow, session_id}`. This triggers the same context-save-and-relaunch flow as the automatic low-context restart, regardless of current token usage.
18. **Retry failed agent**: Users can retry a failed workflow from the failed layer via `POST /api/v1/tickets/:id/workflow/retry-failed` (or `/api/v1/projects/:id/workflow/retry-failed`) with `{workflow, session_id}`. This resets the workflow instance to active, resets phases in the failed layer to pending, increments retry_count, and re-runs the orchestration starting from the failed layer.
19. **Project-scoped workflows**: Workflows can have `scope_type` of `ticket` (default) or `project`. Project-scoped workflows run at project level without requiring a ticket and do not use git worktrees (always run in the original project root). Multiple concurrent instances of the same project workflow are allowed (each gets a unique instance_id). API: `POST /api/v1/projects/:id/workflow/run`, `GET /api/v1/projects/:id/workflow` (returns `all_workflows` keyed by instance_id), `GET /api/v1/projects/:id/agents`. Stop/restart/retry-failed accept optional `instance_id` to target a specific instance. Project agents cannot use `${TICKET_ID}`, `${TICKET_TITLE}`, or `${TICKET_DESCRIPTION}` template variables.
20. **Agent callbacks**: A later-layer agent (e.g., qa-verifier) can trigger a callback to re-run an earlier layer (e.g., implementor). The orchestrator saves `_callback` metadata (instructions, from_agent, level) to workflow instance findings, resets phases/sessions from the target layer forward, and jumps the execution loop back. The target agent's prompt can include `${CALLBACK_INSTRUCTIONS}` to receive the callback instructions. After the callback target layer completes successfully, `_callback` is cleared from findings. Max 3 callbacks per workflow run.
21. **Clock abstraction**: All DB timestamp generation uses a `clock.Clock` interface (`internal/clock/`). Production uses `clock.Real()` (wall clock). Tests use `clock.TestClock` with `Set()`/`Advance()` for deterministic time control, eliminating `time.Sleep` for timestamp ordering.
22. **Take-control (interactive session)**: Users can take interactive control of a running Claude agent via `POST /api/v1/tickets/:id/workflow/take-control` (or `/api/v1/projects/:id/workflow/take-control`) with `{workflow, session_id}`. This kills the running agent, sets session status to `user_interactive`, and returns the session ID for `claude --resume`. The spawner blocks (does not advance to next phase) until `POST .../exit-interactive` with `{workflow, session_id}` is called, which marks the session `interactive_completed` with `result=pass` and unblocks the spawner. Only works for Claude CLI agents (`SupportsResume() == true`). For finished sessions (completed/failed/timeout), use `POST .../resume-session` with `{session_id}` instead — this sets the session to `user_interactive` directly without requiring a running orchestration, then reuses the same PTY handler and exit flow.
23. **PTY WebSocket endpoint**: `GET /api/v1/pty/{session_id}` upgrades to a 1:1 WebSocket and spawns `claude --resume <session_id>` in a pseudo-terminal. Relays stdin/stdout bidirectionally between browser and PTY. Handles terminal resize via JSON `{"type":"resize","rows":N,"cols":N}` text messages. On process exit, triggers exit-interactive flow (status → `interactive_completed`, unblocks spawner). Separate from the broadcast WS at `/api/v1/ws`. Session must be in `user_interactive` status.
24. **Stall detection and auto-restart**: The spawner monitors time since last agent message. If an agent produces no output within `stall_start_timeout_sec` (default 120s) or stops producing output for `stall_running_timeout_sec` (default 480s), it is killed and relaunched via the continuation mechanism after a 15s delay. No context save is attempted (agent is frozen). Stall restarts are capped at 6 per agent. Timeouts are configurable per agent_definition (NULL = defaults, 0 = disabled). `agent.stall_restart` and `agent.stall_waiting` WS events are broadcast. Additionally, **instant stall detection** catches Claude agents that exit with status 0 in under 1 minute with <=3 actionable messages (excluding `[init]` and `[thinking]` which are auto-generated) — these are overridden to continued/instant_stall and relaunched after a 15s delay, sharing the same stall restart budget. Agents that deliberately have no work should run `nrworkflow findings add no-op:no-op` before exiting — the `no-op` finding suppresses instant stall detection. Every agent prompt includes this instruction automatically. When the stall budget is exhausted and an instant stall is detected, the session is marked as failed (`stall_budget_exhausted`) instead of passing. `agent.instant_stall_restart` WS event is broadcast. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for details.
25. **Interactive start & plan mode**: The workflow run API accepts optional `interactive` and `plan_mode` boolean flags. When `interactive: true`, the orchestrator spawns only the L0 agent in interactive mode and returns its `session_id` in `RunWorkflowResponse`. When `plan_mode: true`, a planner agent is spawned interactively. The web UI `RunWorkflowDialog` and `RunWorkflowForm` expose these as mutually exclusive checkboxes; on success the `onInteractiveStart` callback opens the PTY terminal via `AgentTerminalDialog`.
26. **Spawner env vars for direct targeting**: The spawner sets `NRWF_WORKFLOW_INSTANCE_ID` and `NRWF_SESSION_ID` env vars on every spawned agent. The CLI reads these and passes them in all socket requests (`instance_id`, `session_id` fields). The service uses them directly — no ambiguous DB lookup by `(project, ticket, workflow)`. This is required; agents without these env vars cannot use findings or agent commands. Docker agents also get `NRWORKFLOW_AGENT_HOST=host.docker.internal:<port>` to connect via TCP instead of Unix socket. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for the full env var list.
27. **System agent definitions**: Global agent definitions not tied to any project or workflow, stored in `system_agent_definitions` table. Managed via `/api/v1/system-agents` CRUD endpoints (no `X-Project` header required). Used for system-level agents like conflict-resolver.
28. **Automatic merge conflict resolution**: When `MergeAndCleanup()` fails after workflow completion, the orchestrator attempts automatic resolution by spawning a `conflict-resolver` system agent. The agent receives `${BRANCH_NAME}`, `${DEFAULT_BRANCH}`, and `${MERGE_ERROR}` via `ExtraVars`. On success, the feature branch is deleted; on failure or missing resolver, falls through to manual resolution. See [be/internal/orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) for details.
29. **Low consumption mode**: Global setting (`GET/PATCH /api/v1/settings`) stored in the `config` table. When enabled, the spawner overrides the model for agents that have a `low_consumption_model` configured in their agent definition (e.g., `"sonnet"`, `"haiku"`). Only the model is swapped — the agent's own prompt template, timeout, and settings are kept. Session records (agent_type, phase tracking) retain the original agent type. The setting is read once at workflow start — toggling mid-workflow has no effect until the next run. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for details.
30. **Global stall timeouts**: Global settings `stall_start_timeout_sec` and `stall_running_timeout_sec` override the hardcoded stall detection defaults (120s and 480s) when agent definitions don't specify per-agent values. Priority: per-agent def > global config > hardcoded default. Settings are read once at workflow start. `0` = disabled, positive integer = custom seconds, null/empty = use hardcoded default. Managed via `GET/PATCH /api/v1/settings`.
31. **Default templates**: Global agent prompt templates stored in `default_templates` table. Six readonly templates are seeded by migration (setup-analyzer, test-writer, implementor, qa-verifier, doc-updater, ticket-creator). Users can create additional non-readonly templates. Managed via `/api/v1/default-templates` CRUD endpoints (no `X-Project` header required). Readonly templates cannot be updated or deleted via API.

## Quick Start

Web UI: `./restart.sh` then open `http://localhost:5175`

## Agent CLI Commands

Spawned agents use these commands to report results (via Unix socket to the server). Exit 0 = pass (no explicit call needed). Only call `agent fail` for explicit failure.

```bash
# All context derived from NRWF_SESSION_ID + NRWF_WORKFLOW_INSTANCE_ID env vars (set by spawner)
nrworkflow agent fail [--reason <text>]
nrworkflow agent continue
nrworkflow agent callback --level <N>

nrworkflow skip <tag>  # Add skip tag to running workflow instance (reads NRWF_WORKFLOW_INSTANCE_ID from env)

# Own-session findings (write to current agent's session)
nrworkflow findings add <key> <value>
nrworkflow findings add key1:val1 [key2:val2...]
nrworkflow findings append <key> <value>
nrworkflow findings append key1:val1 [key2:val2...]
nrworkflow findings get [key] [-k <key>...]
nrworkflow findings delete <key1> [key2...]

# Cross-agent read (provide target agent-type; uses NRWF_WORKFLOW_INSTANCE_ID to scope)
nrworkflow findings get <agent-type> [key] [-k <key>...]

# Project-level findings (scoped to NRWORKFLOW_PROJECT)
nrworkflow findings project-add <key> <value>
nrworkflow findings project-add key1:val1 [key2:val2...]
nrworkflow findings project-get [key] [-k <key>...]
nrworkflow findings project-append <key> <value>
nrworkflow findings project-append key1:val1 [key2:val2...]
nrworkflow findings project-delete <key1> [key2...]
```

## Ticket CLI Commands

Manage tickets and dependencies via the HTTP API (requires server running):

```bash
nrworkflow tickets list [--status <status>] [--type <type>] [--parent <id>] [--json]
nrworkflow tickets get <id> [--json]
nrworkflow tickets create --title <title> [--id <id>] [--description <text>] [--type <type>] [--priority <1-4>] [--parent <id>] [--created-by <name>] [--json]
nrworkflow tickets update <id> [--title <title>] [--description <text>] [--type <type>] [--priority <1-4>] [--parent <id>]
nrworkflow tickets close <id> [--reason <text>]
nrworkflow tickets reopen <id>
nrworkflow tickets delete <id>

nrworkflow deps list <ticket-id> [--json]
nrworkflow deps add <ticket-id> <blocker-id>
nrworkflow deps remove <ticket-id> <blocker-id>
```

All ticket/deps commands use `--server` (default `NRWORKFLOW_API_URL` or `http://localhost:6587`) and require `NRWORKFLOW_PROJECT` env variable.

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

Each phase entry is a JSON object: `{"agent": "setup-analyzer", "layer": 0}`. The `layer` (integer >= 0) and `agent` (agent definition ID) fields are required. String-only entries and the `parallel` field are rejected. Supported models are loaded from the `cli_models` DB table (seeded with `opus`, `opus_1m`, `sonnet`, `haiku`, `opencode_gpt_normal`, `opencode_gpt_high`, `codex_gpt_normal`, `codex_gpt_high`, `codex_gpt54_normal`, `codex_gpt54_high`). Custom models can be added via `POST /api/v1/cli-models` and are immediately valid for agent definitions. See [be/internal/spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) for model mapping details.

## State Storage

Workflow runtime state is stored in two main tables: `workflow_instances` (one row per workflow run, stores findings, retry count) and `agent_sessions` (one row per agent execution, stores result, pid, findings, context usage, timestamps). Phase statuses, phase order, and current phase are derived at read time from `agent_sessions` rows + workflow definition. Both ticket-scoped and project-scoped workflows allow multiple instances; concurrent runs of the same ticket+workflow are prevented by the orchestrator's `IsRunning` check (not DB constraint). Completion statistics (`completed_at`, `total_duration_sec`, `total_tokens_used`) are computed from agent session data. See [be/internal/db/CLAUDE.md](be/internal/db/CLAUDE.md) for full schema.

## API Response Format

`GET /api/v1/tickets/:id/workflow` returns a v4 format wrapper with `ticket_id`, `has_workflow`, `workflows` list (deduplicated workflow names), and `all_workflows` map keyed by instance_id (same pattern as project workflows). Each workflow state includes version, status, instance_id, workflow name, current phase, phase order, phase layers, phases map, active agents, agent history, and findings. A separate `workflow_findings` field contains workflow-level findings (from `workflow_instances.findings`) filtered to exclude internal keys (starting with `_`); omitted when empty. When any agent writes a `workflow_final_result` finding, the last-written value (by session `ended_at`) is included as a top-level `workflow_final_result` field. Supports `?instance_id=` and `?workflow=` query params for selection. See [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md) for full endpoint listing and [be/internal/service/CLAUDE.md](be/internal/service/CLAUDE.md) for response construction.

## Chain Execution

Chains allow sequential execution of multiple tickets with a single workflow. Tickets are expanded with transitive dependency blockers, topologically sorted, and locked to prevent overlapping runs. Items execute one at a time via the orchestrator. Zombie running chains are marked failed on server startup (crash recovery). See [be/internal/orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) for chain runner details and [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md) for chain API endpoints.

## Server Scripts

| Script | Purpose |
|--------|---------|
| `restart.sh` | Kill existing servers, rebuild both binaries (`make build`), start `nrworkflow_server` + UI in background |
| `stop.sh` | Stop running BE + UI servers |
| `rebuild-cli.sh` | Rebuild and re-symlink the CLI binary (also rebuilds Docker image if it exists) |
| `rebuild-docker.sh` | Nuclear-option Docker image rebuild: stops containers, removes image, rebuilds from scratch |
| `ui/start-server.sh` | Start both servers in foreground (uses `nrworkflow_server serve`) |

Logs are written to `/tmp/nrworkflow/logs/be.log` and `/tmp/nrworkflow/logs/fe.log` when using `restart.sh`.
