# Claude Code Instructions for nrflo

## Overview

nrflo is a multi-workflow state management system for ticket and project-level implementation with spawned AI agents. Supports multiple workflows per ticket, project-scoped workflows (no ticket required), parallel agents (Claude, OpenAI), and real-time WebSocket updates.

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

## Architecture Invariants

Rules every change must respect. (Mandatory Rules above cover layer execution, state-in-DB, the polymorphism rule, and the test/file-size budgets — not duplicated here.)

- **Server-only**: `nrflo_server serve` is the only user-facing command; all management goes through the web UI.
- **Two binaries**: `nrflo_server` (server) and `nrflo` (agent + ticket/deps CLI).
- **Single global SQLite DB**: `~/.nrflo/nrflo.data` (override with `NRFLO_HOME`); migrations auto-run on startup via golang-migrate with embedded SQL; pool is 10 max / 5 idle.
- **Project scope from env**: every CLI/API call resolves the project from `NRFLO_PROJECT` (or the `X-Project` header for HTTP).
- **Service layer**: business logic stays out of HTTP handlers and socket handlers — it lives in `be/internal/service/`.
- **Spawner is in-process**: it broadcasts WS events through the hub directly. No socket fallback for orchestration events.
- **WebSocket-only realtime**: the UI never polls; all live updates flow through `/api/v1/ws`.
- **Agents identify themselves via env**: the spawner sets `NRF_SESSION_ID` + `NRF_WORKFLOW_INSTANCE_ID` on every agent process; without them no socket call (findings, agent.*, skip) can resolve.
- **Spawned agents authenticate to the HTTP API via per-session bearer token**: the spawner mints a `spawn_token` per agent session, persists it on the `agent_sessions` row, and sets `NRFLO_AGENT_TOKEN` on the agent process env. The CLI's HTTPClient sends it as `Authorization: Bearer …`; `requireAuth` accepts it when the session's status is `running` or `user_interactive`, and enforces project scope against the `X-Project` header. Tokens are not admin-equivalent — `requireAdmin` routes always reject them. No SCS `Lifetime`/`IdleTimeout` cap applies; validity ends when the session reaches a terminal status.
- **Agent CLI is a small subset**: `agent fail/finished/continue/callback`, `findings *`, `project_findings *`, `skip`. Anything else goes through the HTTP API.
- **Clock abstraction for tests**: DB timestamps go through the `clock.Clock` interface (`internal/clock/`); tests use `clock.TestClock` with `Set()`/`Advance()` instead of `time.Sleep`.

## Feature Index

Where to look when working on a feature. Each entry is a one-line pointer; full detail lives in the linked downstream `CLAUDE.md`.

### Workflow execution
- **Layer-based concurrent execution + fan-in + agent callbacks** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Manual restart, retry-failed, server-side orchestration entry points** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Low-context relaunch** (resume vs system-agent path, `to_resume` finding, `${PREVIOUS_DATA}`) → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Stall detection / global stall timeouts / restart cap** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Take-control / resume-session / exit-interactive / PTY relay** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Interactive start & plan mode (L0 pre-launch)** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Endless loop mode (project-scoped re-spawn)** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Automatic merge conflict resolution / push-after-merge** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)

### Agents, templates, and configuration
- **Workflow definitions, agent definitions (layer-derived phases), system agents** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) + [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [agent_manual.md](agent_manual.md)
- **Default templates** (readonly seeded agents/injectables, restore endpoint) → [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Low consumption mode (per-agent model swap)** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **CLI models registry / supported models** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)

### Execution backends (`execution_mode`)
- **`api` — in-process Anthropic runner + gating + tool registry + sink** → [spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md)
- **`cli` interactive backend (Claude/Codex/Opencode in PTY) + idle/nudge loop** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **`script` — Python scriptBackend + capability matrix** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Manifest tools (api-mode only)** → [manifest/CLAUDE.md](be/internal/manifest/CLAUDE.md) + [spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md)
- **Python SDK + `script.context` socket method** → [sdk/python/CLAUDE.md](be/internal/sdk/python/CLAUDE.md) + [socket/CLAUDE.md](be/internal/socket/CLAUDE.md)

### Project-scoped & scheduled work
- **Project-scoped workflows (no worktrees, multi-instance)** → [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Scheduled tasks (cron → workflows + chains)** → [scheduler/CLAUDE.md](be/internal/scheduler/CLAUDE.md)
- **Workflow chains and chain runs** → [be/CLAUDE.md](be/CLAUDE.md) (chainrunner) + [api/CLAUDE.md](be/internal/api/CLAUDE.md) + [ui/CLAUDE.md](ui/CLAUDE.md)

### Auth & administration
- **Argon2id + SCS sessions, login rate limit, `--insecure-cookies`** → [auth/CLAUDE.md](be/internal/auth/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **`requireAuth` / `requireAdmin` route list, audit-log + user CRUD endpoints** → [api/CLAUDE.md](be/internal/api/CLAUDE.md)

### Storage & operations
- **Python scripts table + CRUD + validation** → [api/CLAUDE.md](be/internal/api/CLAUDE.md) + [service/CLAUDE.md](be/internal/service/CLAUDE.md)
- **Error tracking (`errors` table + `error.created` events)** → [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Notification channels (Slack/Telegram dispatch + retry)** → [be/CLAUDE.md](be/CLAUDE.md) (notify section)
- **DB schema, migrations, connection pool** → [db/CLAUDE.md](be/internal/db/CLAUDE.md)
- **Versioned config-file editor (manifest workflows)** → [configeditor/CLAUDE.md](be/internal/configeditor/CLAUDE.md)

### nrflo_server subcommands

| Command | Description |
|---------|-------------|
| `serve` | Start the HTTP/WebSocket server (default when no subcommand given) |
| `version` | Print version information |
| `init-customer` | Scaffold a starter customer config directory for manifest-driven api-mode workflows. See [be/internal/manifest/CLAUDE.md](be/internal/manifest/CLAUDE.md). |

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
