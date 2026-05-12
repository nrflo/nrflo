# Scheduler Package

Cron-driven scheduled workflow triggers using `github.com/robfig/cron/v3`.

## Lifecycle

- `New(pool, orch, hub, clk, wfChainRunSvc, wfChainRunner)` — constructs; does not start scheduling
- `Start(ctx)` — loads enabled tasks via `repo.ScheduledTaskRepo.ListEnabled()`, registers cron entries, computes and persists `next_run_at`, calls `cron.Start()`
- `Reload()` — mutex-guarded stop + rebuild; called by `service.ScheduledTaskService` after every Create/Update/Delete mutation
- `Stop()` — drains in-flight cron jobs via `cron.Stop()` context, then returns
- `RunNow(taskID)` — loads task, calls `dispatch()` immediately, returns the inserted `ScheduleRun`

## Dispatch Flow (`scheduler_dispatch.go`)

1. Insert `schedule_runs` row with `status=pending` (includes `chain_runs=[]`)
2. Fan out one goroutine per workflow calling `orchestrator.Start(ctx, RunRequest{ScopeType:"project", ScheduledTaskID:task.ID})` — `ScheduledTaskID` is propagated to the `workflow_instances` row so the run is linked to its originating scheduled task
3. Fan out one goroutine per chain ID calling `wfChainRunSvc.CreateRun(projectID, chainID, "", "schedule:<taskID>")` then `wfChainRunner.Start(ctx, runID)`
4. Join all goroutines via WaitGroup
5. Build `[]ScheduleRunWorkflow` and `[]ScheduleRunChain` slices
6. Determine final status: `failed` only if ALL items (workflows + chain runs) failed; otherwise `triggered`
7. Call `repo.ScheduleRunRepo.UpdateStatusFull(id, status, wfJSON, chainJSON, errMsg)`
8. Call `repo.ScheduledTaskRepo.UpdateTriggerTimestamps(now, nextRun)`
9. Broadcast `ws.EventScheduleTriggered` with `chain_runs` payload

The scheduler does **not** block on chain completion — chains execute asynchronously via `chainrunner.Runner`.
Project scope allows multiple concurrent workflow instances — `IsRunning` is not called.

## Integration

- Constructed in `api.NewServer` alongside the orchestrator; receives shared `wfChainRunner` and `wfChainRunSvc` instances
- Started in `Server.Start()` after `wsHub.Run()`
- Stopped in `Server.Stop()` before `orchestrator.StopAll()`
- `service.ScheduleReloader` interface (defined in `service/scheduled_task.go`) breaks the import cycle: `service` → interface only, `scheduler` → concrete type
