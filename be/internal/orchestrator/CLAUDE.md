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

## Error Capture

The orchestrator records errors to the `errors` table via `errorSvc` (implements `spawner.ErrorRecorder`). Passed through the constructor and threaded to all spawners via `Config.ErrorSvc`. Errors are recorded for:
- **Workflow failure** (`markFailed`): type=workflow, instance_id=wfi_id, message=failure reason
- **Merge conflict resolution failure** (`attemptConflictResolution`): type=system, instance_id=wfi_id, message includes branch name and spawn error

## Connection Pool

The orchestrator's `runLoop` creates a shared `*db.Pool` for the entire workflow run and passes it to all spawners via `spawner.Config.Pool`. This avoids per-call `db.Open()`/`Close()` overhead in the spawner.

## Model Config Loading

CLI model configs are loaded from the `cli_models` DB table once at workflow start (in `Start()` and `retryFailed()`) via `loadModelConfigs()`. The result is a `map[string]spawner.ModelConfig` passed to all spawners in that run via `spawner.Config.ModelConfigs`. This allows the spawner to resolve CLI type, mapped model, reasoning effort, and context length from the DB instead of relying on hardcoded adapter methods. Interactive/plan mode sessions and merge conflict resolution also receive model configs. The helper `cliNameFromModelConfigs()` resolves CLI names from DB configs with fallback to `spawner.DefaultCLIForModel()`.

## Safety Hook Threading

The orchestrator reads `claude_safety_hook` from project config at workflow start and passes it through `spawner.BuildSafetySettingsJSON()` to generate `--settings` JSON for Claude CLI agents. The `claudeSettingsJSON` string is threaded through `Start()`, `retryFailed()`, `runLoop()`, `setupInteractivePreStep()`, `buildInteractivePtyArgs()`, and `attemptConflictResolution()`. Read once at start — mid-workflow config changes have no effect (same pattern as `lowConsumptionMode`).

## Callback Flow

A later-layer agent (e.g., qa-verifier) can trigger a callback to re-run an earlier layer:

1. Agent calls `nrflow agent callback` with `--level <N>`
2. Orchestrator saves `_callback` metadata to workflow instance findings
3. Phases/sessions from the target layer forward are reset
4. Execution loop jumps back to the target layer
5. Target agent's prompt includes `${CALLBACK_INSTRUCTIONS}`
6. After target layer succeeds, `_callback` is cleared
7. Max 3 callbacks per workflow run

## Layer-Skip Logic

Before spawning agents for each layer, the orchestrator checks if the layer should be skipped based on workflow instance `skip_tags`. If any agent in the layer has a tag matching a skip tag, the entire layer is skipped:

1. `shouldSkipLayer()` reloads skip_tags from DB each layer (agents may add tags concurrently)
2. Creates `agent_sessions` with `status=skipped`, `result=skipped` for each agent
3. Broadcasts `EventAgentCompleted` (result=skipped) per agent and `EventLayerSkipped` per layer
4. Skipped layers count as passed for fan-in (loop continues to next layer)

Helpers in `orchestrator_skip.go`: `buildAgentTags()`, `shouldSkipLayer()`, `createSkippedSessions()`.

## Automatic Merge Conflict Resolution

When `MergeAndCleanup()` fails after all layers complete, the orchestrator attempts automatic resolution before falling back to manual resolution:

1. `attemptConflictResolution()` loads `conflict-resolver` from `system_agent_definitions`
2. If not found, returns error → falls through to existing manual-resolution behavior
3. Broadcasts `merge.conflict_resolving` WS event
4. Constructs a spawner with a synthetic `_conflict_resolution` workflow containing a single-phase `conflict-resolver` agent
5. Spawns the resolver in `wt.projectRoot` (original project root on default branch) with `ExtraVars`: `BRANCH_NAME`, `DEFAULT_BRANCH`, `MERGE_ERROR`
6. On success: deletes feature branch, broadcasts `merge.conflict_resolved`
7. On failure: broadcasts `merge.conflict_failed`, returns error → manual resolution

The resolver agent's session is tracked under the existing workflow instance (`wfiID`). The synthetic workflow name `_conflict_resolution` uses an underscore prefix to distinguish from user workflows.

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
| `orchestrator_skip.go` | Layer-skip logic: tag matching, skipped session creation |
| `orchestrator_merge_resolve.go` | Automatic merge conflict resolution via system agent |
| `orchestrator_interactive.go` | Interactive start & plan mode pre-step logic |
| `plan_reader.go` | Plan file reader for plan-before-execute mode |
| `chain_runner.go` | Sequential chain execution runner |

## Git Worktree Lifecycle

