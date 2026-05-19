# Hooks

Custom React hooks for data fetching, WebSocket communication, and shared UI logic.

## State Management

- **Server state**: TanStack Query (`useQuery`, `useMutation`) for all API data.
- **Client state**: Zustand (`src/stores/projectStore.ts`) for project selection only.
- Query keys are defined in `src/hooks/useTickets.ts` — invalidate appropriately on mutations.
- Projects are loaded from API on startup (see `projectStore.ts`).

TanStack Query hooks export a `*Keys` factory + one hook per query/mutation. WS-driven invalidations live in `useWSReducer.ts`.

Hooks live under `ui/src/hooks/`. Run `ls ui/src/hooks/` for the full list.

## WebSocket Protocol v2

The WS layer uses protocol v2 with seq tracking, cursor resume, snapshot hydration, and heartbeat liveness.

### Files

- `useWebSocket.ts` — core connection, message routing, reconnect with cursor resume
- `useWSProtocol.ts` — protocol v2 types: `WSEventV2`, `WSSubscribeMessage`, control event types
- `useWSReducer.ts:1` — event dispatch + seq tracking; per-subscription `seqMap` with idempotency; persists to sessionStorage
- `useWSSnapshot.ts` — snapshot state machine: `idle → receiving → applying`; buffers live events during snapshot
- `useWebSocketSubscription.ts` — consumer hook for ticket-level subscriptions

### Connection

- Connects to `ws://host/api/v1/ws`
- Auto-reconnect with exponential backoff (3s, 6s, 9s...), max 5 attempts
- Events dispatched through `useWSReducer.dispatchV2Event()` (seq tracking + cache invalidation)
- Heartbeat liveness: reconnects if no message received in 60s

### Cursor Resume

On reconnect, subscribe messages include `since_seq` (last applied seq per subscription). Server replays missed events or sends `resync.required` if cursor is too old. Seq state persisted to sessionStorage for tab-refresh resume.

### Snapshot Hydration

Server sends `snapshot.begin` → `snapshot.chunk` (per entity) → `snapshot.end`. During snapshot, live events are buffered and replayed in order after snapshot is applied.

### Control Events

| Type | Description |
|------|-------------|
| `snapshot.begin` | Start of snapshot stream |
| `snapshot.chunk` | Entity data (workflow_state, agent_sessions, findings, ticket_detail, chain_status) |
| `snapshot.end` | End of snapshot, triggers cache application |
| `resync.required` | Server cannot replay from cursor; client re-subscribes with seq=0 |
| `heartbeat` | Server liveness ping |

### Subscribe Protocol

```json
{"action": "subscribe", "project_id": "myproject", "ticket_id": "TICKET-123", "since_seq": 42}
```

Omit `since_seq` for initial subscription (v1 compat). Include `since_seq: 0` to force snapshot.

### Subscription Patterns

**Ticket-scoped:** Components use `useWebSocketSubscription(ticketId)`.

**Project-wide:** `WebSocketProvider` auto-subscribes with empty ticketId.

**Important:** Subscriptions must be gated on `projectsLoaded`. Project ID resolved fresh via `useConnectionsStore.getState().active().activeProject` each time.

## Event Types

Representative sample — canonical list in [be/internal/api/CLAUDE.md](../../../be/internal/api/CLAUDE.md); dispatcher: `useWSReducer.ts:1`.

| Event | Description |
|-------|-------------|
| `agent.started` | Agent spawned |
| `agent.completed` | Agent finished |
| `messages.updated` | Agent messages changed (~2s during execution) |
| `workflow.updated` | Workflow state changed |
| `global.running_agents` | Global broadcast; no project_id/ticket_id/seq |

All v2 events include: `type`, `project_id`, `ticket_id`, `workflow`, `timestamp`, `protocol_version`, `sequence`.

`global.running_agents` is a global broadcast handled as an early return before `dispatchV2Event`.

## Testing

Tests co-located with hook files using `.test.ts` suffix. Run: `make test-ui ARGS="src/hooks/"`
