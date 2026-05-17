# Claude Code Instructions for nrflo

## Overview

nrflo is a multi-workflow state management system for ticket and project-level implementation with spawned AI agents. Supports multiple workflows per ticket, project-scoped workflows (no ticket required), parallel agents (Claude, OpenAI), and real-time WebSocket updates.

## New features
Do not keep old / deprecated / backward compat / legacy code
Remove it right away.

## Mandatory Rules

### 1. CLAUDE.md describes present state, under a cap

CLAUDE.md is auto-loaded into every agent's context window. It is documentation, not changelog. Keep it small.

**Present-state only.** Document the code as it is now, as if it had always been this way. No migration narrative ("T1 introduced", "originally we…"), no transition timelines (T1/T2/T3/T4), no future-cleanup checklists ("flip X to false when upstream ships fix"), no deprecated/legacy sections. When code is removed, remove the doc paragraph in the same commit.

**Prefer deletion over expansion.** When a change requires a doc update, look first for content that became stale or redundant in the same area and remove it. If a section can be replaced by a one-line pointer to source code, do that.

**One canonical location per concept.** Cross-reference, don't duplicate. If the same paragraph would appear in two CLAUDE.md files, keep the deepest (most-package-specific) copy and link from the others.

**Hard caps (bytes; enforced by reviewer):**

| File | Cap |
|------|-----|
| Root CLAUDE.md | 10 KB |
| be/CLAUDE.md, ui/CLAUDE.md | 12 KB |
| Package CLAUDE.md (spawner, db, api, orchestrator, …) | 12 KB (spawner exception: 15 KB) |
| Sub-package / leaf CLAUDE.md | 6 KB |

If a file would exceed its cap, cut content first. Only split as a last resort.

**Banned content:**
- ASCII-art box diagrams (┌─┐, ├──, pipes-and-dashes). Bullet lists or short tables instead.
- Copied Go interface or struct signatures — point to the .go file with `path:line` instead.
- Verbatim JSON/TOML/protocol payload samples longer than 10 lines — link to a test fixture or source.
- Per-test inventories (## Testing sections listing every test file with a description). Use `make test-pkg PKG=<name>` as the universal pointer.
- Per-handler / per-endpoint / per-component enumerations that already exist in the file tree. List directory + grep hint.
- Status matrices duplicated across files (Backend Capability Matrix, etc.) — keep one copy, link from others.

### 2. Layer-Based Phase Execution

Agents are grouped by `layer` number; all agents in the same layer run concurrently, layers execute in ascending order. See [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) for pass policies and fan-in rules.

### 3. State is Stored in Database Tables

Workflow runtime state lives in `workflow_instances` and `agent_sessions`; phases are derived at read time. See [db/CLAUDE.md](be/internal/db/CLAUDE.md) for schema.

### 4. Test suites must complete in under 60 seconds

`make test` (BE) and `make test-ui` (FE) are each capped at 60 s wall time. `time.Sleep` and real CLI binary execution are forbidden in tests.

### 5. Keep Source Files Under 300 Lines

Split files that grow beyond 300 lines into logical sub-files; this applies to code and documentation.

### 6. Polymorphism lives in the implementation, not the call site

When you find yourself writing `if x.Name() == "foo"` at a call site holding a polymorphic interface, push the divergence into the interface — don't accumulate name-checks at the call site.

## Key Files

| File | Purpose |
|------|---------|
| `be/` | Go backend source code (see [be/CLAUDE.md](be/CLAUDE.md)) |
| `ui/` | React web interface (see [ui/CLAUDE.md](ui/CLAUDE.md)) |
| `Makefile` | Build, install, test targets (`make help`) |
| `agent_manual.md` | User-facing agent definition guide (template vars, findings, CLI) |

## Architecture Invariants

Rules every change must respect.

- **Server-only**: `nrflo_server serve` is the only user-facing command; all management goes through the web UI.
- **Two binaries**: `nrflo_server` (server) and `nrflo` (agent + ticket/deps CLI).
- **Single global SQLite DB**: `~/.nrflo/nrflo.data` (override with `NRFLO_HOME`); migrations auto-run on startup.
- **Project scope from env**: every CLI/API call resolves the project from `NRFLO_PROJECT` (or the `X-Project` header for HTTP).
- **Service layer**: business logic stays in `be/internal/service/`.
- **WebSocket-only realtime**: the UI never polls; all live updates flow through `/api/v1/ws`.
- **Agents identify via env**: spawner sets `NRF_SESSION_ID` + `NRF_WORKFLOW_INSTANCE_ID`.
- **Spawned agents authenticate via per-session bearer token in `NRFLO_AGENT_TOKEN`**: see [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md).
- **Agent CLI is a small subset** — see [agent_manual.md](agent_manual.md).

## Feature Index

### Workflow execution
- **Layer-based concurrent execution + layer aggregation + agent callbacks** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Manual restart, retry-failed, server-side orchestration entry points** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Low-context relaunch** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Stall detection / global stall timeouts / restart cap** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Take-control / resume-session / exit-interactive / PTY relay** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Interactive start & plan mode** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Endless loop mode** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Automatic merge conflict resolution / push-after-merge** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)

