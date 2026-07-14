# Claude Code Instructions for nrflo Backend

Go backend for nrflo. Single binary: `nrflo_server`. It provides the HTTP API + WebSocket for the web UI, a Unix socket for agent communication, and hosts the agent infrastructure subcommands the spawner invokes (`nrflo_server agent {mcp,record-event,statusline,context-update}`). Agents themselves drive nrflo (findings, lifecycle, artifacts, …) via MCP tools served by the `agent mcp` bridge — there is no separate `nrflo` CLI. Any standalone (non-spawned) MCP client drives nrflo via the token-authed `agent mcp-external` bridge, which opens a console session at connect (project fixed for the connection) and proxies `tools/list`/`tools/call` to the server-owned console tool catalogue — see [internal/api/CLAUDE.md](internal/api/CLAUDE.md). Deep mechanics (filesize gate, serve flags, .env, auth chain, DB test pattern): [REFERENCE.md](REFERENCE.md).

## Project Structure

Entry point: `be/cmd/server/main.go` (calls `cli.RegisterServerCommands()`).

Top-level packages under `be/internal/`:

- `cli/` — Cobra commands: serve + the agent infra subcommands (mcp, record-event, statusline, context-update)
- `spawner/` — Agent spawner, execution backends (cli_interactive/api/script), low-context save, template engine
- `proc/` — Host process probing (no circular deps)
- `scheduler/` — Cron-driven scheduled task runner
- `orchestrator/` — Layer-based workflow execution, interactive/plan mode, chain runner
- `chainrunner/` — Workflow chain run execution engine
- `api/` — HTTP API handlers, CORS, WebSocket hub, PTY relay (`handlers_*.go`)
- `ws/` — WebSocket protocol, hub, client management, event log
- `pty/` — PTY session management for interactive agent control
- `config/` — Configuration management
- `client/` — Unix socket client
- `static/` — Embedded UI assets + per-kind agent docs (`//go:embed`; `Manual(kind)`)
- `socket/` — Unix socket server (agent communication: findings, callbacks, ws.broadcast)
- `notify/` — Notification dispatch: Slack/Telegram/Script transports, async retry queue
- `service/` — Business logic layer (see [service/CLAUDE.md](internal/service/CLAUDE.md))
- `db/` — SQLite connection pool, migrations (see [db/CLAUDE.md](internal/db/CLAUDE.md))
- `model/` — Data models (structs)
- `sdk/python/` — Embedded Python SDK installed to `$NRFLO_HOME/sdk/` on startup
- `repo/` — Repository pattern (DB access layer)
- `types/` — Shared request/response types
- `clock/` — Time abstraction (`clock.Clock` interface + `Real()` + test clock)
- `integration/` — Integration tests and test harness
- `logger/` — Structured logging with trx propagation and size-based rotation
- `venv/` — Per-project Python venv manager
- `id/` — ID generation

## Source File Size Limit

