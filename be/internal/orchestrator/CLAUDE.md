# Orchestrator Package

Server-side workflow orchestration. Groups phases by layer and executes layers sequentially, with concurrent agent spawning within each layer.

## Layer-Based Parallel Execution

```
┌─────────────────────────────────────────────────────────────────────┐
│                    LAYER-BASED AGENT EXECUTION                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Orchestrator groups phases by layer, executes layers sequentially   │
│       │                                                              │
│       ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  LAYER 0: [agent-a, agent-b]  (concurrent)                   │    │
│  │    ┌────────────────┐  ┌────────────────┐                    │    │
│  │    │ spawner.Spawn  │  │ spawner.Spawn  │  (one goroutine    │    │
│  │    │ (agent-a)      │  │ (agent-b)      │   per agent)       │    │
│  │    └───────┬────────┘  └───────┬────────┘                    │    │
│  │            └───────────┬───────┘                              │    │
│  │                        ▼                                      │    │
│  │  Fan-in: wait for ALL agents in layer to finish               │    │
│  │    ├── pass_count >= 1 → proceed to next layer               │    │
│  │    ├── all skipped → proceed to next layer                   │    │
│  │    └── pass_count == 0 → fail workflow, stop                 │    │
│  └────────────────────────┬────────────────────────────────────┘    │
│                           ▼                                          │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  LAYER 1: [agent-c]  (single, fan-in convergence)            │    │
│  │    ┌────────────────┐                                        │    │
│  │    │ spawner.Spawn  │                                        │    │
│  │    │ (agent-c)      │                                        │    │
│  │    └───────┬────────┘                                        │    │
│  └────────────┼────────────────────────────────────────────────┘    │
│               ▼                                                      │
│  All layers done → workflow completed                                │
│                                                                      │
│  VALIDATION RULES:                                                   │
│    - layer field required (integer >= 0)                             │
│    - parallel field rejected (breaking change)                       │
│    - fan-in: multi-agent layer → next layer must have 1 agent       │
│    - string-only phase entries rejected                              │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Fan-In Rules

- All agents in a layer run concurrently (one goroutine per `spawner.Spawn()` call)
- Layer completes when ALL agents finish
- `pass_count >= 1` → layer passes, proceed to next
- All agents skipped → layer passes
- `pass_count == 0` → workflow fails

## Connection Pool

The orchestrator's `runLoop` creates a shared `*db.Pool` for the entire workflow run and passes it to all spawners via `spawner.Config.Pool`. This avoids per-call `db.Open()`/`Close()` overhead in the spawner.

## Callback Flow

A later-layer agent (e.g., qa-verifier) can trigger a callback to re-run an earlier layer:

1. Agent calls `nrworkflow agent callback` with `--level <N>`
2. Orchestrator saves `_callback` metadata to workflow instance findings
3. Phases/sessions from the target layer forward are reset
4. Execution loop jumps back to the target layer
5. Target agent's prompt includes `${CALLBACK_INSTRUCTIONS}`
6. After target layer succeeds, `_callback` is cleared
7. Max 3 callbacks per workflow run

## Chain Runner

`chain_runner.go` handles sequential chain execution:

- Runs chain items one at a time via the orchestrator
- Each item is a ticket+workflow pair
- On item completion: marks item done, starts next
- On item failure: marks chain failed, releases locks
- Crash recovery: zombie running chains marked failed on server startup
- Uses `ws.EventChainUpdated` constant for WebSocket broadcasts

## Files

| File | Purpose |
|------|-------|
| `orchestrator.go` | Layer-grouped concurrent phase execution, cancellation support |
| `chain_runner.go` | Sequential chain execution runner |

## Ticket Status Management

The orchestrator manages ticket status transitions for ticket-scoped workflows:

- **Start**: `SetInProgress()` on workflow start (only transitions open → in_progress)
- **Complete**: `Close()` on workflow completion (sets status to closed with reason)
- **Fail/Cancel**: `Reopen()` on workflow failure or manual stop (reverts to open)

Each transition broadcasts `ws.EventTicketUpdated` so the frontend sidebar updates without page refresh. Project-scoped workflows skip ticket status changes entirely.

## Testing

Unit tests in-package:
- `orchestrator_ticket_status_test.go` — SetInProgress, markFailed ticket reopen, WS broadcasts
- `orchestrator_mark_completed_test.go` — markCompleted ticket close, project scope, WS broadcasts

Integration tests in `internal/integration/`:
- `chain_epic_test.go` — chain epic auto-close
- `run_epic_workflow_test.go` — run-epic endpoint tests
