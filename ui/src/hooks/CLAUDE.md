# Hooks

Custom React hooks for data fetching, WebSocket communication, and shared UI logic. Contains 13 files.

## State Management

- **Server state**: TanStack Query (`useQuery`, `useMutation`) for all API data
- **Client state**: Zustand (`src/stores/projectStore.ts`) for project selection only
- Query keys are defined in `src/hooks/useTickets.ts` — invalidate appropriately on mutations
- Projects are loaded from API on startup (see `projectStore.ts`)

## WebSocket Hook

`useWebSocket.ts` provides real-time updates via WebSocket. The UI uses WebSocket exclusively — no REST polling.

### Connection

- Connects to `ws://host/api/v1/ws`
- Auto-reconnect with exponential backoff (3s, 6s, 9s...), max 5 attempts
- Events automatically invalidate relevant TanStack Query caches
- The spawner broadcasts `messages.updated` every ~2s while agents run

### Subscription Patterns

**Ticket-scoped subscription:**
```typescript
import { useProjectStore } from '@/stores/projectStore'

const { subscribe, unsubscribe } = useWebSocket()
const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
const currentProject = useProjectStore((s) => s.currentProject)

useEffect(() => {
  if (ticketId && projectsLoaded) {
    subscribe(ticketId)
    return () => unsubscribe(ticketId)
  }
}, [ticketId, projectsLoaded, currentProject, subscribe, unsubscribe])
```

**Project-wide subscription:** `Layout.tsx` subscribes to all project events (empty ticketId) so that Sidebar status counts, ticket lists, and Dashboard receive real-time updates (e.g., `ticket.updated` events).

**Project-scoped workflow events:** Events from project-scoped workflows have empty `ticket_id` and non-empty `project_id`. The WebSocket hook detects this and invalidates `projectWorkflowKeys` instead of `ticketKeys`. `ProjectWorkflowsPage` subscribes with empty ticketId to receive these events.

**Important:** Subscriptions must be gated on `projectsLoaded` to avoid subscribing with the wrong project ID. The hook stores only `ticketId`s internally and resolves the project ID fresh via `getProject()` each time it sends a message.

### WebSocket Protocol

**Subscribe to a ticket:**
```json
{"action": "subscribe", "project_id": "myproject", "ticket_id": "TICKET-123"}
```

**Subscribe to all tickets in project:**
```json
{"action": "subscribe", "project_id": "myproject", "ticket_id": ""}
```

**Unsubscribe:**
```json
{"action": "unsubscribe", "project_id": "myproject", "ticket_id": "TICKET-123"}
```

## Event Types

| Event | Data Fields | Description |
|-------|-------------|-------------|
| `agent.started` | agent_id, agent_type, model_id, session_id, phase | Agent spawned |
| `agent.completed` | agent_id, result, result_reason, model_id | Agent finished |
| `agent.continued` | agent_id, model_id | Agent relaunched for continuation |
| `phase.started` | phase | Phase began |
| `phase.completed` | phase, result | Phase finished |
| `findings.updated` | agent_type, key, action | Findings changed |
| `messages.updated` | session_id, agent_type, model_id | Agent messages changed (~2s interval) |
| `workflow.updated` | action (init, set) | Workflow state changed |
| `workflow_def.created` | workflow_id | Workflow definition created |
| `workflow_def.updated` | workflow_id | Workflow definition updated |
| `workflow_def.deleted` | workflow_id | Workflow definition deleted |
| `agent_def.created` | workflow_id, agent_id | Agent definition created |
| `agent_def.updated` | workflow_id, agent_id | Agent definition updated |
| `agent_def.deleted` | workflow_id, agent_id | Agent definition deleted |
| `orchestration.started` | instance_id | Orchestrated workflow run started |
| `orchestration.completed` | instance_id | Orchestrated workflow run completed |
| `orchestration.failed` | instance_id, reason | Orchestrated workflow run failed |
| `orchestration.retried` | instance_id | Failed workflow retry started |
| `orchestration.callback` | instance_id, from_layer, to_layer, instructions | Callback detected, re-executing from target layer |
| `chain.updated` | chain_id | Chain state changed |
| `ticket.updated` | | Ticket state changed |

All events include: `type`, `project_id`, `ticket_id`, `workflow`, `timestamp`

### Event Sources

- Events from **agent CLI commands** (workflow.init, phase.start/complete, findings.*, agent.complete/fail/kill) are received via the socket handler and broadcast via the in-process WebSocket hub.
- Events from **spawner/orchestrator** (agent.started, messages.updated, agent.completed, phase.started, phase.completed) are broadcast directly via the in-process WebSocket hub.

## Other Hooks

| Hook | Purpose |
|------|---------|
| `useTickets.ts` | TanStack Query hooks for ticket data fetching, query key factory |
| `useProjects.ts` | TanStack Query hook for projects |
| `useChains.ts` | TanStack Query hooks for chain executions (chainKeys factory, polling, append mutation) |
| `useElapsedTime.ts` | Elapsed time hooks (`useElapsedTime`, `useTickingClock`) |
| `useGoBack.ts` | History-aware back navigation (navigate(-1) with fallback path) |

## Testing

Tests are co-located with hook files using `.test.ts` suffix. Some hooks have tests in a `__tests__/` subdirectory.

Run tests: `npx vitest run src/hooks/`
