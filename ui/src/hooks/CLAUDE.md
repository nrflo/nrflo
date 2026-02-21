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
| `findings.updated` | agent_type, key, action | Findings changed |
| `messages.updated` | session_id, agent_type, model_id | Messages changed (~2s) |
| `workflow.updated` | action (init, set) | Workflow state changed |
| `workflow_def.*` | workflow_id | Workflow def CRUD |
| `agent_def.*` | workflow_id, agent_id | Agent def CRUD |
| `agent.take_control` | session_id, agent_type | Agent entered interactive mode |
| `orchestration.*` | instance_id | Orchestration lifecycle |
| `chain.updated` | chain_id | Chain state changed |
| `ticket.updated` | | Ticket state changed |
| `global.running_agents` | | Running agents changed (global broadcast, no subscription scope) |

All v2 events include: `type`, `project_id`, `ticket_id`, `workflow`, `timestamp`, `protocol_version`, `sequence`
**Exception:** `global.running_agents` is a global broadcast with no project_id/ticket_id/seq. Handled as early return before `dispatchV2Event`.

## Other Hooks

| Hook | Purpose |
|------|---------|
| `useTickets.ts` | TanStack Query hooks for ticket data, query key factory |
| `useProjects.ts` | TanStack Query hook for projects |
| `useChains.ts` | TanStack Query hooks for chain executions |
| `useElapsedTime.ts` | Elapsed time hooks |
| `useGoBack.ts` | History-aware back navigation |
| `useTakeControl()` | Mutation: take interactive control of running Claude agent (ticket-scoped) |
| `useExitInteractive()` | Mutation: exit interactive session, unblock spawner (ticket-scoped) |
| `useTakeControlProject()` | Project-scoped variant of useTakeControl |
| `useExitInteractiveProject()` | Project-scoped variant of useExitInteractive |
| `useRunningAgents.ts` | TanStack Query hook for global running agents (`GET /api/v1/agents/running`), 30s polling fallback, 5s stale time. WS `global.running_agents` invalidates cache. |

## Testing

Tests co-located with hook files using `.test.ts` suffix. Run: `npx vitest run src/hooks/`
