# Orchestrator Package

Server-side workflow orchestration. Groups phases by layer and executes layers sequentially, with concurrent agent spawning within each layer.

## Layer-Based Parallel Execution

- Phases grouped by `layer` integer; layers execute in ascending order, sequentially.
- All agents within a layer run concurrently (one goroutine per `spawner.Spawn()` call).
- Layer completes when all agents finish; pass policy evaluated via `denom = passCount + failCount` (skipped excluded).
- All-skipped (`denom == 0`) → layer passes regardless of policy; entry point: `orchestrator_loop.go` `runLoop()`.
- Layer execution, callback plans, retry, and skip partitioning key on `node_id`; model/tag/prompt resolution stays on `agent_type` — see [db/CLAUDE.md](../db/CLAUDE.md).

## Layer Aggregation

- **Denominator rule**: `denom = passCount + failCount` (skipped agents excluded)
- **Callback agents** count as pass (added to `passCount` before policy check)
- **Policy check** (`denom > 0`): `passCount >= policy.Required(denom)` or workflow fails

### Fan-In Pass Policies (per-layer, stored in `workflow_layer_policies`)

| Policy | Required passes |
|--------|----------------|
| `any` (default) | 1 |
| `all` | all agents (denom) |
| `quorum:N` | exactly N |
| `percent:P` | `ceil(denom * P / 100)` |

Policies load at start via `WorkflowLayerPolicyService`; missing entries default to `any`.

## Error Capture

Errors recorded to the `errors` table via `errorSvc` on workflow failure and merge-resolution failure.

## Model Config Loading

Loaded from `cli_models` at workflow start via `loadModelConfigs()`; passed to all spawners as `ModelConfigs` (`orchestrator_lifecycle.go`).

## Safety Hook Threading

`claude_safety_hook` project config → `BuildSafetySettingsJSON()` → threaded through all spawn paths; read once at start (`orchestrator_start.go`).

## Callback Flow

