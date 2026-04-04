# API Client

API client modules for communicating with the nrflow backend. Contains 12 files.

## Architecture

- All API calls go through `client.ts` which provides a configured fetch wrapper
- Project selection is managed via `X-Project` header (or `?project=` query parameter)
- TanStack Query handles caching and refetching in the hooks layer
- Vite proxies `/api` to the backend in development (including WebSocket via `ws: true`)
- Projects are loaded from `/api/v1/projects` endpoint

## API Modules

| Module | Purpose |
|--------|---------|
| `client.ts` | Base API client with X-Project header support |
| `projects.ts` | Project API functions |
| `tickets.ts` | Ticket and workflow API functions |
| `workflows.ts` | Workflow definition and orchestration API functions |
| `projectWorkflows.ts` | Project-scoped workflow API functions (run/stop/get/restart/delete/agent sessions/findings) |
| `agentDefs.ts` | Agent definition API client |
| `chains.ts` | Chain execution API functions (list/get/create/update/start/cancel/append/runEpicWorkflow) |
| `docs.ts` | Documentation API functions (getAgentManual) |
| `settings.ts` | Global settings API (GET/PATCH /api/v1/settings, low consumption mode, session retention limit, stall start/running timeouts) |
| `agents.ts` | Agent API functions (fetchRecentAgents, fetchRunningAgents, fetchSessionMessages, fetchSessionPrompt) |
| `systemAgentDefs.ts` | System agent definition CRUD (global, no X-Project header) |
| `defaultTemplates.ts` | Default template CRUD (global, no X-Project header) |
| `cliModels.ts` | CLI model CRUD + health check test (global, no X-Project header) |
| `errors.ts` | Paginated error log list (`GET /api/v1/errors?page=&per_page=&type=`) |
| `projects.ts` | Also exports `checkSafetyHook()` for dry-run safety hook check (no X-Project header) |

## REST API Endpoints

