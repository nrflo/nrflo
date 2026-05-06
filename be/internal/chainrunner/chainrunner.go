// Package chainrunner executes workflow chain runs sequentially.
// Each step in a chain runs one workflow (project-scope or ticket-scope) via the orchestrator.
// Steps execute one at a time; the runner waits for each workflow instance to finish before
// starting the next.
package chainrunner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/ws"
)

const pollInterval = 5 * time.Second

// Runner executes workflow chain runs.
type Runner struct {
	mu       sync.Mutex
	runs     map[string]context.CancelFunc // runID → cancel
	orch     *orchestrator.Orchestrator
	dataPath string
	wsHub    *ws.Hub
	clock    clock.Clock
}

// New creates a new Runner.
func New(orch *orchestrator.Orchestrator, dataPath string, wsHub *ws.Hub, clk clock.Clock) *Runner {
	return &Runner{
		runs:     make(map[string]context.CancelFunc),
		orch:     orch,
		dataPath: dataPath,
		wsHub:    wsHub,
		clock:    clk,
	}
}

// Start begins execution of a chain run. The run must already be in 'pending' status.
func (r *Runner) Start(ctx context.Context, runID string) error {
	pool, err := db.NewPool(r.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer pool.Close()

	rr := repo.NewWorkflowChainRunRepo(pool, r.clock)
	run, err := rr.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Status != "pending" {
		return fmt.Errorf("chain run must be pending to start (current: %s)", run.Status)
	}

	r.mu.Lock()
	if _, running := r.runs[runID]; running {
		r.mu.Unlock()
		return fmt.Errorf("chain run %s is already executing", runID)
	}

	if err := rr.UpdateRunStatus(runID, "running"); err != nil {
		r.mu.Unlock()
		return err
	}

	trx := logger.NewTrx()
	runCtx, cancel := context.WithCancel(logger.WithTrx(context.Background(), trx))
	r.runs[runID] = cancel
	r.mu.Unlock()

	r.broadcast(ws.EventChainRunStarted, run.ProjectID, runID, nil)
	go r.runLoop(runCtx, run)
	return nil
}

// Cancel stops a running chain run.
func (r *Runner) Cancel(runID string) error {
	r.mu.Lock()
	cancel, ok := r.runs[runID]
	r.mu.Unlock()

	if !ok {
		pool, err := db.NewPool(r.dataPath, db.DefaultPoolConfig())
		if err != nil {
			return err
		}
		defer pool.Close()
		rr := repo.NewWorkflowChainRunRepo(pool, r.clock)
		run, err := rr.GetRun(runID)
		if err != nil {
			return err
		}
		if run.Status == "pending" {
			if err := rr.UpdateRunStatus(runID, "canceled"); err != nil {
				return err
			}
			r.broadcast(ws.EventChainRunFailed, run.ProjectID, runID, map[string]interface{}{"reason": "canceled"})
			return nil
		}
		return fmt.Errorf("chain run %s is not running", runID)
	}
	cancel()
	return nil
}

// IsRunning returns true if the run is actively executing.
func (r *Runner) IsRunning(runID string) bool {
	r.mu.Lock()
	_, ok := r.runs[runID]
	r.mu.Unlock()
	return ok
}

// WaitAll blocks until all running goroutines exit or timeout elapses.
func (r *Runner) WaitAll(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		n := len(r.runs)
		r.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// FailAllRunning marks any runs that are stuck in 'running' status as failed.
// Called during server shutdown to ensure clean state.
func (r *Runner) FailAllRunning() {
	ctx := context.Background()
	pool, err := db.NewPool(r.dataPath, db.DefaultPoolConfig())
	if err != nil {
		logger.Error(ctx, "chainrunner: failed to open DB for shutdown sweep", "err", err)
		return
	}
	defer pool.Close()

	rr := repo.NewWorkflowChainRunRepo(pool, r.clock)
	runs, err := rr.GetActiveRuns()
	if err != nil {
		logger.Error(ctx, "chainrunner: failed to query active runs", "err", err)
		return
	}

	for _, run := range runs {
		logger.Warn(ctx, "chainrunner: marking run failed on shutdown", "run_id", run.ID)
		steps, _ := rr.ListRunSteps(run.ID)
		for _, s := range steps {
			if s.Status == "running" || s.Status == "pending" {
				rr.UpdateRunStepStatus(s.ID, "canceled") //nolint:errcheck
			}
		}
		rr.UpdateRunStatus(run.ID, "failed") //nolint:errcheck
		r.broadcast(ws.EventChainRunFailed, run.ProjectID, run.ID, map[string]interface{}{"reason": "server_shutdown"})
	}
}

func (r *Runner) broadcast(eventType, projectID, runID string, extra map[string]interface{}) {
	if r.wsHub == nil {
		return
	}
	data := map[string]interface{}{"run_id": runID}
	for k, v := range extra {
		data[k] = v
	}
	r.wsHub.Broadcast(ws.NewEvent(eventType, projectID, "", "", data))
}

func (r *Runner) pollInstance(ctx context.Context, instanceID string) (finished, success bool) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, false
		case <-ticker.C:
			if !r.orch.IsInstanceRunning(instanceID) {
				pool, err := db.NewPool(r.dataPath, db.DefaultPoolConfig())
				if err != nil {
					return true, false
				}
				wfiRepo := repo.NewWorkflowInstanceRepo(pool, r.clock)
				wi, err := wfiRepo.Get(instanceID)
				pool.Close()
				if err != nil {
					return true, false
				}
				st := wi.Status
				return true, st == model.WorkflowInstanceCompleted || st == model.WorkflowInstanceProjectCompleted
			}
		}
	}
}
