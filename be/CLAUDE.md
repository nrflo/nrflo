# Claude Code Instructions for nrflo Backend

Go backend for nrflo. Two binaries: `nrflo_server` (server) and `nrflo` (CLI). The server provides HTTP API + WebSocket for the web UI, plus a Unix socket for agent communication. The CLI binary exposes agent commands (`agent fail/continue/callback`), findings commands (`findings add/append/get/delete`), and ticket/deps management.

## Project Structure

Entry points: `be/cmd/nrflo/main.go` (CLI) and `be/cmd/server/main.go` (server).

Top-level packages under `be/internal/`:

- `cli/` — Cobra commands: serve, agent, findings, tickets, deps, skip, chain
- `spawner/` — Agent spawner, execution backends (cli_interactive/api/script), low-context save, template engine
- `proc/` — Host process probing (no circular deps)
- `scheduler/` — Cron-driven scheduled task runner
- `orchestrator/` — Layer-based workflow execution, interactive/plan mode, chain runner
- `chainrunner/` — Workflow chain run execution engine
- `api/` — HTTP API handlers, CORS, WebSocket hub, PTY relay (`handlers_*.go`)
- `ws/` — WebSocket protocol, hub, client management, event log
- `pty/` — PTY session management for interactive agent control
- `config/` — Configuration management
- `client/` — Unix socket + HTTP clients
- `static/` — Embedded UI assets (`//go:embed`)
- `socket/` — Unix socket server (agent communication: findings, callbacks, ws.broadcast)
- `notify/` — Notification dispatch: Slack/Telegram transports, async retry queue
- `service/` — Business logic layer (see [service/CLAUDE.md](internal/service/CLAUDE.md))
- `db/` — SQLite connection pool, migrations (see [db/CLAUDE.md](internal/db/CLAUDE.md))
- `model/` — Data models (structs)
- `sdk/python/` — Embedded Python SDK installed to `$NRFLO_HOME/sdk/` on startup
- `manifest/` — Manifest parsing, python runtime, init-customer scaffold
- `configeditor/` — Versioned config file editing service
- `repo/` — Repository pattern (DB access layer)
- `types/` — Shared request/response types
- `clock/` — Time abstraction (`clock.Clock` interface + `Real()` + test clock)
- `integration/` — Integration tests and test harness
- `logger/` — Structured logging with trx propagation and size-based rotation
- `venv/` — Per-project Python venv manager
- `id/` — ID generation

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
make build-release        # Optimized release build (both binaries)
make install              # Install to PREFIX (default /usr/local)
make help                 # Show all targets
```

No CGO required (pure Go SQLite via modernc.org/sqlite).

## Server Architecture

`nrflo_server serve` provides:
- **HTTP API** on `127.0.0.1:6587` by default — web UI, REST API, WebSocket. Use `--host 0.0.0.0` for LAN access
- **Unix socket** at `$NRFLO_HOME/agent.sock` (override `NRFLO_SOCKET`) — agent communication only; eagerly bound at startup before HTTP listener
- **Auto-migration** — database schema is automatically migrated on startup

### Serve Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--host` | `127.0.0.1` | Host/IP to bind to |
| `--port` | `6587` | HTTP port |
| `--no-tray` | `false` | Disable macOS menu bar tray icon |
| `--mode` | `cli` | Execution mode: `cli` (default) or `api`. In `cli` mode, api-mode routes return 404 and api-mode agent defs return HTTP 400 `api_mode_disabled`. |
| `--insecure-cookies` | `false` | Disable `Secure` flag on `nrflo_session` cookie. |

## Authentication

HTTP routes use SCS cookie-based sessions (`nrflo_session`). Handler chain: `cors → requestID → projectMiddleware → LoadAndSave → mux`. Routes are `protected` (requireAuth), `admin` (requireAdmin), `projectAdmin` (requireProjectAdmin — admin user or service token whose project matches the route), or public (login only). `requireAuth` also accepts `Authorization: Bearer <token>` for spawner-minted `spawn_token` (short-lived, while agent session is `running`/`user_interactive`) and for long-lived `service_tokens` (admin-minted under Settings → Administration → Service Tokens, sha256-hashed at rest, project-scoped). Bearer requests do not populate the user context so `requireAdmin` always 403s them; service-token principals do satisfy `requireProjectAdmin` when the project matches. Login rate limiter: 5 attempts per 5 min per IP+email, returns 429 with `Retry-After`. See [be/internal/auth/CLAUDE.md](internal/auth/CLAUDE.md) and [be/internal/api/CLAUDE.md](internal/api/CLAUDE.md).

