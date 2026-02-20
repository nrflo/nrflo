# Claude Code Instructions for nrworkflow Backend

Go backend for nrworkflow. Two binaries: `nrworkflow_server` (server) and `nrworkflow` (CLI). The server provides HTTP API + WebSocket for the web UI, plus a Unix socket (and optional TCP socket for Docker agents) for agent communication. The CLI binary exposes agent commands (`agent fail/continue/callback`), findings commands (`findings add/append/get/delete`), and ticket/deps management.

## Project Structure

```
be/
├── cmd/nrworkflow/main.go       # CLI binary entry point (agent, findings, tickets, deps)
├── cmd/server/main.go           # Server binary entry point (serve)
├── internal/
│   ├── cli/                     # Cobra commands
│   │   ├── root.go              # Root command, global flags, project discovery
│   │   ├── serve.go             # HTTP API server (auto-migrates DB)
│   │   ├── agent.go             # agent fail/continue/callback (agent-only, -T for project scope)
│   │   ├── findings.go          # findings add/append/get/delete (agent-only, -T for project scope)
│   │   ├── findings_project.go  # project-level findings (project-add/get/append/delete)
│   │   ├── tickets.go           # tickets list/get/create (HTTP)
│   │   ├── tickets_update.go    # tickets update/close/reopen/delete (HTTP)
│   │   └── deps.go              # deps list/add/remove (HTTP)
│   ├── spawner/                 # Agent spawner
│   │   ├── spawner.go           # Spawn and monitor agents
│   │   ├── cli_adapter.go       # CLI adapter pattern (Claude, Opencode, Codex)
│   │   ├── docker_adapter.go    # Docker isolation wrapper (DockerCLIAdapter)
│   │   ├── cli_adapter_test.go  # Adapter tests
│   │   ├── errors.go            # Typed errors (CallbackError for layer re-execution, detected by orchestrator)
│   │   ├── completion.go        # Completion handling, continuation relaunch
│   │   ├── context_save.go      # Low-context save: kill, resume, save findings, relaunch
│   │   ├── context.go           # Context tracking (reads context_left from DB)
│   │   ├── database.go          # DB operations: register start/stop, phase management
│   │   ├── output.go            # Output monitoring, message formatting
│   │   ├── template.go          # Template loading, variable expansion
│   │   └── template_findings.go # Findings expansion, ${PREVIOUS_DATA}, formatting
│   ├── orchestrator/            # Server-side workflow orchestration
│   │   ├── orchestrator.go      # Run workflows from UI (layer-grouped concurrent phases)
│   │   └── chain_runner.go      # Sequential chain execution runner
│   ├── api/                     # HTTP API
│   │   ├── server.go            # Server setup, CORS, WebSocket hub, orchestrator, PTY manager
│   │   ├── handlers_tickets.go  # Ticket list/create/get endpoints
│   │   ├── handlers_tickets_update.go # Ticket update/delete/close/reopen endpoints
│   │   ├── handlers_workflow.go # Workflow state endpoints
│   │   ├── handlers_orchestrate.go # Ticket-scoped orchestration run/stop/restart endpoints
│   │   ├── handlers_project_workflow.go # Project-scoped workflow run/stop/restart/state
│   │   ├── handlers_workflow_def.go # Workflow definition endpoints
│   │   ├── handlers_agent_def.go # Agent definition endpoints
│   │   ├── handlers_pty.go      # PTY WebSocket handler (1:1 interactive terminal relay)
│   │   ├── handlers_chains.go   # Chain execution list/get/create/update/start/cancel + run-epic
│   │   ├── handlers_git.go        # Git commit history endpoints
│   │   ├── handlers_daily_stats.go # Daily stats endpoint
│   │   └── handlers_logs.go       # Log file viewer (BE/FE logs)
│   ├── ws/                      # WebSocket support (protocol v2)
│   │   ├── hub.go               # Client management, event log integration, broadcasting
│   │   ├── client.go            # Connection handling, subscriptions, cursor support
│   │   ├── handler.go           # HTTP upgrade handler
│   │   ├── protocol.go          # Protocol v2 constants and entity types
│   │   ├── replay.go            # Cursor-based replay from event log
│   │   ├── snapshot.go          # Snapshot streaming (begin/chunk/end)
│   │   ├── backpressure.go      # Client queue depth monitoring
│   │   └── testing.go           # Test helpers (NewTestClient)
│   ├── pty/                     # PTY session management for interactive agent control
│   │   ├── session.go           # Session struct wrapping creack/pty (spawn, read/write, resize, close)
│   │   └── manager.go           # Manager: create/get/remove/close-all PTY sessions by session ID
│   ├── config/                  # Configuration management
│   │   └── config.go
│   ├── client/                  # Socket + HTTP clients
│   │   ├── client.go            # Unix socket client for agents
│   │   ├── http.go              # HTTP client for ticket/deps CLI commands
│   │   └── output.go            # Output formatting
│   ├── socket/                  # Unix socket server
│   │   ├── server.go            # Socket listener
│   │   ├── handler.go           # Request routing
│   │   └── protocol.go          # JSON-RPC protocol types
│   ├── service/                 # Business logic layer
│   │   ├── project.go           # Project operations
│   │   ├── ticket.go            # Ticket operations
│   │   ├── workflow.go          # Workflow operations (ticket + project scope)
│   │   ├── workflow_defs.go     # Workflow definitions CRUD
│   │   ├── workflow_config.go   # Workflow config loading
│   │   ├── workflow_types.go    # Workflow type definitions (WorkflowDef, PhaseDef)
│   │   ├── workflow_validation.go # Validation (layer, fan-in, project scope)
│   │   ├── workflow_response.go # V4 response building (active agents, history)
│   │   ├── agent.go             # Agent operations
│   │   ├── agent_definition.go  # Agent definition CRUD
│   │   ├── findings.go          # Findings operations
│   │   ├── chain.go             # Chain build, dependency expansion, topo sort
│   │   ├── chain_append.go      # AppendToChain for running chains
│   │   ├── daily_stats.go       # Daily stats computation from source tables
│   │   ├── git.go               # Git operations (commit listing, detail via os/exec)
│   │   └── snapshot.go          # WS snapshot provider (builds chunks from workflow state)
│   ├── db/                      # Database layer
│   │   ├── db.go                # SQLite connection
│   │   ├── pool.go              # Connection pool (10 max, 5 idle)
│   │   ├── migrate.go           # Migration runner
│   │   └── migrations/          # SQL files (embedded via //go:embed)
│   │       └── embed.go         # Go embed directive
│   ├── model/                   # Data models
│   │   ├── project.go
│   │   ├── ticket.go
│   │   ├── agent_session.go
│   │   ├── agent_message.go
│   │   ├── agent_definition.go
│   │   ├── workflow.go
│   │   ├── workflow_instance.go
│   │   ├── chain.go             # Chain execution, item, lock models
│   │   └── daily_stats.go
│   ├── repo/                    # Repository pattern
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
│   │   ├── daily_stats.go
│   │   └── event_log.go         # WS event log persistence (append, query, cleanup)
│   ├── types/                   # Shared request/response types
│   │   ├── request.go
│   │   └── chain_request.go     # Chain create/update request types
│   ├── clock/                   # Time abstraction for testability
│   │   ├── clock.go             # Clock interface + Real() (production wall clock)
│   │   └── test.go              # TestClock with Set()/Advance() for deterministic tests
│   ├── integration/             # Integration tests
│   │   ├── testenv.go           # NewTestEnv shared harness
│   │   └── testdata/            # Test config, agent templates
│   ├── logger/                  # Structured logging with trx propagation
│   │   └── logger.go            # Init, Info/Warn/Error, NewTrx, WithTrx/TrxFromContext
│   └── id/                      # ID generation
│       └── generator.go
├── scripts/
│   ├── test.sh                  # Test runner (flags: -i -v -c -r)
│   └── context-check.sh         # Context usage hook
├── install.sh                  # Installation script
├── go.mod
├── go.sum
└── Makefile
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

```bash
cd ~/projects/2026/nrworkflow/be
make build                # Build both binaries (CLI + server)
make build-cli            # Build CLI binary (nrworkflow)
make build-server         # Build server binary (nrworkflow_server)
make build-release        # Optimized release build (both binaries)
make build-cli-release    # Optimized release build (CLI only)
make build-server-release # Optimized release build (server only)
sudo make install         # Install both to /usr/local/bin
make clean                # Clean build artifacts
```

No CGO required (pure Go SQLite via modernc.org/sqlite).

## Server Architecture

`nrworkflow_server serve` provides:
- **HTTP API** on port 6587 — web UI, REST API, WebSocket
- **Unix socket** at `/tmp/nrworkflow/nrworkflow.sock` — agent communication only (findings, agent completion, ws.broadcast)
- **TCP socket** on `127.0.0.1:6588` — for Docker agents via `host.docker.internal:6588`, always started
- **Auto-migration** — database schema is automatically migrated on startup

The socket uses a JSON-RPC style protocol (line-delimited JSON). Only `findings.*` (add, add-bulk, get, append, append-bulk, delete), `project_findings.*` (add, add-bulk, get, append, append-bulk, delete), `agent.fail/continue/callback`, and `ws.broadcast` methods are supported.

## Package Documentation

Detailed documentation for each major package is in its own CLAUDE.md:

| Package | Documentation | Key Content |
|---------|--------------|-------------|
| `internal/spawner/` | [spawner/CLAUDE.md](internal/spawner/CLAUDE.md) | CLI adapters, spawn flow, template variables, findings auto-population, output format |
| `internal/orchestrator/` | [orchestrator/CLAUDE.md](internal/orchestrator/CLAUDE.md) | Layer execution, fan-in rules, callback flow, chain runner |
| `internal/api/` | [api/CLAUDE.md](internal/api/CLAUDE.md) | HTTP endpoints, handler mapping, CORS, WebSocket |
| `internal/db/` | [db/CLAUDE.md](internal/db/CLAUDE.md) | Database schema, migrations, connection pool |
| `internal/service/` | [service/CLAUDE.md](internal/service/CLAUDE.md) | Service layer, file mapping, workflow types, common tasks |
| `internal/socket/` | [socket/CLAUDE.md](internal/socket/CLAUDE.md) | Unix socket protocol, supported methods |
| `internal/integration/` | [integration/CLAUDE.md](internal/integration/CLAUDE.md) | Test harness, helpers, running tests |

## Running Tests

```bash
cd be
make test                    # all tests
make test-integration        # integration only (verbose)
./scripts/test.sh -c         # with coverage
./scripts/test.sh -r         # with race detector
```

See [integration/CLAUDE.md](internal/integration/CLAUDE.md) for test harness details and helper methods.