Agents trigger callbacks via `nrflo agent callback` with `--level` (whole-layer), `--agent` (single-agent), or `--chain` (sequential named agents). Flag shapes and restrictions: [doc/common-40-workflow.md](../../../doc/common-40-workflow.md#callback-mechanism).

All callbacks from a settled layer are collected and processed through the plan engine (`orchestrator_callback_plan.go`):

1. **Decompose**: each `CallbackError` → `decomposedRequest` (mode-specific step list, resetScope, resumeLayer).
2. **Merge**: `mergeCallbackPlans` unions steps by layer (whole-layer wins over per-agent), dedupes resetScope, takes max resumeLayer.
3. **Reset once**: `asRepo.ResetAgentSessionsInWorkflow(wfiID, plan.resetScope)` — single call excludes running/continued sessions.
4. **Execute steps**: `runLoop` drains `plan.steps[callbackPlanIdx]` via `spawnPhases` before forward iteration; each whole-layer step uses the pass policy; non-whole-layer steps (agent/chain) fail the workflow if they return a callback.
5. **Resume**: when `callbackPlanIdx` reaches `len(plan.steps)`, `layerIdx` advances to `layerIndexOf(plan.resumeLayer)`, `_callback` findings are cleared, and forward iteration resumes.

Cap: `maxCallbacks = 10` cumulative agent spawns per workflow run (whole-layer steps count len(phases), per-agent steps count len(nodes)). Exceeding the cap fails the workflow. Subset/chain plan steps cannot themselves emit callbacks (v1 restriction).

## Layer-Skip Logic

Skipping is **per-agent**, not all-or-nothing per layer. Before spawning each layer, `applyLayerSkips()` (`orchestrator_skip.go`) partitions the layer's phases against `skip_tags` (reloaded from DB each layer — agents may add tags concurrently via `c.skip(tag)`):

1. Each agent whose `agent_definitions.tag` is in `skip_tags` gets an `agent_sessions` row with `status=skipped`, `result=skipped`, plus a per-agent `EventAgentCompleted` (result=skipped) broadcast.
2. The remaining (runnable) agents still spawn; the skipped subset is excluded from `results`, so `denom = pass+fail` already excludes them in layer aggregation.
3. Only when **every** agent in the layer matches a skip tag does `applyLayerSkips` return `wholeLayerSkipped=true` — `EventLayerSkipped` is broadcast and the loop advances past the layer (counts as passed).

## Consult

`Orchestrator.Consult(ctx, callerSessionID, consultantID, question)` (`consult.go`) is the synchronous consult entry point. It resolves the caller session context, enforces the socket-boundary recursion guard (consultants cannot initiate a consult), builds an api-capable `spawner.Config`, then delegates to `Spawner.Consult`.

## Sub-Workflow Runner

`subworkflow_runner.go`: `StartSubworkflow` starts a `callable_as_subworkflow` def as a detached project-scoped child (persisted `parent_instance_id`/`subworkflow_depth`; `launch_depth` carries the chain cap), enforcing purge-off/no-pause defs, depth/children caps (`service/subworkflow.go`) and a persisted invocation budget (`subworkflow_starts`, atomic across pause/continue/retry). Sub-runs never fire next-on-success. The watcher (`subworkflow_watch.go`) stops children only when the parent is TERMINAL (pause re-arms on the successor runState; retry/continue re-arm via `rearmSubworkflowWatcher`). `GetSubworkflow` returns running/waiting/completed/failed via `GetSessionFindingByKey`; only the matching `parent_instance_id` caller may read a child. Exposed as `run_subworkflow`/`get_subworkflow` via `Config.Subworkflows`.

## Automatic Merge Conflict Resolution

Merge conflicts auto-resolved by the system agent in `orchestrator_merge_resolve.go`.

## Chain Runner

Sequential chain item execution via `ChainRunner` (`chain_runner.go`). Newer workflow-chain-run engine in `be/internal/chainrunner/`.

## Per-Project Python Venv

`venvMgr.Ensure(ctx, projectID, projectRoot)` called once in `runLoop`; result passed as `Config.PythonPath` to all spawners. See `be/internal/venv/`. Failures are non-blocking (falls back to PATH `python3`).

## Git Worktree Lifecycle

Worktrees are only used for **ticket-scoped** workflows. Project-scoped workflows always run in the original project root.

When `use_git_worktrees=true` and `default_branch` configured:

- **Setup**: `setupWorktree()` — project scope returns early; ticket scope creates a branch (ticket ID) + worktree under `/tmp/nrflo/worktrees/`.
- **Success**: removes worktree, merges branch into `default_branch` (up to 5 retry attempts), deletes branch. Conflicts trigger `attemptConflictResolution()`; falls through to manual resolution if not configured or fails.
- **Push after merge**: `pushIfEnabled()`; failures logged + broadcast (`workflow.push_failed`), never fail the workflow.
- **Failure/Cancellation**: force-removes worktree and branch without merging.

## Take-Control (Interactive Session)

- `TakeControl(...)` → sends `RequestTakeControl` to the active spawner.
- Spawner kills the agent, sets status `user_interactive`, closes a per-session readiness channel.
- HTTP handler waits on `WaitTakeControlReady` (10s) before returning, preventing PTY race.
- `CompleteInteractive(sessionID)` → updates DB to `interactive_completed` (result=pass), advances workflow.
- Only works for `SupportsResume() == true` agents (Claude CLI). Project-scoped: `TakeControlProject`.
- `runState.spawners`: sessionID→Spawner map via `OnSessionRegister/Unregister` (`orchestrator.go`).
- `KillInteractive(sessionID)` → closes PTY, marks session failed (reason=user_killed), folds as agent failure in layer aggregation.

## Interactive Start & Plan Mode

Mutually exclusive modes (400 if both set): `interactive=true`, `plan_mode=true`. Both create a `user_interactive` session for the L0 agent and register a wait channel + PTY command before `runLoop` starts.

- **Interactive mode**: `runLoop` blocks until PTY completes, then skips L0 and starts from L1.
- **Plan mode**: `runLoop` blocks until PTY completes, reads plan file via `plan_reader.go`, stores content as `user_instructions` finding, then executes all layers from L0.

See `orchestrator_interactive.go`.

## Concurrent Ticket Workflow Guard

- `HasRunningTicketWorkflows(projectID)` checks `o.runs` for active ticket-scoped instances.
- In `Start()`, if `!project.UseGitWorktrees` and a ticket workflow is running, returns error unless `Force=true`.
- HTTP handler maps this to 409 Conflict; frontend shows "Proceed Anyway" option.

## Endless Loop Mode

- `RunRequest.EndlessLoop=true` on project-scoped runs; persisted as `endless_loop=1` on instance row.
- After `markCompleted`, `maybeRestartEndlessLoop` re-reads instance, exits if `StopEndlessLoopAfterIteration=true`, broadcasts `endless_loop_iterating=true`, spawns a fresh `Start()` in a detached goroutine.
- Failure, `Stop()`, and callback errors terminate the loop.

## Next Workflow on Success

- `workflow_definitions.next_workflow_on_success`: after `markCompleted`, calls `maybeStartNextOnSuccess`.
- Guards: `ctx.Err() != nil`, `finalResult == ""`, or `ChainDepth >= 10` → skip.
- Spawns detached `o.Start(context.Background(), nextReq)` with `ScopeType="project"`, `Instructions=finalResult`, `ChainDepth+1`.

## Finalize Slots

After a workflow reaches terminal status, `runFinalize` (`finalize.go`) executes the outcome-selected slot in the **project root** (never a worktree) under a fixed 5s timeout. It **never changes workflow status**.

- Slot source: `RunRequest.Finalize{Success,Failure}{Command,ScriptID}` (from `workflow_definitions`). Both empty for the outcome → no-op (no finding, no event).
- **Command slot**: `sh -c <cmd>` with outcome env (`NRF_WORKFLOW_STATUS`, `NRF_WORKFLOW_RESULT`=`pass`/`fail`, plus `NRF_WORKFLOW_FINAL_RESULT` on success / `NRF_FAILURE_REASON` on failure) on top of `loadProjectEnv`.
- **Python-script slot**: runs the `python_scripts` row via per-project venv python through a transient `_finalize` `agent_session`; uses `runHookScript` from `hookexec.go` with `agentType="_finalize"`.
- Persists a `_finalize` finding via `persistFinalizeFinding` (`finalize_persist.go`); non-`ok` → `errorSvc.RecordError` + `EventWorkflowFinalizeFailed`, success → `EventWorkflowFinalizeSucceeded`.
- Wired into success path (between `markCompleted` and `maybeStartNextOnSuccess`) and the tail of `markFailed` (after writing a `_failure_reason` finding), **skipped** when the failure reason is `reasonCancelled` (user Stop). `forceStopInstance` bypasses `markFailed`, so force-stop never finalizes.

## Pause Slots

`workflow_layer_policies.pause_after=true` causes `runLoop` to pause after that layer completes (including when skipped), setting instance status to `waiting` and removing it from `o.runs`.

- Hook source: `RunRequest.PauseEvent{Command,ScriptID}` (from `workflow_definitions`). Both empty → no hook fires, but pause still occurs.
- Env: `NRF_WORKFLOW_STATUS=waiting`, `NRF_PAUSED_AFTER_LAYER`, `NRF_NEXT_LAYER`, `NRF_WORKFLOW_INSTANCE_ID` plus `loadProjectEnv`.
- **Python-script slot**: transient `_pause` `agent_session` via `runHookScript` in `hookexec.go`.
- Persists a `_pause` finding (`{paused_after_layer,resume_layer,event:{kind,target,exit_code,status,output_tail},timestamp}`) and broadcasts `EventWorkflowPaused`.
- Resume via `ContinueWorkflow` (`continue.go`): validates `status=waiting`, reads `resume_layer` from `_pause` finding, re-launches `runLoop` at the resume-layer index. Optionally appends instructions to `user_instructions` finding.
- Fail via `FailWorkflow` (`fail.go`): running instance → set `rs.failReason` + `cancel()`; waiting instance → `markFailed` directly (fires failure-finalize slot).

## Scheduled Task Origin Tracking

`RunRequest.ScheduledTaskID` → `workflow_instances.scheduled_task_id`; set by `scheduler_dispatch.go`.

## Purge on Completion

When the instance's `purge_on_completion` snapshot is set, `maybePurgeTrace` (`orchestrator_purge.go`) runs `service.PurgeService` at each terminal path's end (after finalize). Invariant: chain steps pass data via instructions + status polling, never prior-instance findings, else purge breaks hand-off.

## Ticket Status Management

- Start: `SetInProgress()` (open → in_progress); Complete: `Close()`; Fail/Cancel: `Reopen()`.
- Each broadcasts `ws.EventTicketUpdated`. Project-scoped workflows skip ticket status changes.

`make test-pkg PKG=orchestrator`.
