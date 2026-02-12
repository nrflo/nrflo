# Claude Code Instructions for nrworkflow

## Overview

nrworkflow is a multi-workflow state management system for ticket implementation with spawned AI agents. Supports multiple workflows per ticket, parallel agents (Claude, OpenAI), and real-time WebSocket updates.

The server runs as `nrworkflow serve` and provides an HTTP API + WebSocket for the web UI, plus a Unix socket for agent communication. Spawned agents use a minimal CLI subset (`agent complete/fail/continue`, `findings add/append/get/delete`) to report results.

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

### 2. Phase Sequence is Enforced

Agents can only be spawned in workflow phase order. The spawner validates:
- Current phase matches expected next phase
- Prior phases are completed or skipped
- Category-based skip rules are applied

### 3. State is Stored in Database Tables

Workflow runtime state is stored in normalized database tables:
- **`workflow_instances`** — one row per ticket+workflow, stores current phase, phase statuses, category, workflow-level findings
- **`agent_sessions`** — one row per agent execution, stores result, pid, findings, timestamps
- Active agents = `agent_sessions WHERE status = 'running'`
- Agent history = `agent_sessions WHERE status != 'running'`

### 4. Keep Source Files Under 300 Lines

Source files should be kept under 300 lines when possible. When a file grows beyond this limit, split it into logical sub-files. This applies to both code and documentation files.

## Key Files

| File | Purpose |
|------|---------|
| `be/` | Go backend source code (see [be/CLAUDE.md](be/CLAUDE.md)) |
| `ui/` | React web interface (see [ui/CLAUDE.md](ui/CLAUDE.md)) |
| `guidelines/agent-protocol.md` | Agent conventions |
| `nrworkflow.data` | SQLite database (tickets, projects, sessions) |
| `restart.sh` | Rebuild and restart BE + UI servers (background) |
| `stop.sh` | Stop running servers |

## Architecture Principles

1. **Server-only**: `nrworkflow serve` is the only user-facing command; all management via web UI
2. **Agent CLI subset**: Spawned agents use `agent complete/fail/continue` and `findings add/append/get/delete` via Unix socket
3. **Auto-migrate**: Database migrations run automatically on server startup
4. **Go binary**: Single `nrworkflow` binary serves both server and agent-facing CLI roles
5. **Project-scoped**: Project discovered from `NRWORKFLOW_PROJECT` env variable
6. **Single database**: `~/projects/2026/nrworkflow/nrworkflow.data` (SQLite, global for all projects)
7. **Connection Pool**: DB uses connection pooling (10 max, 5 idle)
8. **Versioned migrations**: Schema managed by golang-migrate with embedded SQL files in `db/migrations/`
9. **Service Layer**: Business logic separated from HTTP handlers and socket handlers
10. **Spawner in-process**: Spawner runs inside the server, broadcasts WebSocket events via direct hub (no socket fallback)
11. **Phase validation**: Can't spawn out of order
12. **Category-based skipping**: `skip_for` rules in workflow config
13. **WebSocket real-time**: UI receives all real-time updates via WebSocket (`/api/v1/ws`), no REST polling
14. **DB-stored workflow definitions**: Workflow definitions (phases, categories) stored in `workflows` table, managed via `/api/v1/workflows` API
15. **DB-stored agent definitions**: Agent definitions (model, timeout, prompt template) stored in `agent_definitions` table, managed via `/api/v1/workflows/{wid}/agents` API. The spawner loads templates exclusively from DB.
16. **Server-side orchestration**: Workflows run from the web UI via `POST /api/v1/tickets/:id/workflow/run`. The orchestrator runs each phase sequentially in a goroutine, reusing `spawner.Spawn()`, with cancellation support via `/workflow/stop`.

## Quick Start

Web UI: `./restart.sh` then open `http://localhost:5173`

## Agent CLI Commands

Spawned agents use these commands to report results (via Unix socket to the server):

```bash
nrworkflow agent complete <ticket> <agent-type> -w <workflow> [--model <model>]
nrworkflow agent fail <ticket> <agent-type> -w <workflow> [--model <model>] [--reason <text>]
nrworkflow agent continue <ticket> <agent-type> -w <workflow> [--model <model>]

nrworkflow findings add <ticket> <agent-type> <key> <value> -w <workflow> [--model <model>]
nrworkflow findings add <ticket> <agent-type> key1:val1 [key2:val2...] -w <workflow> [--model <model>]
nrworkflow findings append <ticket> <agent-type> <key> <value> -w <workflow> [--model <model>]
nrworkflow findings append <ticket> <agent-type> key1:val1 [key2:val2...] -w <workflow> [--model <model>]
nrworkflow findings get <ticket> <agent-type> [key] -w <workflow> [--model <model>] [-k <key>...]
nrworkflow findings delete <ticket> <agent-type> <keys...> -w <workflow> [--model <model>]
```

## Workflows

