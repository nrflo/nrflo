# Hooks

Custom React hooks for data fetching, WebSocket communication, and shared UI logic.

## State Management

- **Server state**: TanStack Query (`useQuery`, `useMutation`) for all API data
- **Client state**: Zustand (`src/stores/projectStore.ts`) for project selection only
- Query keys are defined in `src/hooks/useTickets.ts` — invalidate appropriately on mutations
- Projects are loaded from API on startup (see `projectStore.ts`)

## WebSocket Protocol v2

The WS layer uses protocol v2 with seq tracking, cursor resume, snapshot hydration, and heartbeat liveness.

### Files

| File | Purpose |
|------|---------|
| `useWebSocket.ts` | Core connection, message routing, reconnect with cursor resume |
| `useWSProtocol.ts` | Protocol v2 types: `WSEventV2`, `WSSubscribeMessage`, control event types, entity types |
| `useWSReducer.ts` | Event dispatch + seq tracking. Per-subscription `seqMap` with idempotency. Persists to sessionStorage. |
| `useWSSnapshot.ts` | Snapshot state machine: `idle → receiving → applying`. Buffers live events during snapshot, replays after. |
| `useWebSocketSubscription.ts` | Consumer hook for ticket-level subscriptions |

### Connection

- Connects to `ws://host/api/v1/ws`
- Auto-reconnect with exponential backoff (3s, 6s, 9s...), max 5 attempts
- Events dispatched through `useWSReducer.dispatchV2Event()` (seq tracking + cache invalidation)
- Heartbeat liveness: reconnects if no message received in 60s

### Cursor Resume

On reconnect, subscribe messages include `since_seq` (last applied seq per subscription). Server replays missed events or sends `resync.required` if cursor is too old. Seq state persisted to sessionStorage for tab-refresh resume.

### Snapshot Hydration

Server sends `snapshot.begin` → `snapshot.chunk` (per entity) → `snapshot.end`. During snapshot, live events are buffered. After snapshot applied, buffered events are replayed in order.

### Control Events

| Type | Description |
|------|-------------|
| `snapshot.begin` | Start of snapshot stream |
| `snapshot.chunk` | Entity data (workflow_state, agent_sessions, findings, ticket_detail, chain_status) |
| `snapshot.end` | End of snapshot, triggers cache application |
| `resync.required` | Server cannot replay from cursor, client should re-subscribe with seq=0 |
| `heartbeat` | Server liveness ping |

### Subscribe Protocol

```json
{"action": "subscribe", "project_id": "myproject", "ticket_id": "TICKET-123", "since_seq": 42}
```

Omit `since_seq` for initial subscription (v1 compat). Include `since_seq: 0` to force snapshot.

### Subscription Patterns

**Ticket-scoped:** Components use `useWebSocketSubscription(ticketId)`.

**Project-wide:** `WebSocketProvider` auto-subscribes with empty ticketId.

**Important:** Subscriptions must be gated on `projectsLoaded`. Project ID resolved fresh via `getProject()` each time.

## Event Types

