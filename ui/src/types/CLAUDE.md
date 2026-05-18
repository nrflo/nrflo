# Types

TypeScript type definitions matching Go API models. Contains 6 files.

## Artifact Types (`artifact.ts`)

| Type | Description |
|------|-------------|
| `Artifact` | Artifact record with `id`, `project_id`, `workflow_instance_id`, `name`, `type`, `size_bytes`, `content_type?`, `source` (`'input'|'agent'`), `created_by_session?`, `created_at` |
| `ArtifactUploadResponse` | `{ upload_id, name }` — response from `POST /artifact-uploads` |
| `InputArtifactRef` | `{ upload_id, name? }` — reference passed in `input_artifacts` at launch |

## Key Ticket Types (`ticket.ts`)

| Type | Description |
|------|-------------|
| `Ticket` | Base ticket with `parent_ticket_id?: string \| null` |
| `PendingTicket` | Extends `Ticket` with `is_blocked` and `blocked_by` fields |
| `TicketListResponse` | `{ tickets: PendingTicket[], total_count, page, per_page, total_pages }` — paginated list endpoint |
| `SearchResponse` | `{ tickets: PendingTicket[], query: string }` — search also returns PendingTicket |
| `StatusResponse` | Includes `counts.blocked` for sidebar badge |

## Python Script Types (`pythonScript.ts`)

| Type | Description |
|------|-------------|
| `PythonScript` | Python script record with `id`, `project_id`, `name`, `description`, `kind` (`'agent'\|'tool'`), `code`, `file_path`, tool fields (`tool_description`, `input_schema`, `timeout_sec`), timestamps |
| `PythonScriptCreateRequest` | `{ kind: 'agent', name, description?, code, file_path? }` |
| `PythonScriptUpdateRequest` | partial update for agent kind (`name?`, `description?`, `code?`, `file_path?`) |
| `PythonToolCreateRequest` | `{ kind: 'tool', name, description?, tool_description, input_schema, timeout_sec, code?, file_path? }` |
| `PythonToolUpdateRequest` | partial update for tool kind (no `kind` field — backend rejects) |
| `ValidationResult` | `{ ok, error?, line?, col? }` — syntax check result from `/validate` endpoint |

## Key Workflow Types (`workflow.ts`)

| Type | Description |
|------|-------------|
| `ScopeType` | `'ticket' \| 'project'` — workflow scope type |
| `WorkflowState` | Phase states, phase_order, scope_type, instance_id, findings, active_agents map (constructed server-side from `workflow_instances` + `agent_sessions` tables) |
| `WorkflowResponse` | API response with agent_history at top level (ticket-scoped) |
| `ProjectWorkflowResponse` | API response for project-scoped workflows. `all_workflows` keyed by instance_id (not workflow name). Multiple concurrent instances allowed. Each state includes `instance_id` and `workflow` fields. Stop/restart/retry API calls include `instance_id` to target a specific instance. |
| `ProjectAgentSessionsResponse` | API response for project-scoped agent sessions (project_id + sessions array) |
| `RestartDetail` | Per-restart enrichment: reason, duration_sec, context_left (optional), message_count |
| `AgentHistoryEntry` | Agent execution record (agent_id, agent_type, model_id, phase, duration, result, context_left, restart_details, optional effective_mode) |
| `CompletedAgentRow` | Extends `AgentHistoryEntry` with `workflow_label: string` for unified completed agents table |
| `AgentSession` | Session record with `workflow_instance_id`, `result`, `result_reason`, `pid`, `findings`, `started_at`, `ended_at`, `last_messages`, `message_count`, `context_left` |
| `WorkflowFindings` | `Record<string, Record<string, unknown>>` (agent_type → field → value) |
| `ActiveAgentV4` | Active agent record. Optional `effective_mode?: 'cli_interactive'\|'api'\|'script'` sourced from `agent_sessions.effective_mode`; omitted for legacy rows. |
| `AgentDef` | Agent definition. `execution_mode` is `'cli_interactive'\|'api'\|'script'`; includes optional `python_script_id?: string` |

## Chain Types (`chain.ts`)

| Type | Description |
|------|-------------|
| `ChainExecution` | Chain execution record |
| `ChainExecutionItem` | Individual chain item |
| `ChainStatus` | Chain lifecycle status |

## Error Types (`errors.ts`)

| Type | Description |
|------|-------------|
| `ErrorLog` | Error record with `id`, `project_id`, `error_type` (agent/workflow/system), `instance_id`, `message`, `created_at` |
| `ErrorsResponse` | `{ errors: ErrorLog[], total, page, per_page, total_pages }` — paginated error list |

## Schedule Types (`schedules.ts`)

| Type | Description |
|------|-------------|
| `ScheduledTask` | Scheduled task with cron expression, project-scoped workflow list, enabled flag, last/next run timestamps |
| `ScheduleRun` | Single run record with status, triggered_at, per-workflow instance_id and error |
| `ScheduleRunWorkflow` | Per-workflow result within a run (workflow name, instance_id, optional error) |
| `ScheduleRunStatus` | `'pending' \| 'triggered' \| 'running' \| 'failed'` |
| `ScheduledTaskCreateRequest` / `ScheduledTaskUpdateRequest` | CRUD request types |

## User Types (`user.ts`)

| Type | Description |
|------|-------------|
| `User` | User record with id, email, display_name, role (admin\|viewer), status (active\|disabled), must_change_password, timestamps, optional last_login_at, optional system (bool — seeded system users; Delete button hidden in UsersSection when true) |
| `UserListResponse` | `{ users: User[] }` — list endpoint response |
| `CreateUserRequest` | `{ email, display_name, password, role }` |
| `UpdateUserRequest` | `{ display_name?, role?, status? }` — all optional partial update |
| `ResetPasswordRequest` | `{ new_password }` |

## Agent Session Log Types (`agentSessionLogs.ts`)

| Type | Description |
|------|-------------|
| `AgentSessionLogEntry` | Finished session row with `session_id`, `project_id`, `agent_type`, `model_id?`, `status`, `started_at?`, `ended_at?`, `duration_sec?`, `workflow_id`, `workflow_instance_id`, `scheduled`, `execution_mode?`, `workflow_final_result?` |
| `AgentSessionLogsResponse` | `{ sessions: AgentSessionLogEntry[], total, page, per_page, total_pages }` — paginated list |
| `LiveAgentSession` | Live session row with `session_id`, `project_id`, `agent_type`, `model_id?`, `workflow_id`, `workflow_instance_id`, `scheduled`, `execution_mode?`, `started_at?`, `duration_sec`, `pid`, `rss_kb`, `cpu_pct`, `os_uptime_sec`; optional rate-limit fields: `rate_limit_until_ts?`, `rate_limit_wait_seconds?`, `rate_limit_total_wait_seconds?`, `rate_limit_matched_pattern?`, `rate_limit_retry_count?` |
| `LiveAgentSessionsResponse` | `{ sessions: LiveAgentSession[] }` — live sessions list |

## Audit Types (`audit.ts`)

| Type | Description |
|------|-------------|
| `AuditEntry` | Audit log row: id, user_id (optional), action, resource_type, resource_id, ip, user_agent, metadata (JSON string), created_at |
| `AuditListResponse` | `{ items: AuditEntry[], total, page, per_page }` — paginated list |

## Type Safety

- Types in `src/types/` must match the Go API models
- Use `z.infer<typeof schema>` for form types (see TicketForm)
- API responses are typed — check `src/api/tickets.ts`
