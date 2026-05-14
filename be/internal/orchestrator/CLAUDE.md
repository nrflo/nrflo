# Orchestrator Package

Server-side workflow orchestration. Groups phases by layer and executes layers sequentially, with concurrent agent spawning within each layer.

## Layer-Based Parallel Execution

- Phases grouped by `layer` integer; layers execute in ascending order, sequentially.
- All agents within a layer run concurrently (one goroutine per `spawner.Spawn()` call).
- Layer completes when all agents finish; pass policy evaluated via `denom = passCount + failCount` (skipped excluded).
- All-skipped (`denom == 0`) → layer passes regardless of policy; entry point: `orchestrator.go` `runLoop()`.

## Layer Aggregation

- **Denominator rule**: `denom = passCount + failCount` (skipped agents excluded)
- **Callback agents** count as pass (added to `passCount` before policy check)
- **Policy check** (`denom > 0`): `passCount >= policy.Required(denom)` or workflow fails
- Parallel-to-parallel topologies fully supported

### Fan-In Pass Policies (per-layer, stored in `workflow_layer_policies`)

| Policy | Required passes |
|--------|----------------|
| `any` (default) | 1 |
| `all` | all agents (denom) |
| `quorum:N` | exactly N |
| `percent:P` | `ceil(denom * P / 100)` |

Policies loaded from DB at workflow start via `WorkflowLayerPolicyService.GetLayerPolicies`; missing entries default to `"any"`.

## Error Capture

Records to `errors` table via `errorSvc` (`spawner.ErrorRecorder`). Recorded for workflow failure (`markFailed`) and merge conflict resolution failure (`attemptConflictResolution`).

## Connection Pool

One shared `*db.Pool` per workflow run, passed to all spawners via `spawner.Config.Pool` (`orchestrator.go`).

## Model Config Loading

Loaded from `cli_models` at workflow start via `loadModelConfigs()`; passed to all spawners as `ModelConfigs` (`orchestrator.go`).

## Safety Hook Threading

`claude_safety_hook` project config → `BuildSafetySettingsJSON()` → `claudeSettingsJSON`, threaded through all spawn paths. Read once at start; mid-workflow changes have no effect (`orchestrator.go`).

## Callback Flow

A later-layer agent can trigger a callback to re-run an earlier layer:

1. Agent calls `nrflo agent callback` with `--level <N>`
2. Orchestrator saves `_callback` metadata to workflow instance findings
3. Phases/sessions from the target layer forward are reset
4. Execution loop jumps back to the target layer
5. Target agent's prompt includes `${CALLBACK_INSTRUCTIONS}`
6. After target layer succeeds, `_callback` is cleared
7. Max 3 callbacks per workflow run

## Layer-Skip Logic

Before spawning agents for each layer, the orchestrator checks `skip_tags`:

1. `shouldSkipLayer()` reloads skip_tags from DB each layer (agents may add tags concurrently)
2. Creates `agent_sessions` with `status=skipped`, `result=skipped` for each agent
3. Broadcasts `EventAgentCompleted` (result=skipped) per agent and `EventLayerSkipped` per layer
4. Skipped layers count as passed for layer aggregation

Helpers in `orchestrator_skip.go`: `buildAgentTags()`, `shouldSkipLayer()`, `createSkippedSessions()`.

## Automatic Merge Conflict Resolution

Merge conflicts auto-resolved by the system agent defined in `be/internal/orchestrator/orchestrator_merge_resolve.go`.

## Chain Runner

Sequential chain item execution via `ChainRunner` (`chain_runner.go`). Newer workflow-chain-run engine in `be/internal/chainrunner/`.

## Per-Project Python Venv

`venvMgr.Ensure(ctx, projectID, projectRoot)` called once in `runLoop`; result passed as `Config.PythonPath` to all spawners. See `be/internal/venv/`. Failures are non-blocking (falls back to PATH `python3`).

## Git Worktree Lifecycle

Worktrees are only used for **ticket-scoped** workflows. Project-scoped workflows always run in the original project root.

When `use_git_worktrees=true` and `default_branch` configured:

- **Setup** (`Start`/`retryFailed`): `setupWorktree()` returns early for project scope; for ticket scope creates a branch (named after ticket ID) and worktree at `/tmp/nrflo/worktrees/<branchName>`.
- **Success**: removes worktree, merges branch into `default_branch` (up to 5 retry attempts), deletes branch. Conflicts trigger `attemptConflictResolution()`; falls through to manual resolution if not configured or fails.
- **Push after merge**: `pushIfEnabled()` pushes default branch to origin when `push_after_merge` is enabled. Failure logged and broadcast (`workflow.push_failed`) but does not fail the workflow.
- **Failure/Cancellation**: force-removes worktree and branch without merging.

## Take-Control (Interactive Session)

- `TakeControl(projectID, ticketID, workflow, sessionID)` → sends `RequestTakeControl` to the active spawner.
- Spawner kills the agent, sets status `user_interactive`, closes a per-session readiness channel.
- HTTP handler waits on `WaitTakeControlReady` (10s) before returning, preventing PTY race.
- `CompleteInteractive(sessionID)` → updates DB to `interactive_completed` (result=pass), advances workflow.
- Only works for `SupportsResume() == true` agents (Claude CLI). Project-scoped: `TakeControlProject`.
- `runState.spawners` is a `map[string]*spawner.Spawner` keyed by session ID; maintained via `OnSessionRegister`/`OnSessionUnregister` callbacks (`orchestrator.go`).
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
- `/api/v1/projects/{id}/workflow/stop-endless-loop` toggles `stop_endless_loop_after_iteration`.
- Failure, `Stop()`, and callback errors terminate the loop.

## Next Workflow on Success

- `workflow_definitions.next_workflow_on_success`: after `markCompleted`, calls `maybeStartNextOnSuccess`.
- Guards: `ctx.Err() != nil`, `finalResult == ""`, or `ChainDepth >= 10` → skip.
- Spawns detached `o.Start(context.Background(), nextReq)` with `ScopeType="project"`, `Instructions=finalResult`, `ChainDepth+1`.

## Scheduled Task Origin Tracking

`RunRequest.ScheduledTaskID` forwarded through `Init`/`InitProjectWorkflow` → `workflow_instances.scheduled_task_id` (nullable FK to `scheduled_tasks`). Set by `scheduler_dispatch.go`; empty for all other entrypoints.

## Ticket Status Management

- Start: `SetInProgress()` (open → in_progress); Complete: `Close()`; Fail/Cancel: `Reopen()`.
- Each broadcasts `ws.EventTicketUpdated`. Project-scoped workflows skip ticket status changes.

Run `make test-pkg PKG=orchestrator`.
