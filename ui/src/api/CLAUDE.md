# API Client

API client modules for communicating with the nrflo backend. Contains 13 files.

## Architecture

- All API calls go through `client.ts` which provides a configured fetch wrapper
- Project selection is managed via `X-Project` header (or `?project=` query parameter)
- TanStack Query handles caching and refetching in the hooks layer
- Vite proxies `/api` to the backend in development (including WebSocket via `ws: true`)
- Projects are loaded from `/api/v1/projects` endpoint

## API Modules

| Module | Purpose |
|--------|---------|
| `client.ts` | Base API client with X-Project header support, `credentials:'include'`, `UnauthenticatedError`/`ForbiddenError` subclasses, global 401 handler via `set401Handler()` |
| `auth.ts` | Auth API: `login(email, password)`, `logout()`, `getMe()`, `changePassword(current, next)` — wraps apiFetch on `/api/v1/auth/*` |
| `projects.ts` | Project API functions |
| `tickets.ts` | Ticket and workflow API functions |
| `workflows.ts` | Workflow definition and orchestration API functions |
| `projectWorkflows.ts` | Project-scoped workflow API functions (run/stop/get/restart/delete/agent sessions/findings) |
| `agentDefs.ts` | Agent definition API client |
| `chains.ts` | Chain execution API functions (list/get/create/update/start/cancel/delete/append/removeFromChain/runEpicWorkflow) |
| `docs.ts` | Documentation API functions (getAgentManual) |
| `settings.ts` | Global settings API (GET/PATCH /api/v1/settings, low consumption mode, context save via agent, session retention limit, stall start/running timeouts). `GlobalSettings.api_mode_enabled` (bool, read-only — server ignores on PATCH) reflects the `--mode=api` startup flag. |
| `agents.ts` | Agent API functions (fetchRecentAgents, fetchRunningAgents, fetchSessionMessages, fetchSessionPrompt) |
| `systemAgentDefs.ts` | System agent definition CRUD (global, no X-Project header) |
| `defaultTemplates.ts` | Default template CRUD (global, no X-Project header) |
| `cliModels.ts` | CLI model CRUD + health check test (global, no X-Project header) |
| `errors.ts` | Paginated error log list (`GET /api/v1/errors?page=&per_page=&type=`) |
| `scheduledTasks.ts` | Scheduled task CRUD + run history (`GET/POST/PATCH/DELETE /api/v1/scheduled-tasks`, `GET /api/v1/scheduled-tasks/:id/runs`, `POST /api/v1/scheduled-tasks/:id/run-now`; requires X-Project header) |
| `notifications.ts` | Notification channel CRUD + test + deliveries (`GET/POST/PATCH/DELETE /api/v1/notification-channels(/:id)`, `POST /api/v1/notification-channels/:id/test`, `GET /api/v1/notification-deliveries?channel_id=&limit=`; requires X-Project header) |
| `nrvapp.ts` | Vertical app API: review CRUD (list/get/update-draft/approve/reject), config files (list/get/put/history/rollback), insights (summary/edit-rate/throughput). `putConfigFile` sends raw text body with `Content-Type: text/plain`. Path segments encoded individually via `encodePathSegments`. |
| `projects.ts` | Also exports `checkSafetyHook()` for dry-run safety hook check (no X-Project header) |

## Global 401 Handler

`client.ts` exports `set401Handler(fn)`. When any request (except `POST /api/v1/auth/login`) returns 401:
1. Throws `UnauthenticatedError` (subclass of `ApiError`)
2. Calls the registered handler with `window.location.pathname + window.location.search`

`AuthGate` registers this handler on first mount. The handler calls `useAuthStore.getState().clear()` and navigates to `/login?next=<encoded path>` via `window.history.pushState` + popstate event, unless already on `/login`.

403 responses throw `ForbiddenError` without calling the handler. Both `UnauthenticatedError` and `ForbiddenError` are exported from `client.ts`.

## REST API Endpoints

```
# Auth (no X-Project header; login is the only public route)
POST /api/v1/auth/login           # Body: {email, password}. Returns {user: User}. Rate-limited: 5/5min per IP+email (429 + Retry-After)
POST /api/v1/auth/logout          # Returns 204
GET  /api/v1/auth/me              # Returns {user: User}
POST /api/v1/auth/change-password # Body: {current_password, new_password}. Returns 204

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
DELETE /api/v1/chains/:id                         # Delete chain (not running)
POST   /api/v1/chains/:id/append                  # Append tickets
POST   /api/v1/chains/:id/remove-items             # Remove pending items
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

# Session prompt context — returns { prompt: string, system_prompt: string }; 204 → both empty strings
GET    /api/v1/sessions/:id/prompt

# System agent definitions (global, no X-Project header)
GET    /api/v1/system-agents           # List all system agent definitions
POST   /api/v1/system-agents           # Create system agent definition
GET    /api/v1/system-agents/:id       # Get system agent definition
PATCH  /api/v1/system-agents/:id       # Update system agent definition
DELETE /api/v1/system-agents/:id       # Delete system agent definition

# Default templates (global, no X-Project header)
GET    /api/v1/default-templates           # List all default templates (?type= filter: agent, injectable)
POST   /api/v1/default-templates           # Create default template (type defaults to 'agent')
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
GET    /api/v1/daily-stats          # Daily stats; ?range=today|week|month|all (default: today)
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
