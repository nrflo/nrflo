# Types

TypeScript type definitions matching Go API models — one module per domain (`ls ui/src/types/`). Each module mirrors its Go model, so read the `.ts` file for the exact field set; this page records only what the types don't say about themselves.

## Key Workflow Types (`workflow.ts`)

| Type | Description |
|------|-------------|
| `ScopeType` | `'ticket' \| 'project'` — workflow scope type |
| `WorkflowState` | Phase states, phase_order, scope_type, instance_id, findings, active_agents map (constructed server-side from `workflow_instances` + `agent_sessions` tables) |
| `WorkflowResponse` | API response with agent_history at top level (ticket-scoped) |
| `ProjectWorkflowResponse` | Project-scoped response. `all_workflows` keyed by instance_id (not workflow name) — multiple concurrent instances allowed; stop/restart/retry calls include `instance_id` to target one |
| `RestartDetail` | Per-restart enrichment: reason, duration_sec, context_left (optional), message_count |
| `AgentHistoryEntry` | Agent execution record (session_id, agent_type, model_id, phase, duration, result, context_left, restart_details, optional effective_mode) |
| `CompletedAgentRow` | Extends `AgentHistoryEntry` with `workflow_label` for the unified completed-agents table |
| `WorkflowFindings` | `Record<string, Record<string, unknown>>` (agent_type → field → value) |
| `ActiveAgentV4` | Optional `effective_mode?: 'cli_interactive'\|'api'\|'script'` sourced from `agent_sessions.effective_mode`; omitted for legacy rows |
| `AgentDef` (`workflow.agentDefs.ts`, re-exported from `workflow.ts`) | `execution_mode` is `'cli_interactive'\|'api'\|'script'`; `model` is a unified model slug whose selected mode must be enabled; optional `python_script_id?`, `node_role?: 'static'\|'planner'\|'fanout_template'`, `description?`, `reasoning_effort?: string \| null` (`null` clears the override → the row's `default_effort`; omitted entirely for script mode) |
| `PlanTemplate` (`plan.ts`) | Fanout-template catalog row shown when revising a plan; carries the effective `reasoning_effort?` the backend resolved |

## Other Modules

| Module | What's non-obvious |
|--------|--------------------|
| `ticket.ts` | `PendingTicket` extends `Ticket` with `is_blocked`/`blocked_by`; both list *and* search return `PendingTicket`; `StatusResponse.counts.blocked` drives the sidebar badge |
| `pythonScript.ts` | Rows discriminated by `kind` (`'agent'\|'tool'`); tool types add `tool_description`/`input_schema`/`timeout_sec`, and `PythonToolUpdateRequest` omits `kind` (backend rejects it) |
| `user.ts` | `User.system` marks seeded users — UsersSection hides their Delete button |
| `agentSessionLogs.ts` | `LiveAgentSession` adds live process stats (`pid`, `rss_kb`, `cpu_pct`) plus optional `rate_limit_*` fields |
| `artifact.ts` | `Artifact.source` is `'input'\|'agent'`; uploads are two-step (`ArtifactUploadResponse.upload_id` → `InputArtifactRef` at launch) |
| `stepwise.ts` | Per-node step cursor progress mirroring `be/internal/service/stepwise_read.go`; `rotated` is orthogonal to `status` |
| `schedules.ts`, `chain.ts`, `errors.ts`, `audit.ts` | Plain records plus `{ items/errors, total, page, per_page }`-shaped paginated responses |

## Type Safety

- Types in `src/types/` must match the Go API models
- Use `z.infer<typeof schema>` for form types (see TicketForm)
- API responses are typed — check `src/api/tickets.ts`
