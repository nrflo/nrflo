# WebSocket Package

Real-time event broadcasting via WebSocket with protocol v2 support.

## Protocol v2

### Event Envelope
Every WS event includes:
- `protocol_version` — currently 2
- `seq` — monotonic sequence from ws_event_log autoincrement
- `type` — event type string
- `project_id`, `ticket_id`, `workflow` — scope identifiers
- `entity` — entity type (used in snapshot chunks)
- `timestamp` — RFC3339 UTC
- `data` — JSON payload

### Control Events
- `snapshot.begin` — starts a snapshot stream (data: `chunk_count`)
- `snapshot.chunk` — typed section of snapshot data (entity field identifies type)
- `snapshot.end` — ends snapshot (data: `current_seq`)
- `resync.required` — client must request full resync
- `heartbeat` — server liveness signal with latest seq

### Client Subscribe with Cursor
`{ action: "subscribe", project_id: "...", ticket_id: "...", since_seq: 42 }`

When `since_seq` is provided:
1. Server queries ws_event_log for events after `since_seq`
2. If events found: replay them in order, then join live stream
3. If cursor too old (events pruned): send snapshot if provider configured, else `resync.required`
4. If `since_seq` is 0: send snapshot for initial hydration

### Backward Compatibility
v1 clients (no `since_seq`) continue working unchanged — no replay, no snapshot, just live events.

## Files

| File | Purpose |
|------|---------|
| `hub.go` | Client management, event log integration, broadcasting |
| `client.go` | WebSocket connection, read/write pumps, subscribe handling |
| `handler.go` | HTTP upgrade handler |
| `protocol.go` | Protocol v2 constants and entity types |
| `replay.go` | Cursor-based replay from event log |
| `snapshot.go` | Snapshot streaming (begin/chunk/end) |
| `backpressure.go` | Client queue depth monitoring |
| `testing.go` | Test helpers (NewTestClient) |

## Global Broadcast

`BroadcastGlobal(event)` sends an event to ALL connected clients regardless of subscription scope. Used for cross-project signal events like `global.running_agents`. These events are ephemeral — not persisted to the event log and not eligible for cursor-based replay.

The spawner emits `global.running_agents` whenever an agent starts or completes. The frontend refetches running agents via `GET /api/v1/agents/running` on receipt.

## Architecture

Events flow: Producer → Hub.Broadcast → EventLogRepo.Append (assigns seq) → broadcastEvent → clients.
Global events flow: Producer → Hub.BroadcastGlobal → broadcastGlobalEvent → ALL clients (no event log).
Replay flow: Client subscribe with since_seq → handleReplay → EventLogRepo.QuerySince → stream to client.
Snapshot flow: Cursor too old or since_seq=0 → streamSnapshot → SnapshotProvider.BuildSnapshot → chunks to client.
Listener fan-out: After broadcastEvent stamps seq, a single goroutine iterates all registered Listeners and calls OnEvent — never on the broadcast loop, so a slow listener cannot stall the WS pipeline.

## Listener Extension Point

`Hub.RegisterListener(l Listener)` registers an out-of-band receiver for every broadcast event. Must be called before `Hub.Run()`. The `internal/notify` Dispatcher is registered here.

Fan-out is non-blocking: a goroutine is spawned per broadcast, iterating all listeners sequentially. Slow or blocking OnEvent implementations affect only each other, never the WS broadcast pipeline.

## Notification Event Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `EventNotificationChannelCreated` | `notification_channel.created` | Channel created |
| `EventNotificationChannelUpdated` | `notification_channel.updated` | Channel updated |
| `EventNotificationChannelDeleted` | `notification_channel.deleted` | Channel deleted |
| `EventNotificationDelivered` | `notification.delivered` | Delivery sent successfully |
| `EventNotificationFailed` | `notification.failed` | Delivery giving up (3 attempts exhausted) |

## Nrvapp Event Constants

| Constant | Value | Data fields | Description |
|----------|-------|-------------|-------------|
| `EventNrvappDispatchCompleted` | `nrvapp.dispatch_completed` | `tool_name`, `status` (success\|error), `duration_ms`, `dispatch_id` | Emitted after every manifest tool invocation (success or error) |
| `EventNrvappReviewCreated` | `nrvapp.review_created` | `review_item_id`, `tool_name` | Emitted when a manifest tool with `review:true` creates a new review item |
| `EventNrvappReviewUpdated` | `nrvapp.review_updated` | — | Reserved for review item status changes (approve/reject) |
| `EventNrvappConfigUpdated` | `nrvapp.config_updated` | — | Reserved for customer config file edits |
