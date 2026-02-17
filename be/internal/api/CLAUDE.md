# API Package

HTTP API server providing REST endpoints and WebSocket for the web UI.

## Server Architecture

`nrworkflow_server serve` provides:
- **HTTP API** on port 6587 — web UI, REST API, WebSocket
- **CORS** enabled for `http://localhost:5173`
- **WebSocket** at `/api/v1/ws` for real-time updates

## Handler File Mapping

| File | Endpoints |
|------|-----------|
| `server.go` | Server setup, CORS, route registration, orchestrator init |
| `handlers_tickets.go` | Ticket list/create/get |
| `handlers_tickets_update.go` | Ticket update/delete/close/reopen |
| `handlers_workflow.go` | Workflow state get/patch |
| `handlers_orchestrate.go` | Ticket-scoped run/stop/restart/retry-failed/run-epic |
| `handlers_project_workflow.go` | Project-scoped run/stop/restart/retry-failed/state/agents |
| `handlers_workflow_def.go` | Workflow definition CRUD |
| `handlers_agent_def.go` | Agent definition CRUD |
| `handlers_chains.go` | Chain list/get/create/update/start/cancel/append |
| `handlers_git.go` | Git commit history list/detail |
| `handlers_daily_stats.go` | Daily stats endpoint |
| `handlers_docs.go` | Documentation (agent manual) |

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
POST /api/v1/tickets/:id/workflow/run      # Start orchestrated run
POST /api/v1/tickets/:id/workflow/stop     # Stop running orchestration
POST /api/v1/tickets/:id/workflow/restart       # Restart agent (context save + relaunch)
POST /api/v1/tickets/:id/workflow/retry-failed  # Retry failed workflow from failed layer
POST /api/v1/tickets/:id/workflow/run-epic    # Create chain from epic children, optionally start

# Workflow definitions (project-scoped, require X-Project header)
GET    /api/v1/workflows              # List all
POST   /api/v1/workflows              # Create
GET    /api/v1/workflows/:id          # Get one
PATCH  /api/v1/workflows/:id          # Update
DELETE /api/v1/workflows/:id          # Delete

# Project-scoped workflow operations
POST /api/v1/projects/:id/workflow/run      # Start project workflow
POST /api/v1/projects/:id/workflow/stop     # Stop project workflow
POST /api/v1/projects/:id/workflow/restart       # Restart project agent
POST /api/v1/projects/:id/workflow/retry-failed  # Retry failed project workflow
GET  /api/v1/projects/:id/workflow          # Get project workflow state
GET  /api/v1/projects/:id/agents           # Get project agent sessions

# Git (project-scoped, reads from project root_path)
GET  /api/v1/projects/:id/git/commits           # Paginated commit list (?page=&per_page=)
GET  /api/v1/projects/:id/git/commits/:hash     # Single commit detail with diff

# Agent definitions (nested under workflows)
GET    /api/v1/workflows/:wid/agents           # List agents for workflow
POST   /api/v1/workflows/:wid/agents           # Create agent definition
GET    /api/v1/workflows/:wid/agents/:id       # Get agent definition
PATCH  /api/v1/workflows/:wid/agents/:id       # Update agent definition
DELETE /api/v1/workflows/:wid/agents/:id       # Delete agent definition

# Agent sessions
GET /api/v1/tickets/:id/agents
GET /api/v1/tickets/:id/agents?phase=investigation

# Recent agents (cross-project, no X-Project header required)
GET /api/v1/agents/recent
GET /api/v1/agents/recent?limit=10

# Session messages (paginated, lazy-loaded)
GET /api/v1/sessions/:id/messages
GET /api/v1/sessions/:id/messages?limit=100&offset=0

# Dependencies
GET /api/v1/tickets/:id/dependencies  # Get ticket dependencies
POST /api/v1/dependencies             # Add dependency
DELETE /api/v1/dependencies           # Remove dependency

# Chain executions (require X-Project header)
GET    /api/v1/chains              # List chains (?status=&epic_ticket_id= filters)
POST   /api/v1/chains              # Create chain (pending)
GET    /api/v1/chains/:id          # Get chain with items
PATCH  /api/v1/chains/:id          # Update pending chain
POST   /api/v1/chains/:id/start    # Start sequential execution
POST   /api/v1/chains/:id/cancel   # Cancel chain and release locks
POST   /api/v1/chains/:id/append   # Append tickets to running chain

# Documentation
GET /api/v1/docs/agent-manual      # Agent manual markdown content

# Other
GET /api/v1/search?q=              # Full-text search
GET /api/v1/status                 # Dashboard summary
GET /api/v1/daily-stats            # Daily stats (tickets, tokens, agent time) per project
GET /api/v1/ws                     # WebSocket for real-time updates
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
