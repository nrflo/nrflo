# API Package

HTTP API server providing REST endpoints and WebSocket for the web UI.

## Server Architecture

`nrflo_server serve` provides:
- **HTTP API** on port 6587 — web UI, REST API, WebSocket
- **CORS** disabled by default (same-origin serving); configurable via `cors_origins` in config file. `X-Request-ID` is exposed and allowed via CORS headers
- **Request ID** middleware generates a trx (`logger.NewTrx()`) per HTTP request, injects it into context via `logger.WithTrx()`, and sets `X-Request-ID` response header
- **WebSocket** at `/api/v1/ws` for real-time updates

## Handler File Mapping

| File | Endpoints |
|------|-----------|
| `server.go` | Server setup, CORS, route registration, orchestrator init |
| `handlers_tickets.go` | Ticket list/create/get |
| `handlers_tickets_update.go` | Ticket update/delete/close/reopen |
| `handlers_workflow.go` | Workflow state get/patch |
| `handlers_orchestrate.go` | Ticket-scoped run/stop/restart/retry-failed/take-control/resume-session/exit-interactive/run-epic. Run and retry-failed validate ticket is not closed or blocked (409 Conflict) |
| `handlers_project_workflow.go` | Project-scoped run/stop/restart/retry-failed/take-control/resume-session/exit-interactive/state/agents |
| `handlers_pty.go` | PTY WebSocket handler: upgrade, validate session, spawn/relay PTY, handle resize, exit-interactive on process exit. Also supports viewer-attach for `status=running` sessions when a PTY session already exists (interactive CLI backend); disconnect does not trigger exit-interactive in that case. |
| `handlers_workflow_def.go` | Workflow definition CRUD (no `phases` field; phases derived from agent_definitions) |
| `handlers_agent_def.go` | Agent definition CRUD (accepts `layer` field for phase execution order) |
| `handlers_system_agent_def.go` | System agent definition CRUD (global, no project scope) |
| `handlers_cli_models.go` | CLI model CRUD (global, no project scope, readonly delete enforcement, enabled toggle: 400 on system model, 409 on in-use) |
| `handlers_cli_model_check.go` | CLI model health check (POST /api/v1/cli-models/{id}/test) |
| `handlers_default_template.go` | Default template CRUD (global, no project scope, readonly enforcement) |
| `handlers_scheduled_tasks.go` | Scheduled task CRUD + run-now + list-runs (project-scoped via X-Project header) |
| `handlers_notification_channels.go` | Notification channel CRUD + /test + deliveries list (project-scoped via X-Project header); secrets masked in responses |
| `handlers_chains.go` | Chain preview/list/get/create/update/start/cancel/delete/append/remove-items |
| `handlers_git.go` | Git commit history list/detail |
| `handlers_daily_stats.go` | Daily stats endpoint |
| `handlers_global_settings.go` | Global settings GET/PATCH (no project scope) |
| `handlers_tool_definitions.go` | Tool definitions CRUD (global, no project scope; ?project_id and ?workflow_id list filters) |
| `handlers_tool_definitions_register.go` | POST /api/v1/tool-definitions/register — bearer-auth-gated idempotent upsert + safe prune of global tool definitions |
| `handlers_tool_definitions_register_validate.go` | Validation logic for register request entries |
| `handlers_api_credentials.go` | API credentials CRUD (global, no project scope; literal:* secret_ref redacted as literal:*** in responses) |
| `handlers_safety_hook_check.go` | Safety hook dry-run check (POST /api/v1/safety-hook/check, global) |
| `handlers_project_findings.go` | Project findings GET (project-scoped) |
| `handlers_docs.go` | Documentation (agent manual) |
| `handlers_session_prompt.go` | Session prompt context (GET /api/v1/sessions/:id/prompt) |
| `handlers_errors.go` | Error log list (paginated, type filter) |
| `handlers_logs.go` | Backend log file viewer |
| `handlers_nrvapp_review.go` | nrvapp review item CRUD (list, create, get with diff, patch draft, approve, reject); project-scoped via X-Project header; api-mode only |
| `handlers_nrvapp_review_diff.go` | `diffJSON` helper: key-by-key JSON object comparison returning {added, removed, changed} |
| `handlers_nrvapp_config.go` | nrvapp config editor (list files, get content, put content, get history, rollback); project-scoped; api-mode only; builds configeditor.Service per-request from customer_config_dir project setting |
| `handlers_nrvapp_insights.go` | nrvapp insights aggregations (summary, edit-rate, throughput); project-scoped; api-mode only |
| `static_handler.go` | SPA handler: serves embedded UI files with index.html fallback for client-side routing |

## HTTP API Endpoints

