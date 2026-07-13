# Claude Code Instructions for nrflo

## Overview

nrflo is a multi-workflow state management system for ticket and project-level implementation with spawned AI agents: multiple workflows per ticket, project-scoped workflows (no ticket required), parallel agents (Claude, OpenAI), real-time WebSocket updates.

## New features
Do not keep old / deprecated / backward compat / legacy code
Remove it right away.

## Mandatory Rules

### 1. CLAUDE.md describes present state, under a cap

CLAUDE.md is auto-loaded into every agent's context window. It is documentation, not changelog. Keep it small.

**Present-state only.** Document the code as it is now. No migration narrative, transition timelines, future-cleanup checklists, or deprecated/legacy sections. When code is removed, remove the doc paragraph in the same commit.

**Prefer deletion over expansion.** When updating docs, first cut stale or redundant content; replace a section with a one-line source pointer where possible.

**One canonical location per concept.** Cross-reference, don't duplicate. When a paragraph would appear in two CLAUDE.md files, keep the deepest copy and link from the others.

**Hard caps (bytes; enforced by reviewer):**

| File | Cap |
|------|-----|
| Root CLAUDE.md | 10 KB |
| be/CLAUDE.md, ui/CLAUDE.md | 12 KB |
| Package CLAUDE.md (spawner, db, api, orchestrator, …) | 12 KB (spawner exception: 15 KB) |
| Sub-package / leaf CLAUDE.md | 6 KB |

**Banned content:**
- ASCII-art box diagrams (┌─┐, ├──, pipes-and-dashes). Bullet lists or short tables instead.
- Copied Go interface or struct signatures — point to the .go file with `path:line` instead.
- Verbatim JSON/TOML/protocol payload samples longer than 10 lines — link to a test fixture or source.
- Per-test inventories (## Testing sections listing every test file with a description). Use `make test-pkg PKG=<name>` as the universal pointer.
- Per-handler / per-endpoint / per-component enumerations that already exist in the file tree. List directory + grep hint.
- Status matrices duplicated across files (Backend Capability Matrix, etc.) — keep one copy, link from others.

### 2. Layer-Based Phase Execution

Agents are grouped by `layer` number; same-layer agents run concurrently, layers execute in ascending order. See [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md).

### 3. State is Stored in Database Tables

Workflow runtime state lives in `workflow_instances`/`agent_sessions`; phases derive at read time. See [db/CLAUDE.md](be/internal/db/CLAUDE.md).

### 4. Test suites must complete in under 60 seconds

`make test` (BE) and `make test-ui` (FE) are capped at 60s wall time locally (skipped when `$CI` is set — CI is ~4x slower, gates correctness only). `time.Sleep` and real CLI execution are forbidden in tests.

### 5. Keep Source Files Under 300 Lines

Split files over 300 lines into sub-files (code and docs); `.go`/`.ts`/`.tsx` enforced in CI via `make filesize` against the shrink-only `filesize.baseline` — see [be/CLAUDE.md](be/CLAUDE.md).

### 6. Polymorphism lives in the implementation, not the call site

When you find yourself writing `if x.Name() == "foo"` at a call site holding a polymorphic interface, push the divergence into the interface — don't accumulate name-checks at the call site.

## Key Files

| File | Purpose |
|------|---------|
| `be/` | Go backend source code (see [be/CLAUDE.md](be/CLAUDE.md)) |
| `ui/` | React web interface (see [ui/CLAUDE.md](ui/CLAUDE.md)) |
| `Makefile` | Build, install, test targets (`make help`) |
| `doc/` | Agent-authoring docs: common/CLI/Python/API, served at /documentation |

## Architecture Invariants

Rules every change must respect.

- **Server-only**: `nrflo_server` is the only user-facing command; all management goes through the web UI.
- **Single binary**: `nrflo_server` — server + agent subcommands (`mcp`, `record-event`, `statusline`, `context-update`, `mcp-external`); see [be/CLAUDE.md](be/CLAUDE.md). No separate `nrflo` CLI.
- **Single global SQLite DB**: `~/.nrflo/nrflo.data` (override with `NRFLO_HOME`); migrations auto-run on startup.
- **Project scope from env**: every API call resolves the project from `NRFLO_PROJECT` (or `X-Project` for HTTP).
- **Service layer**: business logic stays in `be/internal/service/`.
- **WebSocket-only realtime**: the UI never polls; all live updates flow through `/api/v1/ws`.
- **Agents identify via env**: spawner sets `NRF_SESSION_ID` + `NRF_WORKFLOW_INSTANCE_ID`.
- **Spawned agents authenticate via per-session bearer token `NRFLO_AGENT_TOKEN`**: see [api/CLAUDE.md](be/internal/api/CLAUDE.md).
- **Agents drive nrflo via MCP tools** (`mcp__nrflo__*` Claude, `nrflo/*` codex) served by the `agent mcp` bridge. See [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md).
- **API mode is a runtime admin toggle** (`api_mode_enabled`); see [api/CLAUDE.md](be/internal/api/CLAUDE.md).

## Feature Index

### Workflow execution
- **Layer execution, aggregation, callbacks** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Manual restart, retry-failed, orchestration entry points** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Low-context relaunch** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Stall detection / stall timeouts / restart cap** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Take-control / resume-session / exit-interactive / PTY relay** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Interactive start & plan mode** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Endless loop mode** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Merge conflict auto-resolution / push-after-merge** → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md)
- **Plan lifecycle & sub/dynamic workflows** (planner, revise/approve, self-drafting boundary, `run_subworkflow`/`dynamic_workflow`) → [orchestrator/CLAUDE.md](be/internal/orchestrator/CLAUDE.md) + [service/CLAUDE.md](be/internal/service/CLAUDE.md)