| Workflow | Phases | Use Case |
|----------|--------|----------|
| `feature` | investigation -> test-design -> implementation -> verification -> docs | New features (full TDD) |
| `implement` | implementation -> test-writing -> verification | Direct implementation with post-hoc tests |
| `bugfix` | investigation -> implementation -> verification | Bug fixes |
| `hotfix` | implementation | Urgent fixes |
| `docs` | investigation -> docs | Documentation only |
| `refactor` | investigation -> implementation -> verification | Code refactoring |

**Note:** These are example workflow configurations. Workflows are stored in the database and must be created via the `/api/v1/workflows` API or the Workflows page in the web UI.

### Categories

Categories are defined per workflow in the `categories` field. Phase skipping is controlled by per-phase `skip_for` arrays. Common examples:
- `full` - All phases run
- `simple` - Skip test-design (existing tests cover it)
- `docs` - Skip test-design and verification

## State Storage

Workflow state is stored in normalized database tables. Multiple workflows can exist per ticket via separate `workflow_instances` rows.

### `workflow_instances` table

| Column | Description |
|--------|-------------|
| `id` | UUID primary key |
| `project_id`, `ticket_id` | Links to ticket |
| `workflow_id` | FK to workflow definition |
| `status` | `active` / `completed` / `failed` |
| `current_phase` | Currently active phase ID |
| `category` | Category for skip rules |
| `phase_order` | JSON array of phase IDs |
| `phases` | JSON: `{phase_id: {status, result}}` |
| `findings` | JSON: workflow-level findings |
| `retry_count` | Number of retries |
| `parent_session` | Orchestrating session UUID |
| `created_at`, `updated_at` | Timestamps |

### `agent_sessions` table

| Column | Description |
|--------|-------------|
| `id` | UUID primary key (session ID) |
| `project_id`, `ticket_id` | Links to ticket |
| `workflow_instance_id` | FK to workflow_instances |
| `phase` | Phase name (e.g., "investigation") |
| `agent_type` | Agent identifier (e.g., "setup-analyzer") |
| `model_id` | Model used (e.g., "claude:sonnet") |
| `status` | `running` / `completed` / `failed` / `timeout` / `continued` |
| `result` | `pass` / `fail` / `continue` / `timeout` |
| `result_reason` | Explanation for result |
| `pid` | OS process ID |
| `findings` | JSON: per-agent findings |
| `context_left` | Remaining context window % |
| `ancestor_session_id` | Links continuation chain |
| `spawn_command` | Full CLI command for replay |
| `prompt_context` | System prompt file contents |
| `raw_output` | Raw stdout/stderr output from agent |
| `started_at`, `ended_at` | Execution timestamps |
| `created_at`, `updated_at` | Record timestamps |

### API Response

`GET /api/v1/tickets/:id/workflow` returns a wrapper with all workflow states:

```json
{
  "ticket_id": "TICKET-123",
  "has_workflow": true,
  "state": { /* selected workflow state (v4 format, see below) */ },
  "workflows": ["feature"],
  "all_workflows": {
    "feature": { /* v4 state */ }
  }
}
```

Each v4 workflow state contains:

```json
{
  "version": 4,
  "initialized_at": "2025-01-01T00:00:00Z",
  "workflow": "feature",
  "current_phase": "implementation",
  "category": "full",
  "status": "completed",
  "completed_at": "2025-01-01T05:23:45Z",
  "total_duration_sec": 19425.5,
  "total_tokens_used": 150000,
  "retry_count": 0,
  "phase_order": ["investigation", "test-design", "implementation", "verification", "docs"],
  "phases": {"investigation": {"status": "completed", "result": "pass"}},
  "active_agents": {
    "implementor:claude:opus": {
      "agent_id": "uuid", "agent_type": "implementor", "session_id": "uuid",
      "model_id": "claude:opus", "cli": "claude", "model": "opus",
      "pid": 12345, "started_at": "2025-01-01T00:00:00Z", "context_left": 75
    }
  },
  "agent_retries": {},
  "agent_history": [
    {
      "agent_id": "uuid", "agent_type": "setup-analyzer", "session_id": "uuid",
      "model_id": "claude:sonnet", "status": "completed", "result": "pass",
      "started_at": "...", "ended_at": "...", "context_left": 60
    }
  ],
  "findings": {"setup-analyzer:claude:sonnet": {"files_to_modify": ["..."]}},
  "parent_session": "uuid"
}
```

**Completion Statistics** (present when `status` is `"completed"`):
- `status`: Workflow instance status (`"active"`, `"completed"`, or `"failed"`)
- `completed_at`: ISO 8601 timestamp when workflow completed
- `total_duration_sec`: Total workflow duration in seconds (from creation to completion)
- `total_tokens_used`: Total context/tokens consumed across all agents (calculated using 200K token window * (100 - context_left) / 100 for each completed agent)

## Server Scripts

| Script | Purpose |
|--------|---------|
| `restart.sh` | Kill existing servers, rebuild BE + UI, start both in background |
| `stop.sh` | Stop running BE + UI servers |
| `ui/start-server.sh` | Start both servers in foreground (interactive mode) |

Logs are written to `logs/backend.log` and `logs/ui.log` when using `restart.sh`.
