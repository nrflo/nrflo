# Claude Code Instructions for nrworkflow

## Overview

nrworkflow is a multi-workflow state management system for ticket and project-level implementation with spawned AI agents. Supports multiple workflows per ticket, project-scoped workflows (no ticket required), parallel agents (Claude, OpenAI), and real-time WebSocket updates.

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

### 2. Layer-Based Phase Execution

Agents are grouped by `layer` number. All agents in the same layer run concurrently; layers execute in ascending order. The spawner validates:
- All agents in prior layers are completed or skipped before the current layer starts
- Category-based skip rules are applied per agent via `skip_for`
- Fan-in: if a layer has multiple agents, the next layer must have exactly 1 agent
- At least one agent in a layer must pass for the workflow to proceed (all-skipped continues)

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
11. **Layer-based execution**: Phases grouped by layer; same-layer agents run concurrently, layers execute sequentially with fan-in (pass_count >= 1)
12. **Category-based skipping**: `skip_for` rules in workflow config
13. **WebSocket real-time**: UI receives all real-time updates via WebSocket (`/api/v1/ws`), no REST polling
14. **DB-stored workflow definitions**: Workflow definitions (phases, categories) stored in `workflows` table, managed via `/api/v1/workflows` API
15. **DB-stored agent definitions**: Agent definitions (model, timeout, prompt template) stored in `agent_definitions` table, managed via `/api/v1/workflows/{wid}/agents` API. The spawner loads templates exclusively from DB.
16. **Server-side orchestration**: Workflows run from the web UI via `POST /api/v1/tickets/:id/workflow/run`. The orchestrator groups phases by layer and runs all agents in each layer concurrently (one goroutine per agent calling `spawner.Spawn()`), with cancellation support via `/workflow/stop`.
17. **Low-context relaunch**: When an agent's context drops below threshold (default ~25% remaining, configurable per agent via `restart_threshold` in agent_definitions), the spawner kills the agent, resumes with `claude --resume` instructing it to save progress under the `to_resume` findings key, then spawns a fresh agent with `${PREVIOUS_DATA}` populated from that `to_resume` key. Old sessions get `status='continued'` and are excluded from agent history.
18. **Manual agent restart**: Users can trigger an agent restart from the UI via `POST /api/v1/tickets/:id/workflow/restart` with `{workflow, session_id}`. This triggers the same context-save-and-relaunch flow as the automatic low-context restart, regardless of current token usage.
19. **Project-scoped workflows**: Workflows can have `scope_type` of `ticket` (default) or `project`. Project-scoped workflows run at project level without requiring a ticket. API: `POST /api/v1/projects/:id/workflow/run`, `GET /api/v1/projects/:id/workflow`, `GET /api/v1/projects/:id/agents`. Project agents cannot use `${TICKET_ID}`, `${TICKET_TITLE}`, or `${TICKET_DESCRIPTION}` template variables.

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

## Ticket CLI Commands

Manage tickets and dependencies via the HTTP API (requires server running):

```bash
nrworkflow tickets list [--status <status>] [--type <type>] [--parent <id>] [--json]
nrworkflow tickets get <id> [--json]
nrworkflow tickets create --title <title> [--description <text>] [--type <type>] [--priority <1-4>] [--parent <id>] [--created-by <name>] [--json]
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

### Phase Definition Format

Each phase entry must be an object with a required `layer` field:
```json
{"agent": "setup-analyzer", "layer": 0, "skip_for": ["docs"]}
```

- `layer` (required): Integer >= 0. Same-layer agents run concurrently.
- `agent` (required): Agent definition ID.
- `skip_for` (optional): Categories for which this agent is skipped.
- String-only entries and `parallel` field are rejected.
- Supported models: `opus`, `sonnet`, `haiku`, `gpt_5.3`.

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
| `project_id`, `ticket_id` | Links to ticket (ticket_id empty for project scope) |
| `workflow_id` | FK to workflow definition |
| `scope_type` | `ticket` (default) or `project` |
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
| `restart_count` | Number of low-context restarts (default 0) |
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
  "scope_type": "ticket",
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
      "pid": 12345, "started_at": "2025-01-01T00:00:00Z", "context_left": 75, "restart_count": 0, "restart_threshold": 25
    }
  },
  "agent_retries": {},
  "agent_history": [
    {
      "agent_id": "uuid", "agent_type": "setup-analyzer", "session_id": "uuid",
      "model_id": "claude:sonnet", "status": "completed", "result": "pass",
      "started_at": "...", "ended_at": "...", "context_left": 60, "restart_count": 0
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

## Chain Execution

Chains allow sequential execution of multiple tickets with a single workflow. A chain reserves tickets via locks to prevent overlapping runs across chains.

### Chain Lifecycle

```
[*] → Pending → Running → Completed
                   ↓          ↑
                 Failed    (last item)
Pending → Canceled
Running → Canceled
Running → Failed (item fails)
```

### Chain API

```bash
GET    /api/v1/chains              # List chains (?status= filter)
POST   /api/v1/chains              # Create (pending), expands deps + topo sort
GET    /api/v1/chains/:id          # Get with ordered items
PATCH  /api/v1/chains/:id          # Edit (pending only)
POST   /api/v1/chains/:id/start    # Start sequential execution
POST   /api/v1/chains/:id/cancel   # Cancel + release locks
```

### Key Behaviors

- **Dependency expansion**: Selected tickets are expanded with transitive blockers
- **Topological sort**: Items ordered by dependency graph (tie-break: created_at, then ID)
- **Cycle detection**: DFS-based detection after expansion
- **Lock exclusivity**: UNIQUE(project_id, ticket_id) prevents overlapping chains
- **Sequential execution**: Items run one at a time via orchestrator
- **Crash recovery**: Zombie running chains are marked failed on server startup

## Server Scripts

| Script | Purpose |
|--------|---------|
| `restart.sh` | Kill existing servers, rebuild BE + UI, start both in background |
| `stop.sh` | Stop running BE + UI servers |
| `ui/start-server.sh` | Start both servers in foreground (interactive mode) |

Logs are written to `logs/backend.log` and `logs/ui.log` when using `restart.sh`.
