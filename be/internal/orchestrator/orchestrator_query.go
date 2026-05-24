package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
)

// IsRunning checks if any orchestration is running for a ticket+workflow.
func (o *Orchestrator) IsRunning(projectID, ticketID, workflowName string) bool {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	instances, err := wfiRepo.ListByTicket(projectID, ticketID)
	if err != nil {
		return false
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	for _, wi := range instances {
		if strings.EqualFold(wi.WorkflowID, workflowName) {
			if _, running := o.runs[wi.ID]; running {
				return true
			}
		}
	}
	return false
}

// HasRunningTicketWorkflows checks if any ticket-scoped workflow is currently
// running for the given project. Uses in-memory o.runs for accuracy.
func (o *Orchestrator) HasRunningTicketWorkflows(projectID string) bool {
	// Collect running instance IDs under lock
	o.mu.Lock()
	ids := make([]string, 0, len(o.runs))
	for id := range o.runs {
		ids = append(ids, id)
	}
	o.mu.Unlock()

	if len(ids) == 0 {
		return false
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	for _, id := range ids {
		wi, err := wfiRepo.Get(id)
		if err != nil {
			continue
		}
		if wi.TicketID != "" && strings.EqualFold(wi.ProjectID, projectID) {
			return true
		}
	}
	return false
}

// IsInstanceRunning checks if a specific instance ID has an active orchestration.
func (o *Orchestrator) IsInstanceRunning(instanceID string) bool {
	o.mu.Lock()
	_, running := o.runs[instanceID]
	o.mu.Unlock()
	return running
}

// runSettleTimeout bounds how long retry/continue wait for an in-flight runLoop
// goroutine to release its o.runs slot before concluding the run is genuinely active.
// It must exceed hookTimeout, since markFailed runs the failure finalize slot before the
// goroutine exits and releases the slot.
const runSettleTimeout = hookTimeout + 5*time.Second

// waitForRunToSettle blocks until any in-flight runLoop goroutine for wfiID has finished
// tearing down and released its o.runs slot, returning nil once the slot is free.
//
// markFailed / markCompleted / maybePauseAfterLayer publish the terminal-or-waiting
// status (DB row + WS broadcast) from inside the runLoop goroutine, but the goroutine
// only deletes its o.runs entry in a deferred cleanup that runs afterwards — delayed by
// an up-to-hookTimeout finalize hook on the failure path. A caller that reacts to the
// published status (retry-failed on "failed", continue on "waiting") can therefore still
// observe the slot as occupied. Such callers have already confirmed the instance is no
// longer active, so they wait the teardown out here instead of racing it.
//
// The run's done channel is closed strictly after its o.runs entry is deleted (see the
// runLoop deferred cleanup), so once done fires the slot is guaranteed free. A registered
// run with no done channel cannot be waited on and is treated as genuinely active.
func (o *Orchestrator) waitForRunToSettle(ctx context.Context, wfiID string) error {
	o.mu.Lock()
	rs, ok := o.runs[wfiID]
	o.mu.Unlock()
	if !ok {
		return nil
	}
	if rs.done == nil {
		return fmt.Errorf("workflow is already running")
	}
	select {
	case <-rs.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(runSettleTimeout):
		return fmt.Errorf("workflow is already running")
	}
}

// StopAll cancels all running orchestrations and waits for them to exit (for server shutdown).
func (o *Orchestrator) StopAll() {
	o.mu.Lock()
	logger.Warn(context.Background(), "stopping all orchestrations", "count", len(o.runs))
	doneChans := make([]chan struct{}, 0, len(o.runs))
	for _, rs := range o.runs {
		rs.cancel()
		if rs.done != nil {
			doneChans = append(doneChans, rs.done)
		}
	}
	o.mu.Unlock()

	deadline := time.After(10 * time.Second)
	for _, done := range doneChans {
		select {
		case <-done:
		case <-deadline:
			return
		}
	}
}
