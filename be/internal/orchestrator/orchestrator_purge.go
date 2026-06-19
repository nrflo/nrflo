package orchestrator

import (
	"context"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/service"
)

// maybePurgeTrace scrubs the instance's sensitive trace data when its purge_on_completion
// snapshot is set; otherwise it is a no-op. Invoked at the end of every terminal transition
// (success in runLoop, markFailed, forceStopInstance) — always AFTER result extraction, the
// completion broadcast, and finalize/next-workflow, so those steps still see the data. Opens its
// own short-lived DB handle (mirroring maybeRestartEndlessLoop). Errors are logged, never
// propagated: the workflow is already terminal and purge must not change its outcome.
func (o *Orchestrator) maybePurgeTrace(wfiID string) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(context.Background(), "purge: failed to open DB", "instance_id", wfiID, "err", err)
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	if _, err := service.NewPurgeService(pool, o.clock, o.wsHub, o.dataPath).
		PurgeInstanceTraceIfEnabled(context.Background(), wfiID); err != nil {
		logger.Error(context.Background(), "purge: failed", "instance_id", wfiID, "err", err)
	}
}
