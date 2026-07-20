package orchestrator

import (
	"sync/atomic"
	"testing"
)

// TestStop_WaitsForRunLoopToDrainBeforeReturning is a deterministic regression
// guard for the race fixed in orchestrator_lifecycle.go: Stop must block on
// rs.done (mirroring StopAll/waitForRunToSettle) rather than returning right
// after cancel(), so callers never observe residual background work — e.g.
// writes into the run's TempDir-backed working directory — after Stop
// returns.
//
// It injects a synthetic runState whose "runLoop" is a goroutine that only
// starts tearing down once cancel() fires: it flips drained, deletes the
// o.runs entry, and closes done, exactly mirroring runLoop's real deferred
// cleanup order (orchestrator_loop.go). No time.Sleep and no post-Stop poll:
// if Stop returned before the goroutine finished, the assertions below would
// observe drained == false and/or the instance still present in o.runs.
func TestStop_WaitsForRunLoopToDrainBeforeReturning(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")

	var drained atomic.Bool
	cancelSignal := make(chan struct{})
	done := make(chan struct{})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel: func() { close(cancelSignal) },
		done:   done,
	}
	env.orch.mu.Unlock()

	go func() {
		<-cancelSignal
		drained.Store(true)
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
		close(done)
	}()

	if err := env.orch.Stop(wfiID); err != nil {
		t.Fatalf("Stop(%s): %v", wfiID, err)
	}

	// These must already hold the instant Stop returns — no waitForCondition,
	// no poll loop. A non-draining Stop (the pre-fix behavior) would flake
	// this under load exactly like the ticket's TempDir-not-empty failure.
	if !drained.Load() {
		t.Error("Stop returned before the simulated runLoop teardown finished draining")
	}
	if env.orch.IsInstanceRunning(wfiID) {
		t.Error("IsInstanceRunning() = true immediately after Stop returned, want false")
	}
}
