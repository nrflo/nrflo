package orchestrator

import (
	"context"
	"time"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
)

// subworkflowWatchInterval is the parent-status poll cadence while a paused
// parent is waiting for resume. A var so tests can shrink it.
var subworkflowWatchInterval = 5 * time.Second

// watchSubworkflow ties a child run to its parent's lifecycle: it releases the
// child's concurrency slot (when counted) once the child ends, and stops the
// child when the parent reaches a TERMINAL status. A paused parent (waiting)
// releases its run slot without ending, so the watcher re-arms on the successor
// runState created by ContinueWorkflow instead of stopping the child.
func (o *Orchestrator) watchSubworkflow(parentID string, parentDone chan struct{}, childID string, counted bool) {
	childDone := o.doneChan(childID)
	for childDone != nil {
		select {
		case <-childDone:
			childDone = nil
		case <-parentDone:
			next, terminal := o.awaitParentOutcome(parentID, childDone)
			switch {
			case terminal:
				logger.Info(context.Background(), "parent run ended; stopping sub-workflow", "instance_id", childID)
				_ = o.Stop(childID)
				<-childDone
				childDone = nil
			case next != nil:
				parentDone = next // parent resumed under a fresh runState
			default:
				childDone = nil // child ended while the parent was paused
			}
		}
	}

	// The child's own runLoop closes its done channel on every return path,
	// including suspending at its own plan boundary (not just terminal
	// completion/failure) — so childDone firing above does not mean the child
	// is actually done. Keep watching the parent while the child remains
	// plan-suspended so a parent that dies before the plan is approved
	// doesn't leave an orphaned child holding a live draft.
	o.watchPlanSuspendedChild(parentID, parentDone, childID)

	if counted {
		o.mu.Lock()
		o.subworkflowActive--
		o.mu.Unlock()
	}
}

// watchPlanSuspendedChild polls childID while it remains plan-suspended:
// stops it (forceStopInstance cancels its draft plan and marks it failed) if
// the parent terminates first, or returns once the child is no longer
// plan-suspended (resumed and re-armed via ResumeAfterPlanApproval, or
// independently reached a terminal status).
func (o *Orchestrator) watchPlanSuspendedChild(parentID string, parentDone chan struct{}, childID string) {
	status, err := o.instanceStatus(childID)
	if err != nil || !model.IsPlanSuspended(status) {
		return
	}
	t := time.NewTicker(subworkflowWatchInterval)
	defer t.Stop()
	for {
		select {
		case <-parentDone:
			pStatus, err := o.instanceStatus(parentID)
			if err != nil || (pStatus != model.WorkflowInstanceWaiting && pStatus != model.WorkflowInstanceActive) {
				logger.Info(context.Background(), "parent run ended; stopping plan-suspended sub-workflow", "instance_id", childID)
				_ = o.Stop(childID)
				return
			}
			if next := o.doneChan(parentID); next != nil {
				parentDone = next // parent resumed under a fresh runState
			}
		case <-t.C:
			status, err := o.instanceStatus(childID)
			if err != nil || !model.IsPlanSuspended(status) {
				return
			}
		}
	}
}

// awaitParentOutcome resolves a fired parentDone: (nextDone, false) once a
// successor runState appears after a pause, (nil, true) when the parent is
// terminal, or (nil, false) when the child itself ended while waiting.
func (o *Orchestrator) awaitParentOutcome(parentID string, childDone chan struct{}) (chan struct{}, bool) {
	t := time.NewTicker(subworkflowWatchInterval)
	defer t.Stop()
	for {
		if next := o.doneChan(parentID); next != nil {
			return next, false
		}
		status, err := o.instanceStatus(parentID)
		if err != nil || (status != model.WorkflowInstanceWaiting && status != model.WorkflowInstanceActive) {
			return nil, true
		}
		select {
		case <-childDone:
			return nil, false
		case <-t.C:
		}
	}
}

// rearmSubworkflowWatcher re-ties a retried/continued sub-run to its parent's
// lifecycle so it cannot outlive its root. Uncounted: human-initiated
// recoveries do not consume the concurrency budget. No-op for top-level runs
// and when the parent is already terminal (the recovery then runs detached,
// owned by the human who triggered it).
func (o *Orchestrator) rearmSubworkflowWatcher(wi *model.WorkflowInstance) {
	if wi.ParentInstanceID == "" {
		return
	}
	parentDone := o.doneChan(wi.ParentInstanceID)
	if parentDone == nil {
		status, err := o.instanceStatus(wi.ParentInstanceID)
		if err != nil || status != model.WorkflowInstanceWaiting {
			return // parent terminal or unknown: recovered child runs detached
		}
		parentDone = make(chan struct{})
		close(parentDone) // paused parent: enter the re-arm wait immediately
	}
	go o.watchSubworkflow(wi.ParentInstanceID, parentDone, wi.ID, false)
}

// doneChan returns the live runState done channel for id, or nil.
func (o *Orchestrator) doneChan(id string) chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	if rs := o.runs[id]; rs != nil {
		return rs.done
	}
	return nil
}

// instanceStatus reads the instance's persisted status via a short-lived pool.
func (o *Orchestrator) instanceStatus(id string) (model.WorkflowInstanceStatus, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return "", err
	}
	defer pool.Close()
	wi, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(id)
	if err != nil {
		return "", err
	}
	return wi.Status, nil
}
