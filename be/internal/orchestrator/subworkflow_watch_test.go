package orchestrator

import (
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// closeOnce returns an idempotent closer (env cleanup also cancels leftover runs).
func closeOnce(ch chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

// fakeCancel mimics a real run's cancel: it marks the stop AND closes the run's
// done channel (env cleanup cancels leftover runs then WAITS on their done).
func fakeCancel(stopped, done chan struct{}) func() {
	s, d := closeOnce(stopped), closeOnce(done)
	return func() { s(); d() }
}

// TestWatchSubworkflow_ParentPauseReArms verifies the pause-tolerant cascade:
// a paused parent (status=waiting, run slot released) must NOT kill its child;
// the watcher re-arms on the successor runState and only stops the child when
// the parent reaches a terminal status.
func TestWatchSubworkflow_ParentPauseReArms(t *testing.T) {
	oldInterval := subworkflowWatchInterval
	subworkflowWatchInterval = 2 * time.Millisecond
	defer func() { subworkflowWatchInterval = oldInterval }()

	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")
	childID := env.initProjectWorkflow(t, "test")

	childDone := make(chan struct{})
	childStopped := make(chan struct{})
	parentDone1 := make(chan struct{})
	env.orch.mu.Lock()
	env.orch.runs[childID] = &runState{done: childDone, cancel: fakeCancel(childStopped, childDone)}
	env.orch.runs[parentID] = &runState{done: parentDone1, cancel: func() {}}
	env.orch.mu.Unlock()

	watcherExit := make(chan struct{})
	go func() {
		env.orch.watchSubworkflow(parentID, parentDone1, childID, false)
		close(watcherExit)
	}()

	// Pause: parent status -> waiting, run slot released, done closed.
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='waiting' WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	env.orch.mu.Lock()
	delete(env.orch.runs, parentID)
	env.orch.mu.Unlock()
	close(parentDone1)

	select {
	case <-childStopped:
		t.Fatal("child was stopped on parent PAUSE; must survive until parent is terminal")
	case <-time.After(50 * time.Millisecond):
	}

	// Resume: successor runState with a fresh done channel.
	parentDone2 := make(chan struct{})
	env.orch.mu.Lock()
	env.orch.runs[parentID] = &runState{done: parentDone2, cancel: func() {}}
	env.orch.mu.Unlock()
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='active' WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}

	// Terminal: parent fails and its run slot is released for good.
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='failed' WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	env.orch.mu.Lock()
	delete(env.orch.runs, parentID)
	env.orch.mu.Unlock()
	close(parentDone2)

	select {
	case <-childStopped: // Stop() fired rs.cancel, which closes childStopped
	case <-time.After(2 * time.Second):
		t.Fatal("child was not stopped after parent reached a terminal status")
	}
	// childDone is closed by the fake cancel (Stop path), letting the watcher exit.
	select {
	case <-watcherExit:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not exit after child ended")
	}
}

// TestRearmSubworkflowWatcher_TopLevelNoop ensures top-level runs are ignored
// and a sub-run with a terminal parent is left detached (human-owned recovery).
func TestRearmSubworkflowWatcher_TerminalParentDetaches(t *testing.T) {
	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")
	childID := env.initProjectWorkflow(t, "test")
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='failed' WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET parent_instance_id=? WHERE id=?`, parentID, childID); err != nil {
		t.Fatal(err)
	}

	childStopped := make(chan struct{})
	childDone := make(chan struct{})
	env.orch.mu.Lock()
	env.orch.runs[childID] = &runState{done: childDone, cancel: fakeCancel(childStopped, childDone)}
	env.orch.mu.Unlock()

	wi, err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).Get(childID)
	if err != nil {
		t.Fatal(err)
	}
	env.orch.rearmSubworkflowWatcher(wi)

	select {
	case <-childStopped:
		t.Fatal("recovered child with terminal parent must run detached, not be stopped")
	case <-time.After(30 * time.Millisecond):
	}
}
