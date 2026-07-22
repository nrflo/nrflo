# refinery Package

Per-console-session sidecar that folds WS-driven events into a bounded working-set digest.

## Invariant

Event-driven, never polling: `Manager` (`manager.go`) is a `ws.Listener` routed by `Event.ProjectID` to per-session `sidecar`s (`sidecar.go`), which coalesce triggers behind a `>=30s` `clock.After` debounce (never `time.Sleep`) and fold immediately on `orchestration.completed`/`failed`. A fold (`fold.go`) is a single direct `provider.Run` call — no spawned `agent_sessions` row, no `workflow_instance` — resolved from the `_refinery` `system_agent_definitions` row (`role='refinery'`, `execution_mode='api'`) via `SystemAgentDefinitionService.GetForBackend`. The digest is keyed to the console-chat `agent_sessions` id (not the engine), so it survives engine rotation within that chat, and is capped to 4KB before the single-row `repo.RefineryDigestRepo.Upsert`. `runFoldCore` (`fold.go`) rejects empty/whitespace-only output or a degenerate `StopReason` (`max_tokens`/`refusal`) by returning `ok=false`, so both callers skip their write and the autonomous path leaves `lastFoldedCount` unadvanced for re-fold on the next trigger. `refinery_enabled` (global default off, per-console-chat opt-in) gates `Manager.Start`/`Stop` calls entirely in `console.ChatService` — this package has no gate of its own.

## Import Hygiene

`refinery` imports `service`/`repo`/`ws`/`spawner/apirun/provider`/`clock`/`logger`/`model`/`foldfmt` only. `service` and `spawner` must never import `refinery` back — the `WorkingSetInjector` (spawner) reads the digest through the concrete `repo.RefineryDigestRepo.Get`, so it depends on `repo`, not this package.

## Autonomous Sidecar

`Manager.StartSession`/`StopSession` (`session_sidecar.go`) run a per-autonomous-session sidecar driven by the spawner lifecycle (start on `cli_interactive` spawn, stop before `FinalizeSessionCost`), gated on `refinery_autonomous_enabled` (default ON). Each fold reads the `agent_messages` delta since the last fold (not console event strings) and folds via the shared `runFoldCore` (`fold.go`), writing to a relaunch-stable digest keyed by `(workflow_instance_id, node_id)` in `refinery_autonomous_digests` — the same slot survives a kill->relaunch chain. Fold token usage is attributed to the folding session's running cost via the injected `costAttributor` seam (`SetCostAttributor`, = `spawner.AddSessionCostUsage`). A successful `UpsertSlot` also broadcasts a project-scoped `agent.handoff_digest` WS event (`session_id` in payload) via the injected `broadcaster` seam (`SetBroadcaster`, = `ws.Hub.Broadcast`); debounce is inherited from the fold cadence, so no extra per-slot timer is needed. `GET /api/v1/sessions/{id}/handoff-digest` (`api/handlers_handoff_digest.go`) reads the durable slot row directly via `repo.RefineryDigestRepo.GetSlot`.