The Unix socket uses JSON-RPC line-delimited protocol. Supported methods: `findings.*`, `project_findings.*`, `agent.fail/continue/callback/context_update`, `workflow.skip`, `ws.broadcast`, `artifact.add/list/get`.

## Package Documentation

| Package | Documentation | Key Content |
|---------|--------------|-------------|
| `internal/scheduler/` | [scheduler/CLAUDE.md](internal/scheduler/CLAUDE.md) | Cron scheduler: lifecycle, dispatch flow |
| `internal/notify/` | (inline docs) | Dispatcher (ws.Listener): Slack/Telegram transports, async retry queue (backoff 15s/60s/300s), secret masking, error tracking |
| `internal/spawner/` | [spawner/CLAUDE.md](internal/spawner/CLAUDE.md) | CLI adapters, spawn flow, template variables, execution backends (cli_interactive/api/script), `Config.ProjectEnv` |
| `internal/spawner/apirun/` | [spawner/apirun/CLAUDE.md](internal/spawner/apirun/CLAUDE.md) | In-process Anthropic runner: turn loop, tool dispatch, builtin tools, HTTP tool handler |
| `internal/orchestrator/` | [orchestrator/CLAUDE.md](internal/orchestrator/CLAUDE.md) | Layer execution, layer aggregation, callback flow, chain runner |
| `internal/api/` | [api/CLAUDE.md](internal/api/CLAUDE.md) | HTTP endpoints, CORS, WebSocket, authentication middleware |
| `internal/auth/` | [auth/CLAUDE.md](internal/auth/CLAUDE.md) | Argon2id password hashing (PHC format), SCS session manager |
| `internal/db/` | [db/CLAUDE.md](internal/db/CLAUDE.md) | Migrations, connection pool, Querier interface |
| `internal/service/` | [service/CLAUDE.md](internal/service/CLAUDE.md) | Business logic, per-project env vars |
| `internal/socket/` | [socket/CLAUDE.md](internal/socket/CLAUDE.md) | Unix socket protocol, supported methods |
| `internal/integration/` | [integration/CLAUDE.md](internal/integration/CLAUDE.md) | Test harness, helpers |
| `internal/manifest/` | [manifest/CLAUDE.md](internal/manifest/CLAUDE.md) | Manifest parsing, python script runtime |
| `internal/sdk/python/` | [sdk/python/CLAUDE.md](internal/sdk/python/CLAUDE.md) | Embedded Python SDK for `execution_mode='script'` agents |
| `internal/venv/` | (inline docs) | Per-project venv: `Ensure(ctx, projectID, projectRoot)` syncs with `requirements.txt` (sha256-keyed, atomic rename); non-blocking fallback to PATH python3 |
| `internal/configeditor/` | [configeditor/CLAUDE.md](internal/configeditor/CLAUDE.md) | Versioned config file editing + forward-only migration runner |
| `internal/spec_import/` | [spec_import/CLAUDE.md](internal/spec_import/CLAUDE.md) | Spec import adapters (GitHub Issue, Jira, Markdown passthrough) |

Per-project env vars: see [internal/service/CLAUDE.md](internal/service/CLAUDE.md).
Per-project cleanup toggle (`workflow_cleanup_enabled`, default off) + per-project retention limit: `GET|PUT /api/v1/projects/{id}/settings/cleanup` and `GET|PUT /api/v1/projects/{id}/settings/artifact-storage` (see `internal/api/handlers_project_settings.go`).
Ticket refs: `be/internal/repo/ticket_ref.go` (repo) and `be/internal/model/ticket_ref.go` (model).
SeedFindings on RunRequest: see [orchestrator/CLAUDE.md](internal/orchestrator/CLAUDE.md).

## Running Tests

```bash
make test                    # all tests (from project root)
make test-integration        # integration only (verbose)
make test-pkg PKG=orchestrator  # single package
make test-coverage           # with coverage report
make test-race               # with race detector
```

See [integration/CLAUDE.md](internal/integration/CLAUDE.md) for test harness details and helper methods.