| Event | Data Fields | Description |
|-------|-------------|-------------|
| `agent.started` | agent_id, agent_type, model_id, session_id, phase | Agent spawned |
| `agent.completed` | agent_id, result, result_reason, model_id | Agent finished |
| `agent.continued` | agent_id, model_id | Agent relaunched |
| `agent.context_updated` | session_id, context_left | Context window updated |
| `agent.context_saving` | session_id, agent_type | Context-saver system agent spawned for low-context save |
| `findings.updated` | agent_type, key, action | Findings changed |
| `project_findings.updated` | | Project findings changed |
| `project.env_vars_updated` | | Project env vars changed; invalidates `projectEnvVarKeys.list(project_id)` |
| `messages.updated` | session_id, agent_type, model_id | Messages changed (~2s) |
| `workflow.updated` | action (init, set) | Workflow state changed |
| `workflow_def.*` | workflow_id | Workflow def CRUD |
| `agent_def.*` | workflow_id, agent_id | Agent def CRUD |
| `agent.take_control` | session_id, agent_type | Agent entered interactive mode |
| `agent.nudged` | session_id, agent_type, attempt, max | Agent idle reminder sent; invalidates workflow + agentSessions queries |
| `orchestration.*` | instance_id | Orchestration lifecycle |
| `layer.skipped` | instance_id, layer, skip_tag, agents | Layer skipped due to skip tag |
| `merge.conflict_resolving` | instance_id, branch, merge_error | Merge conflict detected, resolver agent spawned |
| `merge.conflict_resolved` | instance_id, branch | Conflict resolver succeeded |
| `merge.conflict_failed` | instance_id, branch, error | Conflict resolver failed |
| `chain.updated` | chain_id | Chain state changed |
| `ticket.updated` | | Ticket state changed |
| `global.running_agents` | | Running agents changed (global broadcast, no subscription scope) |
| `error.created` | | New error recorded, invalidates error query cache |
| `schedule.created` | | Scheduled task created, invalidates scheduleKeys.all |
| `schedule.updated` | | Scheduled task updated, invalidates scheduleKeys.all |
| `schedule.deleted` | | Scheduled task deleted, invalidates scheduleKeys.all |
| `schedule.triggered` | task_id, run_id, status | Schedule dispatched, invalidates scheduleKeys.all + scheduleKeys.runs(task_id) |
| `notification_channel.created` | | Channel created, invalidates notification-channels |
| `notification_channel.updated` | | Channel updated, invalidates notification-channels |
| `notification_channel.deleted` | | Channel deleted, invalidates notification-channels |
| `notification.delivered` | channel_id (string) | Delivery succeeded, invalidates notification-channels + notification-deliveries(channel_id) |
| `notification.failed` | channel_id (string) | Delivery failed, invalidates notification-channels + notification-deliveries(channel_id) |
| `review.created` | | New review item, invalidates `['review']` |
| `review.updated` | | Review item updated, invalidates `['review']` |
| `config_file.updated` | | Config file changed, invalidates `['config-files']` |
| `tool.dispatched` | | Tool dispatch completed, invalidates `['insights']` with 1s leading+trailing throttle |

All v2 events include: `type`, `project_id`, `ticket_id`, `workflow`, `timestamp`, `protocol_version`, `sequence`
**Exception:** `global.running_agents` is a global broadcast with no project_id/ticket_id/seq. Handled as early return before `dispatchV2Event`.

## Other Hooks

