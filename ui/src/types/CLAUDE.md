# Types

TypeScript type definitions matching Go API models. Contains 3 files.

## Key Ticket Types (`ticket.ts`)

| Type | Description |
|------|-------------|
| `Ticket` | Base ticket with `parent_ticket_id?: string \| null` |
| `PendingTicket` | Extends `Ticket` with `is_blocked` and `blocked_by` fields |
| `TicketListResponse` | `{ tickets: PendingTicket[] }` — list endpoint returns PendingTicket |
| `SearchResponse` | `{ tickets: PendingTicket[], query: string }` — search also returns PendingTicket |
| `StatusResponse` | Includes `counts.blocked` for sidebar badge |

## Key Workflow Types (`workflow.ts`)

| Type | Description |
|------|-------------|
| `ScopeType` | `'ticket' \| 'project'` — workflow scope type |
| `WorkflowState` | Phase states, phase_order, scope_type, instance_id, findings, active_agents map (constructed server-side from `workflow_instances` + `agent_sessions` tables) |
| `WorkflowResponse` | API response with agent_history at top level (ticket-scoped) |
| `ProjectWorkflowResponse` | API response for project-scoped workflows. `all_workflows` keyed by instance_id (not workflow name). Multiple concurrent instances allowed. Each state includes `instance_id` and `workflow` fields. Stop/restart/retry API calls include `instance_id` to target a specific instance. |
| `ProjectAgentSessionsResponse` | API response for project-scoped agent sessions (project_id + sessions array) |
| `AgentHistoryEntry` | Agent execution record (agent_id, agent_type, model_id, phase, duration, result, context_left) |
| `CompletedAgentRow` | Extends `AgentHistoryEntry` with `workflow_label: string` for unified completed agents table |
| `AgentSession` | Session record with `workflow_instance_id`, `result`, `result_reason`, `pid`, `findings`, `started_at`, `ended_at`, `last_messages`, `message_count`, `context_left` |
| `WorkflowFindings` | `Record<string, Record<string, unknown>>` (agent_type → field → value) |

## Chain Types (`chain.ts`)

| Type | Description |
|------|-------------|
| `ChainExecution` | Chain execution record |
| `ChainExecutionItem` | Individual chain item |
| `ChainStatus` | Chain lifecycle status |

## Type Safety

- Types in `src/types/` must match the Go API models
- Use `z.infer<typeof schema>` for form types (see TicketForm)
- API responses are typed — check `src/api/tickets.ts`