Keep source files under 300 lines (`.go`, `.ts`, `.tsx`, including tests and migration scripts). Split anything that grows past the limit into logical sub-files before committing; `make filesize` gates against the shrink-only `filesize.baseline` ratchet in CI. Gate, ratchet rules, split naming/method: [REFERENCE.md § Source File Size Limit](REFERENCE.md#source-file-size-limit) — read before splitting a file or touching the baseline.

## Dependencies

Go 1.25+; pure Go SQLite via modernc.org/sqlite (no CGO). Full list: [REFERENCE.md § Dependencies](REFERENCE.md#dependencies).

## Building from Source

All build targets are in the **root** `Makefile` (not `be/`): `make build` (includes UI), `make build-server-only` (Go-only), `make build-release`, `make install`; `make help` lists all. Annotated list: [REFERENCE.md § Building from Source](REFERENCE.md#building-from-source).

## Server Architecture

`nrflo_server serve` provides:
- **HTTP API** on `127.0.0.1:6587` by default — web UI, REST API, WebSocket. Use `--host 0.0.0.0` for LAN access
- **Unix socket** at `$NRFLO_HOME/agent.sock` (override `NRFLO_SOCKET`) — agent communication only; eagerly bound at startup before HTTP listener
- **Auto-migration** — database schema is automatically migrated on startup

### Serve Flags

`--host` (default `127.0.0.1`), `--port` (default `6587`), `--no-tray`, `--insecure-cookies`. Table: [REFERENCE.md § Serve Flags](REFERENCE.md#serve-flags).

API mode is toggled per-server via the `api_mode_enabled` global setting (Settings → Admin) and read freshly at request/spawn time.

### .env loading

`serve` startup reads a `.env` file from the launch directory into the process env; real env always wins, missing file is a no-op. Mechanics and which vars it feeds: [REFERENCE.md § .env loading](REFERENCE.md#env-loading) — read before changing server env resolution.

## Authentication

HTTP routes use SCS cookie sessions (`nrflo_session`); `requireAuth` also accepts Bearer `spawn_token` / `service_tokens`. Middleware chain, route classes, bearer semantics, login rate limit: [REFERENCE.md § Authentication](REFERENCE.md#authentication) — read before changing auth middleware.

The Unix socket uses line-delimited JSON-RPC; methods in [internal/socket/CLAUDE.md](internal/socket/CLAUDE.md).

## Package Documentation

| Package | Documentation | Key Content |
|---------|--------------|-------------|
| `internal/scheduler/` | [scheduler/CLAUDE.md](internal/scheduler/CLAUDE.md) | Cron scheduler: lifecycle, dispatch flow |
| `internal/notify/` | [REFERENCE.md § notify](REFERENCE.md#notify) | Dispatcher (ws.Listener): Slack/Telegram/Script transports, async retry queue |
| `internal/spawner/` | [spawner/CLAUDE.md](internal/spawner/CLAUDE.md) | CLI adapters, spawn flow, template variables, execution backends (cli_interactive/api/script), `Config.ProjectEnv` |
| `internal/spawner/apirun/` | [spawner/apirun/CLAUDE.md](internal/spawner/apirun/CLAUDE.md) | In-process Anthropic runner: turn loop, tool dispatch, builtin tools, HTTP tool handler |
| `internal/orchestrator/` | [orchestrator/CLAUDE.md](internal/orchestrator/CLAUDE.md) | Layer execution, layer aggregation, callback flow, chain runner |
| `internal/api/` | [api/CLAUDE.md](internal/api/CLAUDE.md) | HTTP endpoints, CORS, WebSocket, authentication middleware |
| `internal/auth/` | [auth/CLAUDE.md](internal/auth/CLAUDE.md) | Argon2id password hashing (PHC format), SCS session manager |
| `internal/db/` | [db/CLAUDE.md](internal/db/CLAUDE.md) | Migrations, connection pool, Querier interface |
| `internal/service/` | [service/CLAUDE.md](internal/service/CLAUDE.md) | Business logic, per-project env vars |
| `internal/socket/` | [socket/CLAUDE.md](internal/socket/CLAUDE.md) | Unix socket protocol, supported methods |
| `internal/integration/` | [integration/CLAUDE.md](internal/integration/CLAUDE.md) | Test harness, helpers |
| `internal/sdk/python/` | [sdk/python/CLAUDE.md](internal/sdk/python/CLAUDE.md) | Embedded Python SDK for `execution_mode='script'` agents |
| `internal/venv/` | [REFERENCE.md § venv](REFERENCE.md#venv) | Per-project Python venv manager |
| `internal/spec_import/` | [spec_import/CLAUDE.md](internal/spec_import/CLAUDE.md) | Spec import adapters (GitHub Issue, Jira, Markdown passthrough) |
| `internal/console/` | [console/CLAUDE.md](internal/console/CLAUDE.md) | Console tool profile + registry |

Per-project env vars: see [internal/service/CLAUDE.md](internal/service/CLAUDE.md).
Per-project cleanup toggle + retention limit endpoints: [REFERENCE.md § Project settings endpoints](REFERENCE.md#project-settings-endpoints).
Ticket refs: `be/internal/repo/ticket_ref.go` (repo) and `be/internal/model/ticket_ref.go` (model).
SeedFindings on RunRequest: see [orchestrator/CLAUDE.md](internal/orchestrator/CLAUDE.md).

## Running Tests

```bash
make test                    # fast backend suite (from project root)
make test-smoke              # slow real-binary/build smoke tests
make test-integration        # integration only (verbose)
make test-pkg PKG=orchestrator  # single package
make test-coverage           # with coverage report
make test-race               # with race detector
```

See [integration/CLAUDE.md](internal/integration/CLAUDE.md) for test harness details and helper methods.

### DB tests never migrate per-test

DB-backed test packages migrate one template DB in `TestMain` and copy it per test (open copies with the `*Existing` openers); never call `db.Open`/`db.NewPoolPath` inside a test for a fresh DB. Full pattern and the one-per-package from-scratch exception: [REFERENCE.md § DB tests never migrate per-test](REFERENCE.md#db-tests-never-migrate-per-test) — read before adding a DB-backed test package.
