# Claude Code Instructions for nrflo Backend

Go backend for nrflo. Two binaries: `nrflo_server` (server) and `nrflo` (CLI). The server provides HTTP API + WebSocket for the web UI, plus a Unix socket for agent communication. The CLI binary exposes agent commands (`agent fail/continue/callback`), findings commands (`findings add/append/get/delete`), and ticket/deps management.

## Project Structure

```
be/
├── cmd/nrflo/main.go       # CLI binary entry point (agent, findings, tickets, deps)
├── cmd/server/main.go           # Server binary entry point (serve)
├── internal/
│   ├── cli/                     # Cobra commands
│   │   ├── root.go              # Root command, global flags, project discovery
│   │   ├── serve.go             # HTTP API server (auto-migrates DB)
│   │   ├── agent.go             # agent fail/continue/callback/chain-next-instructions/chain-next-ticket (context from NRF_SESSION_ID + NRF_WORKFLOW_INSTANCE_ID env vars)
│   │   ├── chain.go             # agent chain-next-instructions and chain-next-ticket subcommands
│   │   ├── findings.go          # findings add/append/get/delete (own-session writes; cross-agent reads via agent-type arg)
│   │   ├── findings_project.go  # project-level findings (project-add/get/append/delete)
│   │   ├── skip.go              # skip <tag> command (adds skip tag to running workflow instance)
│   │   ├── tickets.go           # tickets list/get/create (HTTP)
│   │   ├── tickets_update.go    # tickets update/close/reopen/delete (HTTP)
│   │   └── deps.go              # deps list/add/remove (HTTP)
│   ├── spawner/                 # Agent spawner
│   │   ├── spawner.go           # Spawn and monitor agents
│   │   ├── cli_adapter.go       # CLIAdapter interface, types (SpawnOptions, InteractiveSpawnOptions, ResumeOptions), factory
│   │   ├── cli_adapter_claude.go    # ClaudeAdapter implementation
│   │   ├── cli_adapter_opencode.go  # OpencodeAdapter implementation
│   │   ├── cli_adapter_codex.go     # CodexAdapter implementation
│   │   ├── cli_adapter_test.go  # Adapter tests
│   │   ├── errors.go            # Typed errors (CallbackError for layer re-execution, detected by orchestrator)
│   │   ├── completion.go        # Completion handling, continuation relaunch
│   │   ├── context_save.go      # Low-context save: kill, branch to agent or resume path, relaunch
│   │   ├── context_save_resume.go # Resume-based context save (Claude CLI only, default path)
│   │   ├── context.go           # Context tracking (reads context_left from DB)
│   │   ├── database.go          # DB operations: register start/stop, phase management
│   │   ├── output.go            # Output monitoring, message formatting
│   │   ├── template.go          # Template loading, variable expansion
│   │   └── template_findings.go # Findings expansion, ${PREVIOUS_DATA}, formatting
│   ├── scheduler/               # Cron-driven scheduled task runner (robfig/cron/v3)
│   │   ├── scheduler.go         # New/Start/Reload/Stop/RunNow lifecycle
│   │   ├── scheduler_dispatch.go # dispatch(): fan-out per workflow, update run row, broadcast
│   │   └── CLAUDE.md            # Package documentation
│   ├── orchestrator/            # Server-side workflow orchestration
│   │   ├── orchestrator.go      # Run workflows from UI (layer-grouped concurrent phases)
│   │   ├── orchestrator_interactive.go # Interactive start & plan-before-execute pre-step
│   │   ├── plan_reader.go       # Plan file reader for plan-before-execute mode
│   │   └── chain_runner.go      # Sequential chain execution runner (old chain_executions system)
│   ├── chainrunner/             # Workflow chain run execution engine (workflow_chain_runs system)
│   │   ├── chainrunner.go       # Runner struct, Start/Cancel/IsRunning/WaitAll, pollInstance
│   │   ├── loop.go              # runLoop, executeStep, cancelRun, failRun
│   │   └── recovery.go         # RecoverZombieRuns (crash recovery on startup)
│   ├── api/                     # HTTP API
│   │   ├── server.go            # Server setup, CORS, WebSocket hub, orchestrator, PTY manager
│   │   ├── handlers_tickets.go  # Ticket list/create/get endpoints
│   │   ├── handlers_tickets_update.go # Ticket update/delete/close/reopen endpoints
│   │   ├── handlers_workflow.go # Workflow state endpoints
│   │   ├── handlers_orchestrate.go # Ticket-scoped orchestration run/stop/restart endpoints
│   │   ├── handlers_project_workflow.go # Project-scoped workflow run/stop/restart/delete/state
│   │   ├── handlers_workflow_def.go # Workflow definition endpoints
│   │   ├── handlers_agent_def.go # Agent definition endpoints
│   │   ├── handlers_system_agent_def.go # System agent definition CRUD (global)
│   │   ├── handlers_default_template.go # Default template CRUD (global)
│   │   ├── handlers_python_scripts.go # Python script CRUD + validate (project-scoped; writes admin-only)
│   │   ├── handlers_cli_models.go # CLI model CRUD (global)
│   │   ├── handlers_global_settings.go # Global settings GET/PATCH (no project scope)
│   │   ├── handlers_safety_hook_check.go # Safety hook dry-run check (POST /api/v1/safety-hook/check, global)
│   │   ├── handlers_pty.go      # PTY WebSocket handler (1:1 interactive terminal relay)
│   │   ├── handlers_chains.go   # Chain execution list/get/create/update/start/cancel/append/remove-items + run-epic
│   │   ├── handlers_workflow_chains.go # Workflow chain definition CRUD + step append/update/delete/reorder (project-scoped; admin writes)
│   │   └── handlers_workflow_chain_runs.go # Workflow chain run lifecycle: start/cancel/list/get (project-scoped)
│   │   ├── handlers_git.go        # Git commit history endpoints
│   │   ├── handlers_daily_stats.go # Daily stats endpoint
│   │   ├── handlers_errors.go     # Error log list endpoint (paginated)
│   │   ├── handlers_notification_channels.go # Notification channel CRUD + /test + deliveries list
│   │   └── handlers_logs.go       # Backend log file viewer
│   ├── ws/                      # WebSocket support (protocol v2)
│   │   ├── hub.go               # Client management, event log integration, broadcasting
│   │   ├── client.go            # Connection handling, subscriptions, cursor support
│   │   ├── handler.go           # HTTP upgrade handler
│   │   ├── protocol.go          # Protocol v2 constants, entity types, global event types
│   │   ├── replay.go            # Cursor-based replay from event log
│   │   ├── snapshot.go          # Snapshot streaming (begin/chunk/end)
│   │   ├── backpressure.go      # Client queue depth monitoring
│   │   └── testing.go           # Test helpers (NewTestClient)
│   ├── pty/                     # PTY session management for interactive agent control
│   │   ├── session.go           # Session: spawn arbitrary command in PTY (read/write, resize, close, ExitCode)
│   │   └── manager.go           # Manager: create/get/remove/close-all sessions; RegisterCommand for custom commands
│   ├── config/                  # Configuration management
│   │   └── config.go
│   ├── client/                  # Socket + HTTP clients
│   │   ├── client.go            # Unix socket client for agents
│   │   ├── http.go              # HTTP client for ticket/deps CLI commands
│   │   └── output.go            # Output formatting
│   ├── static/                  # Embedded UI assets (//go:embed)
│   │   ├── embed.go             # Embed directive and DistFS() accessor
│   │   ├── agent_manual.md      # Build artifact: gitignored, auto-copied from repo-root agent_manual.md by the `embed-assets` Make target (a prereq of every `make build*`/`make test*`). Do NOT edit, commit, or hand-copy — edit the root file and let make do the copy.
│   │   └── dist/                # UI build output (populated by `make build-ui`)
│   ├── socket/                  # Unix socket server
│   │   ├── server.go            # Socket listener, Handler struct (stores pool+clk for repo construction)
│   │   ├── handler.go           # Request routing (findings/project_findings/agent/workflow/ws/script)
│   │   ├── handler_script_context.go # script.context — resolves session→wfi→ticket, returns 12-key dict
│   │   └── protocol.go          # JSON-RPC protocol types
│   ├── notify/                  # Notification dispatch subsystem
│   │   ├── notify.go            # Dispatcher (ws.Listener): filters 5 events, inserts delivery rows
│   │   ├── transport.go         # Transport interface, registry, shared http.Client
│   │   ├── transport_slack.go   # Slack webhook transport (init registers)
│   │   ├── transport_telegram.go # Telegram Bot API transport (init registers)
│   │   ├── queue.go             # Worker: drain queue, exponential backoff, WS events
│   │   └── payload.go           # renderSlack/renderTelegram per event type
│   ├── service/                 # Business logic layer
│   │   ├── python_script.go     # PythonScriptService: Create/Get/List/Update/Delete (project-scoped)
│   │   ├── python_script_validate.go # PythonScriptValidator: syntax check via python3 -c (injectable lookPath/cmdFactory)
│   │   ├── project.go           # Project operations
│   │   ├── ticket.go            # Ticket operations
│   │   ├── workflow.go          # Workflow operations (ticket + project scope)
│   │   ├── workflow_defs.go     # Workflow definitions CRUD (phases derived from agent_definitions)
│   │   ├── workflow_config.go   # Workflow config loading (phases built from agent_definitions layer field)
│   │   ├── workflow_types.go    # Workflow type definitions (WorkflowDef, PhaseDef)
│   │   ├── workflow_validation.go # Validation (layer, fan-in, project scope)
│   │   ├── workflow_response.go # V4 response building (active agents, history)
│   │   ├── workflow_restart_details.go # Restart detail loading (duration, context, message count)
│   │   ├── agent.go             # Agent operations
│   │   ├── agent_definition.go  # Agent definition CRUD
│   │   ├── system_agent_definition.go # System agent definition CRUD (global)
│   │   ├── default_template.go  # Default template CRUD (global, readonly enforcement)
│   │   ├── cli_model.go         # CLI model CRUD (global, readonly delete enforcement)
│   │   ├── global_settings.go   # Global and project-scoped settings (wraps pool.GetConfig/SetConfig/GetProjectConfig/SetProjectConfig)
│   │   ├── error_service.go     # Error tracking (RecordError + ListErrors)
│   │   ├── notification.go      # Notification channel CRUD + masking + TestSend
│   │   ├── findings.go          # Findings operations
│   │   ├── chain.go             # Chain build, dependency expansion, topo sort
│   │   ├── chain_append.go      # AppendToChain for running chains
│   │   ├── chain_remove.go     # RemoveFromChain for running chains
│   │   ├── daily_stats.go       # Daily stats computation from source tables
│   │   ├── git.go               # Git operations (commit listing, detail via os/exec)
│   │   ├── workflow_chain.go    # WorkflowChainService: chain+step CRUD, validation (dense positions, step 0 project-scope, workflow_name resolves)
│   │   ├── workflow_chain_run.go # WorkflowChainRunService: CreateRun, CancelRun, ListRuns, GetRunDetail, SetNextStepInstructions, SetNextStepTicket
│   │   └── snapshot.go          # WS snapshot provider (builds chunks from workflow state)
│   ├── db/                      # Database layer
│   │   ├── db.go                # SQLite connection
│   │   ├── pool.go              # Connection pool (10 max, 5 idle)
│   │   ├── migrate.go           # Migration runner
│   │   └── migrations/          # SQL files (embedded via //go:embed)
│   │       └── embed.go         # Go embed directive
│   ├── model/                   # Data models
│   │   ├── python_script.go     # PythonScript struct (id, project_id, name, description, code, timestamps)
│   │   ├── project.go
│   │   ├── ticket.go
│   │   ├── agent_session.go
│   │   ├── agent_message.go
│   │   ├── agent_definition.go
│   │   ├── system_agent_definition.go
│   │   ├── default_template.go
│   │   ├── cli_model.go
│   │   ├── workflow.go
│   │   ├── workflow_instance.go
│   │   ├── chain.go             # Chain execution, item, lock models
│   │   ├── workflow_chain.go    # WorkflowChain, WorkflowChainStep, WorkflowChainRun, WorkflowChainRunStep models
│   │   ├── error_log.go         # ErrorLog struct + ErrorType enum
│   │   ├── daily_stats.go
│   │   ├── scheduled_task.go    # ScheduledTask + ScheduleRun + ScheduleRunWorkflow models
│   │   ├── user.go              # User struct (id, email, role, status, must_change_password, timestamps)
│   │   ├── audit.go             # AuditEntry struct + AuditFilter
│   │   ├── review_item.go       # ReviewItem struct + status constants (pending|approved|rejected)
│   │   ├── tool_dispatch.go     # ToolDispatch + DispatchSummary/EditRateRow/ThroughputPoint aggregates
│   │   └── config_version.go    # ConfigVersion struct
│   ├── sdk/                     # Embedded agent SDKs installed to $NRFLO_HOME/sdk/ on server startup
│   │   └── python/              # Python SDK package (package pythonsdk)
│   │       ├── nrflo_sdk.py     # Single-file Python SDK (pure stdlib, persistent socket)
│   │       └── embed.go         # //go:embed nrflo_sdk.py + WriteSDK(dir) installer
│   ├── manifest/                # Manifest parsing, python runtime, scaffolder (see internal/manifest/CLAUDE.md)
│   │   ├── config/              # Manifest parsing, tool validation, JSON Schema compilation
│   │   ├── python/              # Python script execution runtime (Runner, OSRunner, env scoping)
│   │   └── scaffold/            # init-customer scaffolder (embedded template tree)
│   ├── configeditor/            # Versioned config file editing service (DB-backed) (see internal/configeditor/CLAUDE.md)
│   │   └── migrate/             # Forward-only config migration runner
│   │       └── migrations/      # Migration implementations
│   ├── repo/                    # Repository pattern
│   │   ├── python_script.go     # PythonScriptRepo: Create/Get/List/Update/Delete (project+id scoped, clock-driven timestamps)
│   │   ├── project.go
│   │   ├── ticket.go
│   │   ├── dependency.go
│   │   ├── agent_session.go
│   │   ├── agent_message.go
│   │   ├── agent_definition.go
│   │   ├── workflow.go
│   │   ├── workflow_instance.go
│   │   ├── chain.go             # Chain execution CRUD
│   │   ├── chain_items.go       # Chain item operations (GetMaxPosition, GetTicketIDsByChain)
│   │   ├── chain_locks.go       # Chain lock operations
│   │   ├── workflow_chain.go    # WorkflowChainRepo (chain CRUD) + WorkflowChainStepRepo (step CRUD, BulkReorder)
│   │   ├── workflow_chain_run.go # WorkflowChainRunRepo (run lifecycle, MaterializeRunSteps, GetNextPendingStep, GetActiveRuns, SetRunStepInstance)
│   │   ├── workflow_chain_run_step.go # GetRunStepByInstanceID, SetNextPendingStepInstructions, SetNextPendingStepTicket, ListRunSteps
│   │   ├── error_log.go         # Error log CRUD (Insert, List, Count)
│   │   ├── daily_stats.go
│   │   ├── event_log.go         # WS event log persistence (append, query, cleanup)
│   │   ├── scheduled_task.go    # ScheduledTask CRUD + ListEnabled + UpdateTriggerTimestamps
│   │   ├── schedule_run.go      # ScheduleRun Insert/UpdateStatus/ListByTask/Get
│   │   ├── review.go            # ReviewRepo: Insert/Get/List/UpdateDraft/Approve/Reject
│   │   ├── tool_dispatch.go     # DispatchRepo: Insert/ListSummary/EditRateByTool/Throughput
│   │   ├── config_version.go    # ConfigVersionRepo: Insert (tx, auto-version)/LatestVersion/Get/History
│   │   ├── user_repo.go         # UserRepo: Get/GetByEmail/List/Create/UpdateProfile/UpdatePassword/UpdateLastLogin/CountActiveAdmins/Delete
│   │   └── audit_repo.go        # AuditRepo: Append/List (with AuditFilter + pagination + total count)
│   ├── types/                   # Shared request/response types
│   │   ├── request.go
│   │   ├── python_script.go     # PythonScriptCreateRequest, PythonScriptUpdateRequest, ValidatePythonScriptRequest
│   │   ├── chain_request.go     # Chain create/update request types
│   │   └── scheduled_task_request.go # ScheduledTaskCreate/UpdateRequest types
│   ├── clock/                   # Time abstraction for testability
│   │   ├── clock.go             # Clock interface + Real() (production wall clock)
│   │   └── test.go              # TestClock with Set()/Advance() for deterministic tests
│   ├── integration/             # Integration tests
│   │   ├── testenv.go           # NewTestEnv shared harness
│   │   └── testdata/            # Test config, agent templates
│   ├── logger/                  # Structured logging with trx propagation and size-based rotation
│   │   └── logger.go            # Init, Info/Warn/Error, NewTrx, WithTrx/TrxFromContext, rotate (10MB). HTTP requests get trx injected via requestIDMiddleware
│   └── id/                      # ID generation
│       └── generator.go
├── go.mod
└── go.sum
```

