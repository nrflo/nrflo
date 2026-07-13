package orchestrator

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// TestWatchPlanSuspendedChild_ParentTerminates_StopsAndCancelsChild is the
// acceptance-critical parent-death regression case: a plan-suspended child
// (no runState, per reality — plan-suspended instances are never added to
// o.runs) with a live draft plan must be stopped (draft cancelled, instance
// marked failed) once its parent reaches a terminal status. Before the
// DYNWF-5 forceStopInstance change, Stop() silently no-op'd on such a child.
func TestWatchPlanSuspendedChild_ParentTerminates_StopsAndCancelsChild(t *testing.T) {
	oldInterval := subworkflowWatchInterval
	subworkflowWatchInterval = 2 * time.Millisecond
	defer func() { subworkflowWatchInterval = oldInterval }()

	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")
	childID := env.initProjectWorkflow(t, "test")

	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(childID, model.WorkflowInstanceWaitingApproval); err != nil {
		t.Fatalf("UpdateStatus(child): %v", err)
	}
	appendDraftPlan(t, env, childID, validManifest("goal", "n/a"))

	// Child deliberately has no runState (mirrors reality). Parent gets a live
	// fake runState, mirroring subworkflow_watch_test.go's fakeCancel pattern.
	parentDone := make(chan struct{})
	parentStopped := make(chan struct{})
	env.orch.mu.Lock()
	env.orch.runs[parentID] = &runState{done: parentDone, cancel: fakeCancel(parentStopped, parentDone)}
	env.orch.mu.Unlock()

	watcherExit := make(chan struct{})
	go func() {
		env.orch.watchPlanSuspendedChild(parentID, parentDone, childID)
		close(watcherExit)
	}()

	// Terminate the parent and release its run slot.
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='failed' WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	env.orch.mu.Lock()
	delete(env.orch.runs, parentID)
	env.orch.mu.Unlock()
	close(parentDone)

	select {
	case <-watcherExit:
	case <-time.After(2 * time.Second):
		t.Fatal("watchPlanSuspendedChild did not return after parent terminated")
	}

	head, err := repo.NewPlanRepo(env.pool, clock.Real()).GetHead(childID)
	if err != nil {
		t.Fatalf("GetHead(child): %v", err)
	}
	if head.Status != model.PlanStatusCancelled {
		t.Errorf("child plan status = %v, want cancelled", head.Status)
	}
	wi := env.getWorkflowInstance(t, childID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("child instance status = %v, want failed", wi.Status)
	}
}

// TestWatchPlanSuspendedChild_ChildResumes_StopsPollingWithoutTouchingChild:
// the child leaves plan-suspended status on its own (simulating a real
// ResumeAfterPlanApproval) without the parent ever terminating — the watcher
// must return on its own and touch nothing.
func TestWatchPlanSuspendedChild_ChildResumes_StopsPollingWithoutTouchingChild(t *testing.T) {
	oldInterval := subworkflowWatchInterval
	subworkflowWatchInterval = 2 * time.Millisecond
	defer func() { subworkflowWatchInterval = oldInterval }()

	env := newTestEnv(t)
	childID := env.initProjectWorkflow(t, "test")
	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(childID, model.WorkflowInstanceWaitingApproval); err != nil {
		t.Fatalf("UpdateStatus(child): %v", err)
	}

	parentDone := make(chan struct{}) // never fired
	watcherExit := make(chan struct{})
	go func() {
		env.orch.watchPlanSuspendedChild("parent-never-touched", parentDone, childID)
		close(watcherExit)
	}()

	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='active' WHERE id=?`, childID); err != nil {
		t.Fatal(err)
	}

	select {
	case <-watcherExit:
	case <-time.After(2 * time.Second):
		t.Fatal("watchPlanSuspendedChild did not return after child left plan-suspended status")
	}

	wi := env.getWorkflowInstance(t, childID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("child instance status = %v, want unchanged active (watcher must not touch it)", wi.Status)
	}
}

// TestWatchPlanSuspendedChild_NotPlanSuspended_ReturnsImmediately covers the
// early-return guard: a child that was never plan-suspended returns at once.
func TestWatchPlanSuspendedChild_NotPlanSuspended_ReturnsImmediately(t *testing.T) {
	env := newTestEnv(t)
	childID := env.initProjectWorkflow(t, "test")

	done := make(chan struct{})
	go func() {
		env.orch.watchPlanSuspendedChild("parent-x", make(chan struct{}), childID)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watchPlanSuspendedChild did not return immediately for a non-plan-suspended child")
	}
}