```bash
# Projects
GET /api/v1/projects
GET /api/v1/projects/:id
POST /api/v1/projects
PATCH /api/v1/projects/:id
DELETE /api/v1/projects/:id

# Tickets (require X-Project header or ?project= param)
GET /api/v1/tickets                          # Paginated: ?page=&per_page=&sort_by=&sort_order=
GET /api/v1/tickets/:id       # Returns enriched ticket with blockers, blocks, children, parent_ticket, siblings
POST /api/v1/tickets
PATCH /api/v1/tickets/:id
DELETE /api/v1/tickets/:id
POST /api/v1/tickets/:id/close

# Workflow state (ticket-scoped runtime state)
GET /api/v1/tickets/:id/workflow
PATCH /api/v1/tickets/:id/workflow

# Workflow orchestration (run/stop/restart from UI)
POST /api/v1/tickets/:id/workflow/run      # Start orchestrated run; body accepts `interactive` (bool), `plan_mode` (bool), `force` (bool). interactive/plan_mode mutually exclusive (400 if both true). force bypasses concurrent ticket workflow guard (409 without it when worktrees disabled and another ticket workflow running). When interactive/plan_mode set, response includes `session_id` and status `"interactive"` or `"planning"`
POST /api/v1/tickets/:id/workflow/stop     # Stop running orchestration (optional instance_id in body)
POST /api/v1/tickets/:id/workflow/restart       # Restart agent (context save + relaunch)
POST /api/v1/tickets/:id/workflow/retry-failed  # Retry failed workflow from failed layer
POST /api/v1/tickets/:id/workflow/take-control     # Kill agent, return session ID for interactive use
POST /api/v1/tickets/:id/workflow/resume-session   # Resume finished Claude session (set to user_interactive)
POST /api/v1/tickets/:id/workflow/exit-interactive  # Signal interactive session completed, unblock spawner
POST /api/v1/tickets/:id/workflow/run-epic    # Create chain from epic children, optionally start

# Workflow definitions (project-scoped, require X-Project header)
# Note: no `phases` field — phases are derived from agent_definitions at read time
GET    /api/v1/workflows              # List all (response includes phases derived from agent_definitions)
POST   /api/v1/workflows              # Create (accepts id, description, scope_type, groups, close_ticket_on_complete; no phases)
GET    /api/v1/workflows/:id          # Get one (response includes phases derived from agent_definitions)
PATCH  /api/v1/workflows/:id          # Update (accepts description, scope_type, groups, close_ticket_on_complete; no phases)
DELETE /api/v1/workflows/:id          # Delete

# Project-scoped workflow operations
POST /api/v1/projects/:id/workflow/run      # Start project workflow; body accepts `interactive` (bool) and `plan_mode` (bool), mutually exclusive (400 if both true). When set, response includes `session_id` and status `"interactive"` or `"planning"`
POST /api/v1/projects/:id/workflow/stop     # Stop project workflow
POST /api/v1/projects/:id/workflow/restart       # Restart project agent
POST /api/v1/projects/:id/workflow/retry-failed  # Retry failed project workflow
POST /api/v1/projects/:id/workflow/take-control     # Kill project agent, return session ID
POST /api/v1/projects/:id/workflow/resume-session   # Resume finished project Claude session
POST /api/v1/projects/:id/workflow/exit-interactive  # Signal project interactive session completed
POST /api/v1/projects/:id/workflow/stop-endless-loop # Toggle endless-loop stop flag on an active instance
DELETE /api/v1/projects/:id/workflow/:instance_id  # Delete completed/failed project workflow instance (409 if active)
GET  /api/v1/projects/:id/workflow          # Get project workflow state
GET  /api/v1/projects/:id/agents           # Get project agent sessions
GET  /api/v1/projects/:id/findings         # Get all project findings as JSON map

# Git (project-scoped, reads from project root_path)
GET  /api/v1/projects/:id/git/commits           # Paginated commit list (?page=&per_page=)
GET  /api/v1/projects/:id/git/commits/:hash     # Single commit detail with diff

# Agent definitions (nested under workflows)
# Each agent definition includes a `layer` field that determines phase execution order
GET    /api/v1/workflows/:wid/agents           # List agents for workflow (ordered by layer ASC, id ASC)
POST   /api/v1/workflows/:wid/agents           # Create agent definition (accepts `layer` field; validates fan-in rules)
GET    /api/v1/workflows/:wid/agents/:id       # Get agent definition
PATCH  /api/v1/workflows/:wid/agents/:id       # Update agent definition (accepts `layer` field; validates fan-in rules)
DELETE /api/v1/workflows/:wid/agents/:id       # Delete agent definition

# System agent definitions (global, no project scope)
# Request/response includes: id, role, execution_mode (cli|api), model, timeout, prompt,
# tools (CSV), api_max_iterations, restart_threshold, max_fail_restarts,
# stall_start_timeout_sec, stall_running_timeout_sec, created_at, updated_at.
# POST/PATCH: execution_mode defaults to 'cli'; invalid value returns 400.
# Unique constraint on (role, execution_mode); duplicate returns 409.
GET    /api/v1/system-agents           # List all system agent definitions
POST   /api/v1/system-agents           # Create system agent definition
GET    /api/v1/system-agents/:id       # Get system agent definition
PATCH  /api/v1/system-agents/:id       # Update system agent definition
DELETE /api/v1/system-agents/:id       # Delete system agent definition

# Default templates (global, no project scope)
GET    /api/v1/default-templates           # List all default templates (?type= filter: agent, injectable)
POST   /api/v1/default-templates           # Create default template (always non-readonly)
GET    /api/v1/default-templates/:id       # Get default template
PATCH  /api/v1/default-templates/:id       # Update default template (readonly: template-only, 400 if name provided)
DELETE /api/v1/default-templates/:id       # Delete default template (403 if readonly)
POST   /api/v1/default-templates/:id/restore  # Restore readonly template to original default_template text (400 if non-readonly)

# CLI models (global, no project scope)
GET    /api/v1/cli-models           # List all CLI models
POST   /api/v1/cli-models           # Create CLI model (always non-readonly)
GET    /api/v1/cli-models/:id       # Get CLI model
PATCH  /api/v1/cli-models/:id       # Update CLI model
DELETE /api/v1/cli-models/:id       # Delete CLI model (400 if readonly system model)
POST   /api/v1/cli-models/:id/test  # Health check: spawn minimal agent, return success/error/duration_ms

# Global settings (no project scope)
GET    /api/v1/settings           # Returns {"low_consumption_mode": bool, "context_save_via_agent": bool, "session_retention_limit": int, "stall_start_timeout_sec": int|null, "stall_running_timeout_sec": int|null, "api_mode_enabled": bool (read-only, reflects --mode=api flag)}
PATCH  /api/v1/settings           # Accepts {"low_consumption_mode": bool, "context_save_via_agent": bool, "session_retention_limit": int (>= 10), "stall_start_timeout_sec": int|null (>= 0), "stall_running_timeout_sec": int|null (>= 0)}; api_mode_enabled is silently ignored on PATCH

# Safety hook check (global, no project scope)
POST   /api/v1/safety-hook/check  # Dry-run command against safety hook config. Body: {config: SafetyHookConfig, command: string}. Returns {allowed: bool, reason: string}

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
GET /api/v1/sessions/:id/messages?category=subagent  # text|tool|subagent|skill|user_input

# Session prompt context (returns generated prompt for an agent session)
GET /api/v1/sessions/:id/prompt  # 200 with {prompt, system_prompt}, 204 when both are empty, 404 if not found

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
DELETE /api/v1/chains/:id              # Delete chain (409 if running, cascades items+locks)
POST   /api/v1/chains/:id/append       # Append tickets to running chain
POST   /api/v1/chains/:id/remove-items # Remove pending items from running chain

# Documentation
GET /api/v1/docs/agent-manual      # Agent manual markdown content

# Logs
GET /api/v1/logs                   # Log file contents (?type=be, default be; ?filter=<string> searches full file case-insensitive, no 1000-line cap)

# Tool definitions (global, no project scope; routes registered only when --mode=api; return 404 in cli mode)
GET    /api/v1/tool-definitions           # ?project_id= and ?workflow_id= filter the list
POST   /api/v1/tool-definitions           # Create (id, name, endpoint, input_schema required; input_schema must be valid JSON)
GET    /api/v1/tool-definitions/{id}
PUT    /api/v1/tool-definitions/{id}
DELETE /api/v1/tool-definitions/{id}
POST   /api/v1/tool-definitions/register  # Idempotent upsert manifest + safe prune; bearer auth via NRFLO_REGISTER_TOKEN env var; 503 when unset; 401 on bearer mismatch; body: {tools: [{name,endpoint,input_schema,auth_method?,auth_ref?,timeout_sec?,description?}]}; prunes global tools absent from payload unless name is matched by any agent_definitions.tools pattern (literal, prefix*, or *)

# API credentials (global, no project scope; routes registered only when --mode=api; return 404 in cli mode)
GET    /api/v1/api-credentials            # secret_ref values starting with literal: are redacted as literal:***
POST   /api/v1/api-credentials            # secret_ref must start with env:|file:|literal:
GET    /api/v1/api-credentials/{id}
PUT    /api/v1/api-credentials/{id}       # Plaintext literal:* is accepted on input; never returned on output
DELETE /api/v1/api-credentials/{id}

# nrvapp (api-mode only; all require X-Project header)

## Review items
GET    /api/v1/nrvapp/review                    # List review items; ?status=pending|approved|rejected, ?limit=, ?offset=
POST   /api/v1/nrvapp/review                    # Create review item; body: {tool_name, input, session_id?, output?, draft?}
GET    /api/v1/nrvapp/review/{id}               # Get one; includes diff field when draft is set ({added,removed,changed})
PATCH  /api/v1/nrvapp/review/{id}               # Update draft; body: {draft}; broadcasts nrvapp.review_updated
POST   /api/v1/nrvapp/review/{id}/approve       # Approve (copies draft→output if output empty); broadcasts nrvapp.review_updated
POST   /api/v1/nrvapp/review/{id}/reject        # Reject; body: {reason}; broadcasts nrvapp.review_updated

## Config editor (customer_config_dir must be set in project settings)
GET    /api/v1/nrvapp/config/files                        # List managed files (manifest + tool config_files + disk yaml/json)
GET    /api/v1/nrvapp/config/content/{file...}            # Get file content (DB version if edited, else disk fallback) + version number
PUT    /api/v1/nrvapp/config/content/{file...}            # Update file; raw body = new content; validates against sidecar schema; broadcasts nrvapp.config_updated
GET    /api/v1/nrvapp/config/history/{file...}            # Version history (newest first)
POST   /api/v1/nrvapp/config/rollback/{file...}           # Rollback; body: {version: int}; creates new version with old content; broadcasts nrvapp.config_updated

## Insights
GET    /api/v1/nrvapp/insights/summary          # Dispatch stats + review counts; ?range=7d|30d (default 7d)
GET    /api/v1/nrvapp/insights/edit-rate        # Per-tool review outcomes with edit_rate_pct; ?range=7d|30d
GET    /api/v1/nrvapp/insights/throughput       # Bucketed dispatch counts; ?range=7d|30d; ?bucket=1h|6h|1d (default 1h for 7d, 6h for 30d)

# Notification channels (require X-Project header)
GET    /api/v1/notification-channels              # List channels (configs masked)
POST   /api/v1/notification-channels              # Create; body: {name, kind, enabled?, config?, event_types?}
GET    /api/v1/notification-channels/{id}         # Get one (config masked)
PATCH  /api/v1/notification-channels/{id}         # Partial update; masked secrets preserved if echoed back
DELETE /api/v1/notification-channels/{id}         # Delete
POST   /api/v1/notification-channels/{id}/test    # Enqueue synthetic test delivery; returns {status:"queued"}
GET    /api/v1/notification-deliveries            # ?channel_id= (required) + ?limit=; newest first

# Scheduled tasks (require X-Project header)
GET    /api/v1/scheduled-tasks              # List all scheduled tasks for project
POST   /api/v1/scheduled-tasks              # Create; body: {id?, name, description?, cron_expression, workflows, enabled?}; validates cron + project-scope workflows; 400/409
GET    /api/v1/scheduled-tasks/{id}         # Get one
PATCH  /api/v1/scheduled-tasks/{id}         # Partial update; same validations as Create
DELETE /api/v1/scheduled-tasks/{id}         # Delete (cascades schedule_runs)
GET    /api/v1/scheduled-tasks/{id}/runs    # Paginated run history (?limit=&offset=)
POST   /api/v1/scheduled-tasks/{id}/run-now # Dispatch immediately; returns inserted ScheduleRun

# Errors (require X-Project header or ?project= param)
GET /api/v1/errors                 # Paginated: ?page=&per_page=&type= (agent|workflow|system)

# Other
GET /api/v1/search?q=              # Full-text search
GET /api/v1/status                 # Dashboard summary
GET /api/v1/daily-stats            # Daily stats (tickets, tokens, agent time) per project; ?range=today|week|month|all (default: today)
GET /api/v1/ws                     # WebSocket for real-time updates (broadcast)
GET /api/v1/pty/:session_id        # PTY WebSocket (1:1 interactive terminal relay)
```

## Endless Loop Mode (Project Scope)

`POST /api/v1/projects/{id}/workflow/run` accepts `endless_loop: bool` alongside the existing `interactive`/`plan_mode`/`instructions` body. Validation (HTTP 400 on failure):
- Rejects `endless_loop=true` combined with `interactive=true` or `plan_mode=true` (mutually exclusive).
- Rejects `endless_loop=true` when the workflow definition's `scope_type != "project"`.
- Server clears `instructions` when `endless_loop=true` before forwarding to the orchestrator.

`POST /api/v1/projects/{id}/workflow/stop-endless-loop` — body `{"instance_id": "...", "stop": true|false}`:
- 200: instance exists, belongs to the project, status is `active`, and `endless_loop=1`. Updates `workflow_instances.stop_endless_loop_after_iteration` and broadcasts `EventWorkflowUpdated`. The flag only affects the next restart check; the in-flight iteration is not interrupted.
- 404: instance not found or not owned by the project.
- 400: instance is not active, or the workflow is not in endless-loop mode.

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