## Source File Size Limit

Keep source files under 300 lines. If a newly created or modified file exceeds 300 lines, refactor it by splitting into logical sub-files before committing. This applies to all Go source files (`.go`), test files, and migration scripts.

## Dependencies

- Go 1.25+
- github.com/spf13/cobra - CLI framework
- modernc.org/sqlite - Pure Go SQLite (no CGO)
- github.com/google/uuid - UUID generation
- github.com/gorilla/websocket - WebSocket implementation
- github.com/creack/pty - PTY allocation for interactive agent sessions
- github.com/golang-migrate/migrate - Database migrations

## Building from Source

All build targets are in the **root** `Makefile` (not `be/`):

```bash
cd ~/projects/2026/nrflo
make build                # Build both binaries (CLI + server, includes UI)
make build-cli            # Build CLI binary (nrflo)
make build-server         # Build server binary with embedded UI
make build-server-only    # Go-only rebuild (skip UI build)
make build-ui             # Build UI and copy dist to embed directory
make build-release        # Optimized release build (both binaries)
make install              # Install to PREFIX (default /usr/local)
make clean                # Clean build artifacts
make help                 # Show all targets
```

No CGO required (pure Go SQLite via modernc.org/sqlite).

## Server Architecture

`nrflo_server serve` provides:
- **HTTP API** on `127.0.0.1:6587` by default — web UI, REST API, WebSocket. Use `--host 0.0.0.0` for LAN access
- **Unix socket** at `/tmp/nrflo/nrflo.sock` — agent communication only (findings, agent completion, ws.broadcast)
- **Auto-migration** — database schema is automatically migrated on startup

