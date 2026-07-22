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

**Overflow goes to REFERENCE.md.** When a package CLAUDE.md nears its cap, move flow mechanics into a sibling `REFERENCE.md` (not auto-loaded, uncapped) and keep the invariant + a pointer in CLAUDE.md. Read the REFERENCE.md before changing the flows it documents.

**Populating (one pass, no thrash).** (1) Pick ONE owning package CLAUDE.md per changed area — deepest wins; no behavior change → no doc edit. (2) Add/update ≤3 sentences in the relevant section; anything longer is REFERENCE.md material. (3) Never reword existing unrelated content to free space. (4) Measure `wc -c` once at the END; if over cap, move the largest mechanics paragraph to REFERENCE.md — do not compress wording.

**Hard caps** (bytes, enforced by reviewer + `.claude/skills/finalize/check_caps.sh`): root 10240; be/, ui/, package 12288 (spawner 15360); leaf 6144. Banned content (box diagrams, copied signatures, long payload samples, per-test/per-endpoint inventories, duplicated matrices) and the **one-pass populate procedure**: [REFERENCE.md](REFERENCE.md#doc-authoring-full-rule-1) — read it before editing any CLAUDE.md.

### 2. Layer-Based Phase Execution

Agents are grouped by `layer` number; same-layer agents run concurrently, layers execute in ascending order. See [orchestrator](be/internal/orchestrator/CLAUDE.md).

### 3. State is Stored in Database Tables

Workflow runtime state lives in `workflow_instances`/`agent_sessions`; phases derive at read time. See [db](be/internal/db/CLAUDE.md).

### 4. Test suites must complete in under 60 seconds

`make test` (BE) and `make test-ui` (FE) are capped at 60s wall time locally (skipped when `$CI` is set — CI is ~4x slower, gates correctness only; an over-cap run on a machine whose 1-min load was already ≥ half the cores at start degrades to a warning). `time.Sleep` and real CLI execution are forbidden in tests.

### 5. Keep Source Files Under 300 Lines

Split files over 300 lines into sub-files (code and docs); `.go`/`.ts`/`.tsx` enforced in CI via `make filesize` against the shrink-only `filesize.baseline` — see [be](be/CLAUDE.md).

### 6. Polymorphism lives in the implementation, not the call site

When you find yourself writing `if x.Name() == "foo"` at a call site holding a polymorphic interface, push the divergence into the interface — don't accumulate name-checks at the call site.

### 7. Comments: only where needed

Don't add excessive comments. Comment only where intent is non-obvious; keep them short and descriptive — never narrate what the code already says.

## Key Files

| File | Purpose |
|------|---------|
| `be/` | Go backend (see [be](be/CLAUDE.md)) |
| `ui/` | React web interface (see [ui](ui/CLAUDE.md)) |
| `Makefile` | Build, install, test targets (`make help`) |
| `doc/` | Agent-authoring docs: common/CLI/Python/API, served at /documentation |

## Architecture Invariants

Rules every change must respect.

- **Server-only**: `nrflo_server` is the only user-facing command; management goes through the web UI, except `nrflo_server console`, which opens the native TUI for a server-owned Claude/Codex/API conversation.
- **Single binary**: `nrflo_server` — server + agent subcommands (`mcp`, `record-event`, `statusline`, `context-update`, `mcp-external`) + `console`; see [be](be/CLAUDE.md). No separate `nrflo` CLI.
- **Single global SQLite DB**: `~/.nrflo/nrflo.data` (override with `NRFLO_HOME`); migrations auto-run on startup.
- **Project scope from env**: every API call resolves the project from `NRFLO_PROJECT` (or `X-Project` for HTTP).
- **Service layer**: business logic stays in `be/internal/service/`.
- **WebSocket-only realtime**: the UI never polls; all live updates flow through `/api/v1/ws`.
- **Agents identify via env**: spawner sets `NRF_SESSION_ID` + `NRF_WORKFLOW_INSTANCE_ID`.
- **Spawned agents authenticate via per-session bearer token `NRFLO_AGENT_TOKEN`**: see [api](be/internal/api/CLAUDE.md).
- **Agents drive nrflo via MCP tools** (`mcp__nrflo__*` Claude, `nrflo/*` codex) served by the `agent mcp` bridge. See [spawner](be/internal/spawner/CLAUDE.md).
- **API mode is a runtime admin toggle** (`api_mode_enabled`); see [api](be/internal/api/CLAUDE.md).

## Feature Index

Feature → owning-doc routing table: [REFERENCE.md](REFERENCE.md#feature-index). Per-package doc table: [be/CLAUDE.md](be/CLAUDE.md).

## Workflows

| Workflow | Phases (by layer) | Use Case |
|----------|-------------------|----------|
| `feature` | setup-analyzer -> test-writer -> implementor -> qa-verifier -> doc-updater | New features (full TDD) |
| `bugfix` | setup-analyzer -> implementor -> qa-verifier | Bug fixes |
| `hotfix` | implementor | Urgent fixes |
| `docs` | setup-analyzer -> doc-updater | Documentation only |
| `refactor` | setup-analyzer -> implementor -> qa-verifier | Code refactoring |
| `dynamic` (global) | plan-driven | On-demand multi-agent plan |

## Building & Installing

`make build` (dev, includes UI), `make build-release`, `make install` (→ `PREFIX`), `make test`; `make help` lists all targets.

### Docker image

`ghcr.io/nrflo/nrflo-server` — build/runtime details: [REFERENCE.md](REFERENCE.md#docker-image).