Worktrees are only used for **ticket-scoped** workflows. Project-scoped workflows always run in the original project root, regardless of `use_git_worktrees` setting.

When a project has `use_git_worktrees=true` and `default_branch` configured, the orchestrator creates an isolated git worktree for each ticket-scoped workflow run:

- **Setup** (`Start`/`retryFailed`): `setupWorktree()` returns early with `nil` worktree for project scope. For ticket scope, creates a branch (named after ticket ID) and worktree at `/tmp/nrflow/worktrees/<branchName>`. Overrides `projectRoot` so all agents work in the worktree.
- **Success** (`runLoop` completion): Removes the worktree first (commits live in `.git/refs/heads/`, not the worktree dir), then merges the branch into `default_branch` with retry (up to 5 attempts with stale `index.lock` removal), and deletes the branch. If merge fails (conflicts), attempts automatic conflict resolution via `attemptConflictResolution()`. Falls through to manual resolution if resolver is not configured or fails.
- **Failure/Cancellation** (`runLoop` defer): Force-removes worktree and branch without merging.

The `worktreeInfo` struct in `runLoop` tracks original project root, worktree path, branch name, and default branch. A `worktreeHandled` flag prevents the cleanup defer from running after a successful merge.

## Take-Control (Interactive Session)

The orchestrator supports taking interactive control of a running agent:

1. `TakeControl(projectID, ticketID, workflow, sessionID)` — finds the active spawner and sends `RequestTakeControl`
2. Spawner kills the agent (SIGTERM → grace → SIGKILL), sets session status to `user_interactive`
3. Spawner blocks `monitorAll` on an `interactiveWaitCh` — the workflow does not advance
4. `CompleteInteractive(sessionID)` — updates DB to `interactive_completed` with `result=pass`, closes the wait channel
5. Spawner unblocks, treats the proc as PASS, and `finalizePhase` proceeds normally

Only works for Claude CLI agents (`SupportsResume() == true`). Project-scoped equivalents: `TakeControlProject`, same `CompleteInteractive`.

## Interactive Start & Plan Mode

The orchestrator supports two pre-launch modes that create a `user_interactive` agent session before the normal layer execution begins. Both are triggered via the workflow run endpoint and are mutually exclusive (passing both `interactive=true` and `plan_mode=true` returns 400).

**Interactive mode** (`interactive=true`):

1. `Start()` creates an agent session with `status=user_interactive` for the L0 agent and registers a wait channel before launching `runLoop` (avoids race with PTY registration)
2. A PTY command is registered via the `OnRegisterPtyCommand` callback so the UI can open a terminal
3. `runLoop` blocks until the PTY session completes (wait channel closed)
4. After unblocking, `runLoop` skips L0 and begins execution from L1

**Plan mode** (`plan_mode=true`):

1. Same pre-launch setup: `user_interactive` session, wait channel, PTY command registration
2. `runLoop` blocks until the PTY session completes
3. After unblocking, the orchestrator reads the plan file from `~/.claude/plans/` via `plan_reader.go`
4. Plan file content is stored as `user_instructions` in the workflow instance findings
5. `runLoop` then executes all layers starting from L0 (no layer skip)

The wait channel is registered in `Start()` before the `runLoop` goroutine launches to ensure the PTY handler can find it immediately, preventing a race between PTY connection and orchestrator startup.

## Concurrent Ticket Workflow Guard

When `use_git_worktrees=false` for a project, multiple ticket-scoped workflows operating in the same project root can cause file conflicts and git state corruption. The orchestrator guards against this:

- `HasRunningTicketWorkflows(projectID)` checks the in-memory `o.runs` map cross-referenced with DB to find active ticket-scoped instances
- In `Start()`, after loading the project, if `!project.UseGitWorktrees` and a ticket-scoped workflow is already running, returns error unless `RunRequest.Force=true`
- The guard does NOT apply to project-scoped workflows or when worktrees are enabled
- The HTTP handler maps this error to 409 Conflict; the frontend detects it and shows a warning with "Proceed Anyway" option

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
- `orchestrator_worktree_test.go` — worktree setup/merge/cleanup lifecycle, project scope skips worktree
- `orchestrator_take_control_test.go` — TakeControl/TakeControlProject/CompleteInteractive methods
- `orchestrator_skip_tag_test.go` — buildAgentTags, shouldSkipLayer, createSkippedSessions, WS events
- `orchestrator_concurrent_ticket_test.go` — HasRunningTicketWorkflows, concurrent guard (block/force/worktrees/project-scope)

Integration tests in `internal/integration/`:
- `chain_epic_test.go` — chain epic auto-close
- `run_epic_workflow_test.go` — run-epic endpoint tests