### Agents, templates, and configuration
- **Workflow / agent / system-agent definitions** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) + [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [doc/](doc/)
- **Default templates** → [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **CLI models registry / supported models** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)

### Execution backends (`execution_mode`)
- **`api` — in-process Anthropic runner** → [spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md)
- **`cli_interactive` backend** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **`script` — Python scriptBackend** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md)
- **Per-project venv** → [venv/](be/internal/venv/)
- **Python tools (api-mode only)** → [spawner/apirun/CLAUDE.md](be/internal/spawner/apirun/CLAUDE.md)
- **Python SDK + `script.context` socket method** → [sdk/python/CLAUDE.md](be/internal/sdk/python/CLAUDE.md) + [socket/CLAUDE.md](be/internal/socket/CLAUDE.md)
- **Provider capability matrix** → [capabilities.md](capabilities.md)

### Project-scoped & scheduled work
- **Project-scoped workflows** → [service/CLAUDE.md](be/internal/service/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Scheduled tasks** → [scheduler/CLAUDE.md](be/internal/scheduler/CLAUDE.md)
- **Workflow chains and chain runs** → [be/CLAUDE.md](be/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Run trace timeline** (`GET /workflow-instances/{iid}/trace` + Trace UI tab) → [service/CLAUDE.md](be/internal/service/CLAUDE.md)

### Auth & administration
- **Auth + sessions + login rate limit** → [auth/CLAUDE.md](be/internal/auth/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Route list, audit-log + user CRUD** → [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Service tokens** (long-lived project or global bearer tokens) → [api/CLAUDE.md](be/internal/api/CLAUDE.md)

### Storage & operations
- **Artifact storage + agent runtime** (`NRF_ARTIFACTS_DIR`, `#{ARTIFACTS}`, `artifact_*` MCP tools) → [artifact/](be/internal/artifact/) + [service/artifact.go](be/internal/service/artifact.go)
- **Agent session logs + live sessions** → [api/CLAUDE.md](be/internal/api/CLAUDE.md)
- **Per-project env vars** → [service/CLAUDE.md](be/internal/service/CLAUDE.md)
- **DB schema, migrations, connection pool** → [db/CLAUDE.md](be/internal/db/CLAUDE.md)

### Observer
- **Observer agents (experimental)** → [spawner/CLAUDE.md](be/internal/spawner/CLAUDE.md) + [api/CLAUDE.md](be/internal/api/CLAUDE.md)

## Workflows

| Workflow | Phases (by layer) | Use Case |
|----------|-------------------|----------|
| `feature` | L0: setup-analyzer -> L1: test-writer -> L2: implementor -> L3: qa-verifier -> L4: doc-updater | New features (full TDD) |
| `bugfix` | L0: setup-analyzer -> L1: implementor -> L2: qa-verifier | Bug fixes |
| `hotfix` | L0: implementor | Urgent fixes |
| `docs` | L0: setup-analyzer -> L1: doc-updater | Documentation only |
| `refactor` | L0: setup-analyzer -> L1: implementor -> L2: qa-verifier | Code refactoring |
| `deep-research` (global) | L0: scope -> L1: research -> L2: verify_a/b/c -> L3: synthesize | Multi-source web research, runnable from any project |
| `dynamic` (global) | plan-driven | On-demand multi-agent plan |

## Building & Installing

`make build` (dev, includes UI), `make build-release`, `make install` (→ `PREFIX`), `make test`; `make help` lists all targets.

### Docker image

`ghcr.io/nrflo/nrflo-server` ([Dockerfile](Dockerfile)). Api-mode off by default; bundles Claude Code + codex CLIs (native musl, sha256-pinned) and poppler-utils (codex PDF extraction); no opencode. Non-root; `/data`=`NRFLO_HOME` vol; logs `$NRFLO_HOME/logs/be.log`.