### Serve Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--host` | `127.0.0.1` | Host/IP to bind to |
| `--port` | `6587` | HTTP port |
| `--no-tray` | `false` | Disable macOS menu bar tray icon |
| `--mode` | `cli` | Execution mode: `cli` (default) or `api`. Set to `api` to enable in-process Anthropic execution (execution_mode='api' agent definitions, tool-definitions and api-credentials endpoints). In `cli` mode those routes return 404 and creating/updating api-mode agent defs returns HTTP 400 `api_mode_disabled`. The mode touches: `handlers_agent_def.go`, `handlers_tool_definitions.go`, `handlers_api_credentials.go`, `handlers_global_settings.go` (`api_mode_enabled` field), and `spawner.Config.APIMode`. |
| `--insecure-cookies` | `false` | Disable `Secure` flag on `nrflo_session` cookie. Use for local HTTP dev without TLS. Passed as `dev=true` to `auth.NewManager`. |

## Authentication

HTTP API routes are protected by SCS cookie-based sessions (sqlite3store, `nrflo_session` cookie). The handler chain is:

```
cors → requestID → projectMiddleware → LoadAndSave (for /api/* only) → mux
```

Per-route auth: `protected` (requireAuth) or `admin` (requireAdmin = admin role). Public: `POST /api/v1/auth/login` only.

