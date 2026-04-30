# Scheduler Package

Cron-driven scheduled workflow triggers using `github.com/robfig/cron/v3`.

## Lifecycle

- `New(pool, orch, hub, clk)` — constructs; does not start scheduling
- `Start(ctx)` — loads enabled tasks via `repo.ScheduledTaskRepo.ListEnabled()`, registers cron entries, computes and persists `next_run_at`, calls `cron.Start()`
- `Reload()` — mutex-guarded stop + rebuild; called by `service.ScheduledTaskService` after every Create/Update/Delete mutation
- `Stop()` — drains in-flight jobs via `cron.Stop()` context, then returns
- `RunNow(taskID)` — loads task, calls `dispatch()` immediately, returns the inserted `ScheduleRun`

## Dispatch Flow (`scheduler_dispatch.go`)

1. Insert `schedule_runs` row with `status=pending`
2. Fan out one goroutine per workflow calling `orchestrator.Start(ctx, RunRequest{ScopeType:"project"})`
3. Join all goroutines via channel
4. Marshal `[]ScheduleRunWorkflow` JSON
5. Call `repo.ScheduleRunRepo.UpdateStatus(triggered|failed, json, errMsg)`
6. Call `repo.ScheduledTaskRepo.UpdateTriggerTimestamps(now, nextRun)`
7. Broadcast `ws.EventScheduleTriggered`

Project scope allows multiple concurrent workflow instances — `IsRunning` is not called.

## Integration

- Constructed in `api.NewServer` alongside the orchestrator
- Started in `Server.Start()` after `wsHub.Run()`
- Stopped in `Server.Stop()` before `orchestrator.StopAll()`
- `service.ScheduleReloader` interface (defined in `service/scheduled_task.go`) breaks the import cycle: `service` → interface only, `scheduler` → concrete type