### Agents, templates, and configuration
- **Workflow definitions, agent definitions, system agents** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) + [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [agent_manual.md](agent_manual.md)
- **Default templates** → [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Low consumption mode** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **CLI models registry / supported models** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)

### Execution backends (`execution_mode`)
- **`api` — in-process Anthropic runner** → [spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md)
- **`cli_interactive` backend** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **`script` — Python scriptBackend** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Per-project venv** → [venv/](be/internal/venv/)
- **Manifest tools (api-mode only)** → [manifest/CLAUDE.md](be/internal/manifest/CLAUDE.md) + [spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md)
- **Python SDK + `script.context` socket method** → [sdk/python/CLAUDE.md](be/internal/sdk/python/CLAUDE.md) + [socket/CLAUDE.md](be/internal/socket/CLAUDE.md)
- **Provider capability matrix** → [capabilities.md](capabilities.md)

### Project-scoped & scheduled work
- **Project-scoped workflows** → [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Scheduled tasks** → [scheduler/CLAUDE.md](be/internal/scheduler/CLAUDE.md)
- **Workflow chains and chain runs** → [be/CLAUDE.md](be/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md) + [ui/CLAUDE.md](ui/CLAUDE.md)

### Auth & administration
- **Auth + sessions + login rate limit** → [auth/CLAUDE.md](be/internal/auth/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Route list, audit-log + user CRUD** → [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Service tokens (long-lived project-scoped bearer tokens for external callers)** → [api/CLAUDE.md](be/internal/api/CLAUDE.md)

### Storage & operations
- **Artifact storage + agent runtime (`NRF_ARTIFACTS_DIR`, `#{ARTIFACTS}`, `nrflo agent artifact add|list|get`, api-mode builtins, Python SDK `c.artifacts`)** → [artifact/](be/internal/artifact/) + [service/artifact.go](be/internal/service/artifact.go)
- **Agent session logs + live sessions** → [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Notification channels** → [be/CLAUDE.md](be/CLAUDE.md)
- **Per-project env vars** → [service/CLAUDE.md](be/internal/service/CLAUDE.md)
- **DB schema, migrations, connection pool** → [db/CLAUDE.md](be/internal/db/CLAUDE.md)
- **Versioned config-file editor** → [configeditor/CLAUDE.md](be/internal/configeditor/CLAUDE.md)

See `be/cmd/server/main.go` for subcommands.

## Workflows

| Workflow | Phases (by layer) | Use Case |
|----------|-------------------|----------|
| `feature` | L0: setup-analyzer -> L1: test-writer -> L2: implementor -> L3: qa-verifier -> L4: doc-updater | New features (full TDD) |
| `bugfix` | L0: setup-analyzer -> L1: implementor -> L2: qa-verifier | Bug fixes |
| `hotfix` | L0: implementor | Urgent fixes |
| `docs` | L0: setup-analyzer -> L1: doc-updater | Documentation only |
| `refactor` | L0: setup-analyzer -> L1: implementor -> L2: qa-verifier | Code refactoring |

## API Response Format

`GET /api/v1/tickets/:id/workflow` returns a v4 format wrapper with workflow state, findings, and agent history. See [be/internal/api/CLAUDE.md](be/internal/api/CLAUDE.md) for full details.

## Building & Installing

```bash
make build          # Build both binaries (dev, includes UI)
make build-release  # Optimized release build
make install        # Install to /usr/local/bin (or PREFIX=...)
make test           # Run backend tests
make help           # Show all targets
```

### Docker image

Distributed as `ghcr.io/nrflo/nrflo-server` (see [Dockerfile](Dockerfile) and [.github/workflows/docker.yml](.github/workflows/docker.yml)). Api-mode only; non-root user `nrflo`; `/data` as the `NRFLO_HOME` volume.

Logs are written to `~/.nrflo/logs/be.log` (or `$NRFLO_HOME/logs/be.log`).