`requireAuth` also accepts `Authorization: Bearer <agent_token>` for spawned-agent calls. The token is the `agent_sessions.spawn_token` minted by the spawner and exposed to the agent process via `NRFLO_AGENT_TOKEN`; it is valid only while the row's status is `running` or `user_interactive`. The middleware also requires the `X-Project` header (when present) to match the session's project. Bearer-authenticated requests do **not** populate the user context, so `requireAdmin` always 403s for them.

Admin-gated writes: `POST /projects`, `DELETE /projects/{id}`, all `/users` endpoints, `GET /audit-log`, system-agents writes, cli-models writes, default-templates writes, scheduled-tasks writes, tool-definitions writes (api-mode), api-credentials writes (api-mode), `PATCH /settings`.

Login rate limiter: 5 attempts per 5 min per IP+email key, returns HTTP 429 with `Retry-After`.

Default seeded admin: `admin` / `admin`, `must_change_password=0` (migration 000078). See [be/internal/auth/CLAUDE.md](internal/auth/CLAUDE.md).

The socket uses a JSON-RPC style protocol (line-delimited JSON). Only `findings.*` (add, add-bulk, get, append, append-bulk, delete), `project_findings.*` (add, add-bulk, get, append, append-bulk, delete), `agent.fail/continue/callback/context_update`, `workflow.skip`, and `ws.broadcast` methods are supported.

