# Orchestrator Package

Server-side workflow orchestration: phases grouped by layer, layers executed sequentially, agents within a layer spawned concurrently. Deep flow mechanics: [REFERENCE.md](REFERENCE.md) — read it before changing callbacks, sub-workflows, worktrees, take-control, finalize/pause slots, or the plan boundary.

## Layer Execution & Aggregation

- Phases grouped by `layer` integer; layers run in ascending order; all agents within a layer run concurrently (one goroutine per `spawner.Spawn()`). Entry point: `runLoop()` (`orchestrator_loop.go`).
- Layer execution, callback plans, retry, and skip partitioning key on `node_id`; model/tag/prompt resolution stays on `agent_type` — see [db/CLAUDE.md](../db/CLAUDE.md).
- Fan-in: `denom = passCount + failCount` (skipped excluded; callback agents count as pass). Layer passes when `passCount >= policy.Required(denom)`; `denom == 0` (all skipped) always passes. Per-layer policies (`workflow_layer_policies`, loaded at start, default `any`): `any`=1, `all`=denom, `quorum:N`=N, `percent:P`=`ceil(denom*P/100)`.

## Startup Loading

`loadModelConfigs()` (cli_models → all spawners, `orchestrator_lifecycle.go`); `claude_safety_hook` project config → `BuildSafetySettingsJSON()`, read once (`orchestrator_start.go`); `venvMgr.Ensure(...)` once per run → `Config.PythonPath` (non-blocking, falls back to PATH `python3`).

## Callback Flow

Agents trigger callbacks via `agent_callback` (`level` = whole-layer, `target_agent`, or `chain`; latter two mutually exclusive — shapes: [doc/common-40-workflow.md](../../../doc/common-40-workflow.md#callback-mechanism)). Settled-layer callbacks merge into one plan, affected sessions reset once, plan steps drain before forward iteration resumes; cap `maxCallbacks=10` spawns per run. Engine: REFERENCE.md § Callback Plan Engine.

## Layer-Skip Logic

Skipping is **per-agent**: before spawning each layer, `applyLayerSkips()` (`orchestrator_skip.go`) partitions phases against `skip_tags` (reloaded per layer); matching agents get `status=skipped` sessions (excluded from `denom`), the rest spawn. All-match → `wholeLayerSkipped=true`, `EventLayerSkipped`, layer counts as passed.

## Consult / Planner

Synchronous one-off children under the caller's instance; `_`-prefixed node ids keep both out of the v4 read model. `Consult(...)` (`consult.go`) enforces the recursion guard and delegates to `Spawner.Consult`. `RunPlanner(...)` (`planner.go`, `service.PlannerRunner`) spawns a fresh `_planner` child — workflow-local `node_role='planner'` def (the `dynamic` workflow ships `dynamic-planner`), else the `planner` system agent — which emits a validated `_workflow_plan`; `renderTemplateLibrary` renders each template's `description` + effective model/effort (never its prompt) and omits install-unusable templates (`service.EnabledTemplates`). See [service/CLAUDE.md](../service/CLAUDE.md).

## Sub-Workflow Runner

`StartSubworkflow`/`StartDynamicWorkflow` start detached child runs under caps (depth/children/invocations, `service/subworkflow.go`); a watcher stops children when the parent is terminal; `GetSubworkflow`/`RevisePlan`/`ApprovePlan` are parent-ownership-guarded. Exposed via `Config.Subworkflows` as `run_subworkflow`/`get_subworkflow`/`dynamic_workflow`/`revise_plan`/`approve_plan`. Mechanics: REFERENCE.md § Sub-Workflow Runner.

## Chain Runner

Sequential chain item execution via `ChainRunner` (`chain_runner.go`); newer engine in `be/internal/chainrunner/`.

## Git Worktree Lifecycle

Worktrees are only used for **ticket-scoped** workflows; project-scoped runs stay in the project root. Success merges into `default_branch` (auto conflict resolution → manual fallback) and optionally pushes; failure/cancel force-removes worktree + branch. Mechanics: REFERENCE.md § Git Worktree Lifecycle.

## Take-Control / Interactive / Plan Mode

User can seize a live Claude CLI session (`TakeControl`, PTY relay), complete or kill it; workflows can also start in interactive or plan mode (mutually exclusive) with a pre-L0 PTY session. Mechanics: REFERENCE.md § Take-Control, § Interactive Start & Plan Mode.

## Finalize & Pause Slots

Terminal runs fire an outcome-selected finalize slot (command or python script, project root, 5s cap, never changes status); `pause_after` layers suspend the run (`waiting`) with a `_pause` finding until `ContinueWorkflow`. Mechanics: REFERENCE.md § Finalize Slots, § Pause Slots.

## Plan Boundary & Materialization

Plan-driven workflows (`service.IsPlanDriven`), after their static layers, self-draft or read the plan head: approved plans materialize into instance nodes and execution continues; otherwise the run suspends in a plan status (`planning`/`waiting_input`/`waiting_approval`) that stays non-terminal to callers. `ResumeAfterPlanApproval` resumes at the first materialized layer. Mechanics: REFERENCE.md § Plan Boundary & Materialization.

## Endless Loop / Next-on-Success / Concurrent Guard

Project-scoped runs can loop until stopped (`endless_loop`); `next_workflow_on_success` chains a follow-up run (`ChainDepth` cap 10); a non-worktree project allows one running ticket workflow at a time (409 + Force override). Mechanics: REFERENCE.md § Endless Loop / Next-on-Success / Concurrent Guard.

## Misc

- `APIWorkflowControl(pool)` (`orchestrator_apirun_control.go`) adapts `ContinueWorkflow`/`FailWorkflow` for callers that pass an `instance_id` as a tool argument (api-mode agents, console tools). Invariant: it rejects an instance whose `project_id` differs from the caller's — both methods resolve the project root and workflow def from the *caller's* `projectID`, so an unguarded foreign id would fail another project's run, or resume its instance in the wrong repo.
- `RunRequest.ScheduledTaskID` → `workflow_instances.scheduled_task_id` (`scheduler_dispatch.go`).
- Purge on completion: `maybePurgeTrace` (`orchestrator_purge.go`) runs `service.PurgeService` at terminal paths (after finalize). Invariant: chain steps pass data via instructions + status polling, never prior-instance findings, else purge breaks hand-off.
- Ticket status: start → `SetInProgress()`, complete → `Close()`, fail/cancel → `Reopen()` (each broadcasts `ws.EventTicketUpdated`); project-scoped runs skip ticket status changes.

`make test-pkg PKG=orchestrator`.
