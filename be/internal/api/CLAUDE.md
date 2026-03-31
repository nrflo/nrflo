# API Package

HTTP API server providing REST endpoints and WebSocket for the web UI.

## Server Architecture

`nrworkflow_server serve` provides:
- **HTTP API** on port 6587 — web UI, REST API, WebSocket
- **CORS** enabled for `http://localhost:5175`
- **WebSocket** at `/api/v1/ws` for real-time updates

## Handler File Mapping

| File | Endpoints |
|------|-----------|
| `server.go` | Server setup, CORS, route registration, orchestrator init |
| `handlers_tickets.go` | Ticket list/create/get |
| `handlers_tickets_update.go` | Ticket update/delete/close/reopen |
| `handlers_workflow.go` | Workflow state get/patch |
| `handlers_orchestrate.go` | Ticket-scoped run/stop/restart/retry-failed/take-control/resume-session/exit-interactive/run-epic |
| `handlers_project_workflow.go` | Project-scoped run/stop/restart/retry-failed/take-control/resume-session/exit-interactive/state/agents |
| `handlers_pty.go` | PTY WebSocket handler: upgrade, validate session, spawn/relay PTY, handle resize, exit-interactive on process exit |
| `handlers_workflow_def.go` | Workflow definition CRUD |
| `handlers_agent_def.go` | Agent definition CRUD |
| `handlers_system_agent_def.go` | System agent definition CRUD (global, no project scope) |
| `handlers_default_template.go` | Default template CRUD (global, no project scope, readonly enforcement) |
| `handlers_chains.go` | Chain preview/list/get/create/update/start/cancel/append |
| `handlers_git.go` | Git commit history list/detail |
| `handlers_daily_stats.go` | Daily stats endpoint |
| `handlers_global_settings.go` | Global settings GET/PATCH (no project scope) |
| `handlers_project_findings.go` | Project findings GET (project-scoped) |
| `handlers_docs.go` | Documentation (agent manual) |
| `handlers_session_prompt.go` | Session prompt context (GET /api/v1/sessions/:id/prompt) |
| `handlers_logs.go` | Log file viewer (BE/FE logs) |

## HTTP API Endpoints