### Per-Project Settings (config table, `PATCH /api/v1/projects/:id`)

| Key | Type | Description |
|-----|------|-------------|
| `claude_safety_hook` | string (JSON) | Safety hook config — blocks dangerous commands via `--settings` |
| `push_after_merge` | bool | Push default branch to origin after successful worktree merge |
| `interactive_cli_mode` | bool | Enable interactive terminal mode for CLI agents (consumed by T3) |
| `customer_config_dir` | string (abs path) | Absolute path to an existing directory containing customer config files; validated on PATCH (must be absolute, must exist, must be a directory) |

## Package Documentation

Detailed documentation for each major package is in its own CLAUDE.md:

| Package | Documentation | Key Content |
|---------|--------------|-------------|
| `internal/scheduler/` | [scheduler/CLAUDE.md](internal/scheduler/CLAUDE.md) | Cron scheduler: lifecycle, dispatch flow, integration with orchestrator |
| `internal/notify/` | (inline docs) | Notification subsystem: Dispatcher (ws.Listener), Slack/Telegram transports, async retry queue with backoff 15s/60s/300s, secret masking, error tracking on giving_up |
| `internal/spawner/` | [spawner/CLAUDE.md](internal/spawner/CLAUDE.md) | CLI adapters, spawn flow, template variables, findings auto-population, output format. T1 introduces an `ExecutionBackend` seam (`backend.go`). T2 added the provider abstraction + Anthropic streaming impl. T3 wires `apirun.Runner` and `apiBackend` into the seam for text-only API-mode execution; tools/continuation arrive in T4-T5. `Config` adds `DispatchRepo`, `ReviewRepo`, `PythonRunner`, `CustomerConfigDir` for manifest-tool dispatch. `scriptBackend` (root CLAUDE.md principle 46) handles `execution_mode='script'` agents — see spawner/CLAUDE.md Backend Capability Matrix and Script Backend section. |
| `internal/spawner/apirun/` | [spawner/apirun/CLAUDE.md](internal/spawner/apirun/CLAUDE.md) | In-process Anthropic runner: turn loop, tool dispatch, builtin tools, HTTP tool handler, sink (streaming bridge), take-control rejection, low-context save override, stall detection behavior. Three registry sources: builtins → manifest tools (`tools_manifest`) → HTTP defs. |
| `internal/orchestrator/` | [orchestrator/CLAUDE.md](internal/orchestrator/CLAUDE.md) | Layer execution, fan-in rules, callback flow, chain runner |
| `internal/api/` | [api/CLAUDE.md](internal/api/CLAUDE.md) | HTTP endpoints, handler mapping, CORS, WebSocket |
| `internal/auth/` | [auth/CLAUDE.md](internal/auth/CLAUDE.md) | Argon2id password hashing (PHC format), SCS session manager constructor, seedhash tool |
| `internal/db/` | [db/CLAUDE.md](internal/db/CLAUDE.md) | Database schema, migrations (000001–000078), connection pool |
| `internal/service/` | [service/CLAUDE.md](internal/service/CLAUDE.md) | Service layer, file mapping, workflow types, common tasks; includes AuthService and UserService |
| `internal/socket/` | [socket/CLAUDE.md](internal/socket/CLAUDE.md) | Unix socket protocol, supported methods |
| `internal/integration/` | [integration/CLAUDE.md](internal/integration/CLAUDE.md) | Test harness, helpers, running tests |
| `internal/manifest/` | [manifest/CLAUDE.md](internal/manifest/CLAUDE.md) | Manifest parsing, python script runtime, init-customer scaffold |
| `internal/sdk/python/` | [sdk/python/CLAUDE.md](internal/sdk/python/CLAUDE.md) | Embedded Python SDK for `execution_mode='script'` agents (auto-installed to `$NRFLO_HOME/sdk/`) |
| `internal/configeditor/` | [configeditor/CLAUDE.md](internal/configeditor/CLAUDE.md) | Versioned config file editing service + forward-only migration runner |

## Running Tests

```bash
make test                    # all tests (from project root)
make test-integration        # integration only (verbose)
make test-pkg PKG=orchestrator  # single package
make test-coverage           # with coverage report
make test-race               # with race detector
```

See [integration/CLAUDE.md](internal/integration/CLAUDE.md) for test harness details and helper methods.