```
# Projects
GET    /api/v1/projects
GET    /api/v1/projects/:id
POST   /api/v1/projects
PATCH  /api/v1/projects/:id
DELETE /api/v1/projects/:id

# Tickets (require X-Project header or ?project= param)
GET    /api/v1/tickets                          # Paginated: ?page=&per_page=&sort_by=&sort_order=
GET    /api/v1/tickets/:id
POST   /api/v1/tickets
PATCH  /api/v1/tickets/:id
DELETE /api/v1/tickets/:id
POST   /api/v1/tickets/:id/close

# Dependencies
GET    /api/v1/tickets/:id/dependencies

# Workflow state
GET    /api/v1/tickets/:id/workflow
PATCH  /api/v1/tickets/:id/workflow

# Workflow orchestration
POST   /api/v1/tickets/:id/workflow/run           # Start orchestrated run
POST   /api/v1/tickets/:id/workflow/stop          # Stop running orchestration
POST   /api/v1/tickets/:id/workflow/restart       # Restart a running agent
POST   /api/v1/tickets/:id/workflow/retry-failed  # Retry from failed layer

# Project-scoped workflows
GET    /api/v1/projects/:id/workflow              # Get project workflow state
POST   /api/v1/projects/:id/workflow/run          # Start project-scoped workflow
POST   /api/v1/projects/:id/workflow/stop         # Stop project workflow
POST   /api/v1/projects/:id/workflow/restart      # Restart agent in project workflow
POST   /api/v1/projects/:id/workflow/retry-failed # Retry project workflow
GET    /api/v1/projects/:id/agents                # Get project agent sessions
DELETE /api/v1/projects/:id/workflow/:instance_id # Delete project workflow instance
GET    /api/v1/projects/:id/findings              # Get all project findings as JSON map

# Workflow definitions (require X-Project header)
GET    /api/v1/workflows
POST   /api/v1/workflows
GET    /api/v1/workflows/:id
PATCH  /api/v1/workflows/:id
DELETE /api/v1/workflows/:id

# Agent definitions (nested under workflows, require X-Project header)
GET    /api/v1/workflows/:wid/agents
POST   /api/v1/workflows/:wid/agents
GET    /api/v1/workflows/:wid/agents/:id
PATCH  /api/v1/workflows/:wid/agents/:id
DELETE /api/v1/workflows/:wid/agents/:id

# Chain executions (require X-Project header)
GET    /api/v1/chains                             # List (?status=&epic_ticket_id=)
GET    /api/v1/chains/:id                         # Get with items
POST   /api/v1/chains                             # Create (pending)
PATCH  /api/v1/chains/:id                         # Update pending chain
POST   /api/v1/chains/:id/start                   # Start execution
POST   /api/v1/chains/:id/cancel                  # Cancel + release locks
POST   /api/v1/chains/:id/append                  # Append tickets
POST   /api/v1/tickets/:id/workflow/run-epic       # Create chain from epic children

# Agent sessions
GET    /api/v1/tickets/:id/agents
GET    /api/v1/tickets/:id/agents?phase=investigation

# Recent agents (cross-project)
GET    /api/v1/agents/recent
GET    /api/v1/agents/recent?limit=10

# Session messages (paginated)
GET    /api/v1/sessions/:id/messages
GET    /api/v1/sessions/:id/messages?limit=100&offset=0

# Session prompt context
GET    /api/v1/sessions/:id/prompt

# System agent definitions (global, no X-Project header)
GET    /api/v1/system-agents           # List all system agent definitions
POST   /api/v1/system-agents           # Create system agent definition
GET    /api/v1/system-agents/:id       # Get system agent definition
PATCH  /api/v1/system-agents/:id       # Update system agent definition
DELETE /api/v1/system-agents/:id       # Delete system agent definition

# Default templates (global, no X-Project header)
GET    /api/v1/default-templates           # List all default templates
POST   /api/v1/default-templates           # Create default template
GET    /api/v1/default-templates/:id       # Get default template
PATCH  /api/v1/default-templates/:id       # Update default template (readonly: template-only, 400 if name provided)
DELETE /api/v1/default-templates/:id       # Delete default template (403 if readonly)
POST   /api/v1/default-templates/:id/restore  # Restore readonly template to original text (400 if non-readonly)

# CLI Models (global, no X-Project header)
GET    /api/v1/cli-models           # List all CLI models
POST   /api/v1/cli-models           # Create CLI model
GET    /api/v1/cli-models/:id       # Get CLI model
PATCH  /api/v1/cli-models/:id       # Update CLI model (non-readonly only)
DELETE /api/v1/cli-models/:id       # Delete CLI model (non-readonly only)
POST   /api/v1/cli-models/:id/test  # Health check: spawn minimal agent, return success/error/duration

# Global Settings
GET    /api/v1/settings             # Get global settings (low_consumption_mode)
PATCH  /api/v1/settings             # Update global settings

# Documentation
GET    /api/v1/docs/agent-manual    # Agent manual markdown content

# Safety Hook
POST   /api/v1/safety-hook/check    # Dry-run check command against safety hook config (no X-Project header)

# Other
GET    /api/v1/search?q=            # Full-text search
POST   /api/v1/dependencies         # Add dependency
DELETE /api/v1/dependencies         # Remove dependency
GET    /api/v1/status               # Dashboard summary
GET    /api/v1/daily-stats          # Daily stats
GET    /api/v1/ws                   # WebSocket
```

Project is specified via `X-Project` header or `?project=` query parameter.

## Live Tracking

When agents are running, the UI shows real-time messages via WebSocket:

- Agent sessions display: status (running/completed/failed/timeout/continued), model ID, messages loaded from API (newest first)
- Clicking any agent node in PhaseGraph opens a modal with full message history
- The spawner broadcasts `messages.updated` every ~2s during agent execution

### Message Format

The spawner parses JSON stream output and formats messages with tool details:

```
[Bash] git status
[Read] /path/to/file.ts
[Grep] pattern in src/api/
[Skill] skill:jira-ticket REF-12425
[Task] codebase-explorer: Find error handlers
[Edit] /src/main.ts
[stderr] API rate limit warning
```

Both Claude CLI and Opencode output formats are supported with automatic normalization:
- Tool names normalized to title case (`bash` → `Bash`)
- Long messages truncated as `START (300 chars) ... [N chars] ... END (150 chars)`
- Stderr captured with `[stderr]` prefix
- Scanner buffer: 10MB limit for large JSON outputs