```bash
# Projects
GET /api/v1/projects
GET /api/v1/projects/:id
POST /api/v1/projects
PATCH /api/v1/projects/:id
DELETE /api/v1/projects/:id

# Tickets (require X-Project header or ?project= param)
GET /api/v1/tickets
GET /api/v1/tickets/:id       # Returns enriched ticket with blockers, blocks, children, parent_ticket, siblings
POST /api/v1/tickets
PATCH /api/v1/tickets/:id
DELETE /api/v1/tickets/:id
POST /api/v1/tickets/:id/close

# Workflow state (ticket-scoped runtime state)
GET /api/v1/tickets/:id/workflow
PATCH /api/v1/tickets/:id/workflow

# Workflow orchestration (run/stop/restart from UI)
POST /api/v1/tickets/:id/workflow/run      # Start orchestrated run; body accepts `interactive` (bool) and `plan_mode` (bool), mutually exclusive (400 if both true). When set, response includes `session_id` and status `"interactive"` or `"planning"`
POST /api/v1/tickets/:id/workflow/stop     # Stop running orchestration (optional instance_id in body)
POST /api/v1/tickets/:id/workflow/restart       # Restart agent (context save + relaunch)
POST /api/v1/tickets/:id/workflow/retry-failed  # Retry failed workflow from failed layer
POST /api/v1/tickets/:id/workflow/take-control     # Kill agent, return session ID for interactive use
POST /api/v1/tickets/:id/workflow/resume-session   # Resume finished Claude session (set to user_interactive)
POST /api/v1/tickets/:id/workflow/exit-interactive  # Signal interactive session completed, unblock spawner
POST /api/v1/tickets/:id/workflow/run-epic    # Create chain from epic children, optionally start

# Workflow definitions (project-scoped, require X-Project header)
GET    /api/v1/workflows              # List all
POST   /api/v1/workflows              # Create
GET    /api/v1/workflows/:id          # Get one
PATCH  /api/v1/workflows/:id          # Update
DELETE /api/v1/workflows/:id          # Delete

# Project-scoped workflow operations
POST /api/v1/projects/:id/workflow/run      # Start project workflow; body accepts `interactive` (bool) and `plan_mode` (bool), mutually exclusive (400 if both true). When set, response includes `session_id` and status `"interactive"` or `"planning"`
POST /api/v1/projects/:id/workflow/stop     # Stop project workflow
POST /api/v1/projects/:id/workflow/restart       # Restart project agent
POST /api/v1/projects/:id/workflow/retry-failed  # Retry failed project workflow
POST /api/v1/projects/:id/workflow/take-control     # Kill project agent, return session ID
POST /api/v1/projects/:id/workflow/resume-session   # Resume finished project Claude session
POST /api/v1/projects/:id/workflow/exit-interactive  # Signal project interactive session completed
DELETE /api/v1/projects/:id/workflow/:instance_id  # Delete completed/failed project workflow instance (409 if active)
GET  /api/v1/projects/:id/workflow          # Get project workflow state
GET  /api/v1/projects/:id/agents           # Get project agent sessions
GET  /api/v1/projects/:id/findings         # Get all project findings as JSON map

# Git (project-scoped, reads from project root_path)
GET  /api/v1/projects/:id/git/commits           # Paginated commit list (?page=&per_page=)
GET  /api/v1/projects/:id/git/commits/:hash     # Single commit detail with diff

# Agent definitions (nested under workflows)
GET    /api/v1/workflows/:wid/agents           # List agents for workflow
POST   /api/v1/workflows/:wid/agents           # Create agent definition
GET    /api/v1/workflows/:wid/agents/:id       # Get agent definition
PATCH  /api/v1/workflows/:wid/agents/:id       # Update agent definition
DELETE /api/v1/workflows/:wid/agents/:id       # Delete agent definition

# System agent definitions (global, no project scope)
GET    /api/v1/system-agents           # List all system agent definitions
POST   /api/v1/system-agents           # Create system agent definition
GET    /api/v1/system-agents/:id       # Get system agent definition
PATCH  /api/v1/system-agents/:id       # Update system agent definition
DELETE /api/v1/system-agents/:id       # Delete system agent definition

# Default templates (global, no project scope)
GET    /api/v1/default-templates           # List all default templates
POST   /api/v1/default-templates           # Create default template (always non-readonly)
GET    /api/v1/default-templates/:id       # Get default template
PATCH  /api/v1/default-templates/:id       # Update default template (403 if readonly)
DELETE /api/v1/default-templates/:id       # Delete default template (403 if readonly)

# Global settings (no project scope)
GET    /api/v1/settings           # Returns {"low_consumption_mode": bool, "session_retention_limit": int, "stall_start_timeout_sec": int|null, "stall_running_timeout_sec": int|null}
PATCH  /api/v1/settings           # Accepts {"low_consumption_mode": bool, "session_retention_limit": int (>= 10), "stall_start_timeout_sec": int|null (>= 0), "stall_running_timeout_sec": int|null (>= 0)}

# Agent sessions
GET /api/v1/tickets/:id/agents
GET /api/v1/tickets/:id/agents?phase=investigation

# Running agents (cross-project, no X-Project header required)
GET /api/v1/agents/running
GET /api/v1/agents/running?limit=50

# Recent agents (cross-project, no X-Project header required)
GET /api/v1/agents/recent
GET /api/v1/agents/recent?limit=10

# Session messages (paginated, lazy-loaded, filterable by category)
GET /api/v1/sessions/:id/messages
GET /api/v1/sessions/:id/messages?limit=100&offset=0
GET /api/v1/sessions/:id/messages?category=subagent  # text|tool|subagent|skill

# Session prompt context (returns generated prompt for an agent session)
GET /api/v1/sessions/:id/prompt  # 200 with {prompt_context}, 204 if empty, 404 if not found

# Dependencies
GET /api/v1/tickets/:id/dependencies  # Get ticket dependencies
POST /api/v1/dependencies             # Add dependency
DELETE /api/v1/dependencies           # Remove dependency

# Chain executions (require X-Project header)
GET    /api/v1/chains              # List chains (?status=&epic_ticket_id= filters)
POST   /api/v1/chains              # Create chain (pending); optional ordered_ticket_ids for custom order
POST   /api/v1/chains/preview      # Preview: expanded ticket_ids, deps map, added_by_deps
GET    /api/v1/chains/:id          # Get chain with items + deps map
PATCH  /api/v1/chains/:id          # Update pending chain; optional ordered_ticket_ids
POST   /api/v1/chains/:id/start    # Start sequential execution
POST   /api/v1/chains/:id/cancel   # Cancel chain and release locks
POST   /api/v1/chains/:id/append   # Append tickets to running chain

# Documentation
GET /api/v1/docs/agent-manual      # Agent manual markdown content

# Logs
GET /api/v1/logs                   # Log file contents (?type=be|fe, default be)

# Other
GET /api/v1/search?q=              # Full-text search
GET /api/v1/status                 # Dashboard summary
GET /api/v1/daily-stats            # Daily stats (tickets, tokens, agent time) per project
GET /api/v1/ws                     # WebSocket for real-time updates (broadcast)
GET /api/v1/pty/:session_id        # PTY WebSocket (1:1 interactive terminal relay)
```

## Common Tasks

### Modifying API Endpoints

1. Update handlers in `be/internal/api/`
2. Update routes in `server.go`
3. Consider if the same logic should be in socket handler
4. **Documentation updates:**
   - This file — update endpoint listing
   - `ui/CLAUDE.md` — update API Endpoints section
   - `ui/src/api/` — update corresponding API client
   - `ui/src/types/` — update TypeScript types if needed
