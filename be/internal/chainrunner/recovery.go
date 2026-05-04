package chainrunner

import (
	"context"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/ws"
)

// RecoverZombieRuns marks any runs that are stuck in 'running' status as failed.
// Called on server startup to handle crash recovery.
func (r *Runner) RecoverZombieRuns() {
	ctx := context.Background()
	pool, err := db.NewPool(r.dataPath, db.DefaultPoolConfig())
	if err != nil {
		logger.Error(ctx, "chainrunner: failed to open DB for recovery", "err", err)
		return
	}
	defer pool.Close()

	rr := repo.NewWorkflowChainRunRepo(pool, r.clock)
	runs, err := rr.GetActiveRuns()
	if err != nil {
		logger.Error(ctx, "chainrunner: failed to query zombie runs", "err", err)
		return
	}

	for _, run := range runs {
		logger.Warn(ctx, "chainrunner: recovering zombie run", "run_id", run.ID)
		steps, _ := rr.ListRunSteps(run.ID)
		for _, s := range steps {
			if s.Status == "running" || s.Status == "pending" {
				rr.UpdateRunStepStatus(s.ID, "canceled") //nolint:errcheck
			}
		}
		rr.UpdateRunStatus(run.ID, "failed") //nolint:errcheck
		if r.wsHub != nil {
			r.wsHub.Broadcast(ws.NewEvent(ws.EventChainRunFailed, run.ProjectID, "", "", map[string]interface{}{
				"run_id": run.ID,
				"reason": "server_restart",
			}))
		}
	}
}
