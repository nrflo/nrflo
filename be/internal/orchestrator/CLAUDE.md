# Orchestrator Package

Server-side workflow orchestration. Groups phases by layer and executes layers sequentially, with concurrent agent spawning within each layer.

## Layer-Based Parallel Execution

- Phases grouped by `layer` integer; layers execute in ascending order, sequentially; all agents within a layer run concurrently (one goroutine per `spawner.Spawn()` call).
- Layer completes when all agents finish; pass policy evaluated via `denom = passCount + failCount` (skipped excluded). All-skipped (`denom == 0`) → layer passes regardless of policy. Entry point: `orchestrator_loop.go` `runLoop()`.
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

## Model Config Loading

Loaded from `cli_models` at workflow start via `loadModelConfigs()`; passed to all spawners as `ModelConfigs` (`orchestrator_lifecycle.go`).

## Safety Hook Threading

`claude_safety_hook` project config → `BuildSafetySettingsJSON()` → threaded through all spawn paths; read once at start (`orchestrator_start.go`).

## Callback Flow

Agents trigger callbacks via the `agent_callback` tool (`level` = whole-layer, `target_agent`, or `chain`; the latter two are mutually exclusive). Shapes and restrictions: [doc/common-40-workflow.md](../../../doc/common-40-workflow.md#callback-mechanism).

All callbacks from a settled layer are collected and processed through the plan engine (`orchestrator_callback_plan.go`): decompose each `CallbackError` into a `decomposedRequest`, merge via `mergeCallbackPlans` (whole-layer wins over per-agent, max resumeLayer), reset affected sessions once (`ResetAgentSessionsInWorkflow`), then `runLoop` drains `plan.steps[callbackPlanIdx]` via `spawnPhases` before forward iteration (non-whole-layer steps fail the workflow if they return a callback), resuming at `plan.resumeLayer` once drained.

Cap: `maxCallbacks = 10` cumulative agent spawns per run (whole-layer counts len(phases), per-agent counts len(nodes)); exceeding it fails the workflow. Subset/chain plan steps cannot themselves emit callbacks (v1).

## Layer-Skip Logic

Skipping is **per-agent**, not all-or-nothing per layer. Before spawning each layer, `applyLayerSkips()` (`orchestrator_skip.go`) partitions its phases against `skip_tags` (reloaded from DB each layer): each matching agent gets a `status=skipped` session + `EventAgentCompleted` broadcast, excluded from `results` (so `denom` already excludes it); the runnable subset still spawns. Only when **every** agent matches does it return `wholeLayerSkipped=true` — `EventLayerSkipped` broadcasts and the loop advances (counts as passed).

## Consult / Planner

Synchronous one-off children under the caller's instance; `_`-prefixed node ids keep both out of the v4 read model.

- `Consult(...)` (`consult.go`): resolves the caller session, enforces the recursion guard (consultants cannot consult), builds an api-capable `spawner.Config`, delegates to `Spawner.Consult`.
- `RunPlanner(...)` (`planner.go`, `service.PlannerRunner`): a fresh `_planner` child (workflow-local `node_role='planner'` def, else the `planner` system agent) emits a validated `_workflow_plan`. See [service/CLAUDE.md](../service/CLAUDE.md).
- The `dynamic` workflow ships its own `node_role='planner'` def (`dynamic-planner`), so it always resolves ahead of the system planner; `renderTemplateLibrary` renders each template's `description` + effective model/`reasoning_effort`, not its prompt, and omits templates unusable on this install (`service.EnabledTemplates`).

## Sub-Workflow Runner

`subworkflow_runner.go` + `subworkflow_child_start.go`: `StartSubworkflow`/`StartDynamicWorkflow` (`dynamic_workflow.go`) share `startChildRun` — callable_as_subworkflow/purge/pause checks, depth/children caps (`service/subworkflow.go`), the persisted invocation budget (`subworkflow_starts`, atomic across pause/continue/retry), a concurrency slot, detached `o.Start`, and the watcher. Sub-runs never fire next-on-success. The watcher (`subworkflow_watch.go`) stops children (`Stop`, cancelling any live draft plan) only when the parent is TERMINAL — a plan-suspended child is polled via `watchPlanSuspendedChild` instead (pause/plan-approve re-arm via `rearmSubworkflowWatcher`). `GetSubworkflow` returns an `apirun.SubworkflowState` (running/waiting/plan-status — with `Plan`/`Revision`/`Questions`/completed/failed); `assertChildOwnership` (shared with `RevisePlan`/`ApprovePlan`) enforces that only the matching `parent_instance_id` caller may read/drive a child. Exposed via `Config.Subworkflows` as `run_subworkflow`/`get_subworkflow`/`dynamic_workflow`/`revise_plan`/`approve_plan`.

## Chain Runner

Sequential chain item execution via `ChainRunner` (`chain_runner.go`); newer engine in `be/internal/chainrunner/`.

## Per-Project Python Venv

`venvMgr.Ensure(ctx, projectID, projectRoot)`, called once in `runLoop`, passes `Config.PythonPath` to all spawners (`be/internal/venv/`); non-blocking failure falls back to PATH `python3`.

## Git Worktree Lifecycle

Worktrees are only used for **ticket-scoped** workflows. Project-scoped workflows always run in the original project root.

When `use_git_worktrees=true` and `default_branch` configured: **setup** (`setupWorktree()`) — project scope returns early, ticket scope creates a branch + worktree under `/tmp/nrflo/worktrees/`; **success** — removes worktree, merges into `default_branch` (up to 5 retries; conflicts trigger `attemptConflictResolution()`, `orchestrator_merge_resolve.go`, else manual resolution), deletes branch; **push after merge** via `pushIfEnabled()` (logged + broadcast, never fails the workflow); **failure/cancellation** force-removes worktree and branch without merging.

## Take-Control (Interactive Session)

`TakeControl(...)` sends `RequestTakeControl` to the active spawner, which kills the agent, sets status `user_interactive`, and closes a per-session readiness channel; the HTTP handler waits on `WaitTakeControlReady` (10s) before returning, preventing a PTY race. `CompleteInteractive(sessionID)` updates DB to `interactive_completed` (result=pass) and advances the workflow. Only works for `SupportsResume() == true` agents (Claude CLI); project-scoped via `TakeControlProject`. `runState.spawners` is a sessionID→Spawner map (`OnSessionRegister/Unregister`, `orchestrator.go`). `KillInteractive(sessionID)` closes the PTY and marks the session failed (user_killed), folding as an agent failure in layer aggregation.

## Interactive Start & Plan Mode

Mutually exclusive modes (400 if both set): `interactive=true`, `plan_mode=true` (unrelated to the plan lifecycle/materialization boundary below, despite the shared name). Both create a `user_interactive` session for the L0 agent and register a wait channel + PTY command before `runLoop` starts. Interactive mode: `runLoop` blocks until PTY completes, then skips L0, starts from L1. Plan mode: blocks until PTY completes, reads the plan file (`plan_reader.go`), stores it as a `user_instructions` finding, executes all layers from L0.

## Concurrent Ticket Workflow Guard

`HasRunningTicketWorkflows(projectID)` checks `o.runs` for active ticket-scoped instances; `Start()` errors (unless `Force=true`) when `!project.UseGitWorktrees` and one is already running for the ticket. HTTP handler maps this to 409 Conflict; frontend shows "Proceed Anyway".

## Endless Loop Mode

`RunRequest.EndlessLoop=true` on project-scoped runs persists as `endless_loop=1`. After `markCompleted`, `maybeRestartEndlessLoop` re-reads the instance, exits if `StopEndlessLoopAfterIteration=true`, else broadcasts `endless_loop_iterating=true` and spawns a fresh `Start()` detached. Failure, `Stop()`, and callback errors terminate the loop.

## Next Workflow on Success

`workflow_definitions.next_workflow_on_success`: after `markCompleted`, `maybeStartNextOnSuccess` (guards: `ctx.Err() != nil`, `finalResult == ""`, `ChainDepth >= 10` → skip) spawns detached `o.Start(context.Background(), nextReq)` with `ScopeType="project"`, `Instructions=finalResult`, `ChainDepth+1`.

## Finalize Slots

After a workflow reaches terminal status, `runFinalize` (`finalize.go`) executes the outcome-selected slot in the **project root** (never a worktree) under a fixed 5s timeout; it **never changes workflow status**. Slot source: `RunRequest.Finalize{Success,Failure}{Command,ScriptID}` — both empty is a no-op. Command slot: `sh -c <cmd>` with outcome env (`NRF_WORKFLOW_STATUS`, `NRF_WORKFLOW_RESULT`, `NRF_WORKFLOW_FINAL_RESULT`/`NRF_FAILURE_REASON`) on `loadProjectEnv`. Python-script slot: per-project venv via a transient `_finalize` session. Persists a `_finalize` finding; broadcasts `EventWorkflowFinalizeFailed`/`Succeeded`. Wired into success + tail of `markFailed` (skipped for `reasonCancelled`); `forceStopInstance` bypasses `markFailed` so force-stop never finalizes.

## Pause Slots

`workflow_layer_policies.pause_after=true` causes `runLoop` to pause after that layer completes (including when skipped), setting instance status to `waiting` and removing it from `o.runs`. Hook slot / env / python-script mechanics mirror Finalize Slots above (`RunRequest.PauseEvent{Command,ScriptID}`; env adds `NRF_PAUSED_AFTER_LAYER`/`NRF_NEXT_LAYER`).

- Persists a `_pause` finding (`{paused_after_layer,resume_layer,event,timestamp}`) + broadcasts `EventWorkflowPaused`.
- Resume via `ContinueWorkflow` (`continue.go`): validates `status=waiting`, reads `resume_layer` from `_pause` finding, re-launches `runLoop` at the resume-layer index. Optionally appends instructions to `user_instructions` finding.
- Fail via `FailWorkflow` (`fail.go`): running instance → set `rs.failReason` + `cancel()`; waiting instance → `markFailed` directly (fires failure-finalize slot).

## Plan Boundary & Materialization

Once a plan-driven workflow (`service.IsPlanDriven`) exhausts its static layers, `reloadPlanLayers` (`plan_boundary.go`) checks the plan head: approved → `Materialize` (idempotent) splices nodes via `service.EffectivePhases`, loop continues; no head (or an empty draft) → `draftPlanAndProceed` self-drafts inline (blocking `PlanService.Revise{Revision:0}` — the run has no live layer here), then either auto-approves+materializes (`RunRequest.PlanAutoApprove && service.DynamicAutoEnabled`, no suspend) or suspends via `DerivePlanInstanceStatus` (`planning`/`waiting_input`/`waiting_approval`); an existing draft suspends the same way; cancelled → `markFailed`. `continue.go`/retry build `layerGroups` the same way (read, never re-create, materialized nodes). `ResumeAfterPlanApproval` (`plan_resume.go`, the `ContinueWorkflow` twin) relaunches `runLoop` at the first materialized layer. Not pause_after — no `_pause` finding; the plan statuses keep plan-driven defs callable (`GetSubworkflow` treats them as non-terminal).

## Scheduled Task Origin Tracking

`RunRequest.ScheduledTaskID` → `workflow_instances.scheduled_task_id`; set by `scheduler_dispatch.go`.

## Purge on Completion

When the instance's `purge_on_completion` snapshot is set, `maybePurgeTrace` (`orchestrator_purge.go`) runs `service.PurgeService` at each terminal path's end (after finalize). Invariant: chain steps pass data via instructions + status polling, never prior-instance findings, else purge breaks hand-off.

## Ticket Status Management

Start: `SetInProgress()` (open → in_progress); Complete: `Close()`; Fail/Cancel: `Reopen()` — each broadcasts `ws.EventTicketUpdated`. Project-scoped workflows skip ticket status changes.

`make test-pkg PKG=orchestrator`.