| Hook | Purpose |
|------|---------|
| `useTickets.ts` | TanStack Query hooks for ticket data, query key factory |
| `useProjects.ts` | TanStack Query hook for projects |
| `useChains.ts` | TanStack Query hooks for chain executions. `useChain` supports optional `refetchInterval` for running chain polling fallback. `useRemoveFromChain` mutation for removing pending items. |
| `useScheduledTasks.ts` | TanStack Query hooks for scheduled tasks and run history. Key factory: `scheduleKeys`. Exports: `useScheduledTasks`, `useScheduleRuns(taskId, page)`, `useCreateScheduledTask`, `useUpdateScheduledTask`, `useDeleteScheduledTask`, `useRunScheduleNow`. All mutations invalidate `scheduleKeys.all`. |
| `useWorkflowChains.ts` | TanStack Query hooks for workflow chain definitions. Key factory: `workflowChainKeys` (static `all`, `lists(project)`, `detail(project, id)`). Exports: `useWorkflowChainsList`, `useWorkflowChain(id)`, `useCreateWorkflowChain`, `useUpdateWorkflowChain`, `useDeleteWorkflowChain`, `useAppendStep`, `useUpdateStep`, `useDeleteStep`, `useReorderSteps`. All mutations invalidate `workflowChainKeys.all`. WS events `chain_def.created/updated/deleted` invalidate cache. |
| `useElapsedTime.ts` | Elapsed time hooks |
| `useGoBack.ts` | History-aware back navigation |
| `useTakeControl()` | Mutation: take interactive control of running Claude agent (ticket-scoped) |
| `useExitInteractive()` | Mutation: exit interactive session, unblock spawner (ticket-scoped) |
| `useTakeControlProject()` | Project-scoped variant of useTakeControl |
| `useExitInteractiveProject()` | Project-scoped variant of useExitInteractive |
| `useRunningAgents.ts` | TanStack Query hook for global running agents (`GET /api/v1/agents/running`), 30s polling fallback, 5s stale time. WS `global.running_agents` invalidates cache. |
| `useIsMobile.ts` | Media query hook for mobile detection (`max-width: 639px`). Used by PhaseGraph for responsive layout and touch interactions. |
| `useDeleteProjectWorkflowInstance()` | Mutation: delete a project workflow instance (failed or completed) |
| `useSessionPrompt.ts` | TanStack Query hook for fetching session prompt context (lazy, staleTime: Infinity) |
| `useProjectFindings()` | TanStack Query hook for project findings (`GET /api/v1/projects/:id/findings`). Invalidated by `project_findings.updated` WS event. Defined in `useTickets.ts`. |
| `useCLIModels()` | TanStack Query hook for CLI models (`GET /api/v1/cli-models`). Defined in `useCLIModels.ts`. |
| `useReview.ts` | TanStack Query hooks for review items: `reviewKeys` factory rooted at `['review',...]`; `useReviewItems`, `useReviewItem`, `useUpdateReviewDraft`, `useApproveReview`, `useRejectReview`. |
| `useConfigFiles.ts` | TanStack Query hooks for config files: `configFileKeys` factory rooted at `['config-files',...]`; `useConfigFiles`, `useConfigFile`, `usePutConfigFile`, `useConfigHistory`, `useRollbackConfig`. |
| `useInsights.ts` | TanStack Query hooks for insights: `insightsKeys` factory rooted at `['insights',...]`; `useInsightsSummary`, `useInsightsEditRate`, `useInsightsThroughput`. |
| `useModelOptions()` | Derives `DropdownOptionGroup[]` from `useCLIModels()` data, grouped by `cli_type` with provider-prefixed labels (e.g., "Claude: Opus"). Groups and options sorted alphabetically. Unknown `cli_type` values fall back to capitalized name. Used by AgentForm, AgentDefForm. Defined in `useCLIModels.ts`. |
| `useErrors.ts` | TanStack Query hook for paginated error logs (`GET /api/v1/errors`). Key factory: `errorKeys`. Invalidated by `error.created` WS event. |
| `useAgentSessionLogs.ts` | TanStack Query hooks for agent sessions. Key factory: `agentSessionLogKeys` with `all`, `list(params)`, `live(projectId)`. `useAgentSessionLogs(params)`: paginated finished sessions (`GET /api/v1/agent-session-logs`); `agentSessionLogKeys.all` invalidated on `agent.completed` WS event. `useLiveAgentSessions()`: live sessions (`GET /api/v1/agent-session-logs/live`); staleTime Infinity, no auto-refresh, no refetchOnWindowFocus. `useKillAgentSession()`: mutation (`POST /api/v1/agent-sessions/{id}/kill`); on success invalidates `agentSessionLogKeys.live(project)` + `agentSessionLogKeys.all`. |
| `usePythonScripts.ts` | TanStack Query hooks for Python scripts. Key factory: `pythonScriptKeys`. Exports: `usePythonScripts()`, `usePythonScript(id)`, `useCreatePythonScript()`, `useUpdatePythonScript()`, `useDeletePythonScript()`, `useValidatePythonScript()`. All write mutations invalidate `pythonScriptKeys.all`. |
| `useUsers.ts` | TanStack Query hooks for user management (admin-only, no X-Project): `userKeys` factory; `useUsers()`, `useCreateUser()`, `useUpdateUser()`, `useResetUserPassword()`, `useDeleteUser()`. All mutations invalidate `userKeys.all`. Errors propagate as `ApiError` for `email_exists`/`last_admin`/`cannot_delete_self` mapping at call site. |
| `useAuditLog.ts` | TanStack Query hook for paginated audit log (admin-only): `auditKeys` factory; `useAuditLog({page,per_page,user_id,action})`. Keyed by all params so filter changes trigger refetch. |
| `useProjectEnvVars.ts` | TanStack Query hooks for per-project env vars. Key factory: `projectEnvVarKeys` (`all`, `list(projectId)`). Exports: `useProjectEnvVars(projectId)`, `usePutProjectEnvVar()`, `useDeleteProjectEnvVar()`. Mutations invalidate `projectEnvVarKeys.list(projectId)` on success. Invalidated by `project.env_vars_updated` WS event. |

## Testing

Tests co-located with hook files using `.test.ts` suffix. Run: `make test-ui ARGS="src/hooks/"`
