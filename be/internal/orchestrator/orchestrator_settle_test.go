package orchestrator

import (
	"context"
	"testing"
)

// TestWaitForRunToSettle covers the fast paths of the teardown-window guard shared by
// retry-failed and continue: no run, a run with no done channel, a run whose done
// channel closes after releasing its slot, and a cancelled caller context.
func TestWaitForRunToSettle(t *testing.T) {
	env := newTestEnv(t)

	// Purge the runs this test injects so newTestEnv's cleanup (which waits on every
	// run's done channel) doesn't block on the never-closing channels below. Runs
	// LIFO-before the env cleanup.
	t.Cleanup(func() {
		env.orch.mu.Lock()
		for _, id := range []string{"wfi-nodone", "wfi-settling", "wfi-blocked"} {
			delete(env.orch.runs, id)
		}
		env.orch.mu.Unlock()
	})

	// Unregistered instance → already settled.
	if err := env.orch.waitForRunToSettle(context.Background(), "wfi-none"); err != nil {
		t.Fatalf("expected nil for unregistered instance, got: %v", err)
	}

	// Registered run with no done channel → treated as genuinely active.
	env.orch.mu.Lock()
	env.orch.runs["wfi-nodone"] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	if err := env.orch.waitForRunToSettle(context.Background(), "wfi-nodone"); err == nil ||
		err.Error() != "workflow is already running" {
		t.Fatalf("expected 'workflow is already running', got: %v", err)
	}

	// Registered run whose teardown completes (slot deleted, then done closed) → settles.
	done := make(chan struct{})
	env.orch.mu.Lock()
	env.orch.runs["wfi-settling"] = &runState{cancel: func() {}, done: done}
	env.orch.mu.Unlock()
	go func() {
		// Mirror the runLoop deferred cleanup ordering: delete the slot, then close done.
		env.orch.mu.Lock()
		delete(env.orch.runs, "wfi-settling")
		env.orch.mu.Unlock()
		close(done)
	}()
	if err := env.orch.waitForRunToSettle(context.Background(), "wfi-settling"); err != nil {
		t.Fatalf("expected nil after teardown, got: %v", err)
	}
	env.orch.mu.Lock()
	_, stillThere := env.orch.runs["wfi-settling"]
	env.orch.mu.Unlock()
	if stillThere {
		t.Fatal("slot should be released after settle")
	}

	// Cancelled caller context with a never-closing run → returns the context error
	// promptly instead of blocking for runSettleTimeout.
	env.orch.mu.Lock()
	env.orch.runs["wfi-blocked"] = &runState{cancel: func() {}, done: make(chan struct{})}
	env.orch.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := env.orch.waitForRunToSettle(ctx, "wfi-blocked"); err == nil {
		t.Fatal("expected context error for cancelled ctx")
	}
}
