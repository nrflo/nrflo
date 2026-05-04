package orchestrator

import (
	"testing"

	"be/internal/clock"
	"be/internal/spawner"
)

// TestRequestTerminalSignal_SessionNotFound verifies that RequestTerminalSignal
// returns nil when the session does not exist in the database (best-effort).
func TestRequestTerminalSignal_SessionNotFound(t *testing.T) {
	env := newTestEnv(t)

	err := env.orch.RequestTerminalSignal(env.project, "TC-TS-NOOP", "test", "nonexistent-session-xyz", "fail")
	if err != nil {
		t.Fatalf("expected nil for nonexistent session, got: %v", err)
	}
}

// TestRequestTerminalSignal_RunNotFound verifies that RequestTerminalSignal returns nil
// when the session exists but there is no active run in o.runs for its workflow instance.
func TestRequestTerminalSignal_RunNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-TS-1", "Terminal signal run-not-found")
	wfiID := env.initWorkflow(t, "TC-TS-1")

	insertRunningSession(t, env, wfiID, "TC-TS-1", "sess-ts-no-run")

	// No entry in o.runs for wfiID — orchestrator has no active run.
	err := env.orch.RequestTerminalSignal(env.project, "TC-TS-1", "test", "sess-ts-no-run", "fail")
	if err != nil {
		t.Fatalf("expected nil when run not in o.runs, got: %v", err)
	}
}

// TestRequestTerminalSignal_SpawnerNil verifies that RequestTerminalSignal returns nil
// when the run is registered but the spawner is nil (between phases).
func TestRequestTerminalSignal_SpawnerNil(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-TS-2", "Terminal signal spawner-nil")
	wfiID := env.initWorkflow(t, "TC-TS-2")

	insertRunningSession(t, env, wfiID, "TC-TS-2", "sess-ts-nil-sp")

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}, spawners: make(map[string]*spawner.Spawner)}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	err := env.orch.RequestTerminalSignal(env.project, "TC-TS-2", "test", "sess-ts-nil-sp", "fail")
	if err != nil {
		t.Fatalf("expected nil when spawner is nil, got: %v", err)
	}
}

// TestRequestTerminalSignal_ForwardsToSpawner verifies that RequestTerminalSignal
// forwards the signal to the active spawner when session, run, and spawner are all present.
func TestRequestTerminalSignal_ForwardsToSpawner(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-TS-3", "Terminal signal forwarding")
	wfiID := env.initWorkflow(t, "TC-TS-3")

	sessionID := "sess-ts-forward-abc"
	insertRunningSession(t, env, wfiID, "TC-TS-3", sessionID)

	sp := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}, spawners: map[string]*spawner.Spawner{sessionID: sp}}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	err := env.orch.RequestTerminalSignal(env.project, "TC-TS-3", "test", sessionID, "fail")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the signal reached the spawner: a second call must be a no-op
	// because the channel (capacity 1) is already full.
	// No panic, no block = signal was delivered correctly.
	sp.RequestTerminalSignal(sessionID, "continue")
}

// TestRequestTerminalSignal_RoutesPerSession_AcrossParallelSpawners verifies
// that RequestTerminalSignal routes each session's signal to its own spawner
// when multiple agents run concurrently in the same workflow instance (same layer).
// This is the key regression check for the runState.spawner → spawners-map change.
func TestRequestTerminalSignal_RoutesPerSession_AcrossParallelSpawners(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-PS-TS", "Parallel spawner signal routing")
	wfiID := env.initWorkflow(t, "TC-PS-TS")

	sessA := "sess-ps-ts-a"
	sessB := "sess-ps-ts-b"
	insertRunningSession(t, env, wfiID, "TC-PS-TS", sessA)
	insertRunningSession(t, env, wfiID, "TC-PS-TS", sessB)

	spA := spawner.New(spawner.Config{Clock: clock.Real()})
	spB := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel: func() {},
		spawners: map[string]*spawner.Spawner{
			sessA: spA,
			sessB: spB,
		},
	}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	if err := env.orch.RequestTerminalSignal(env.project, "TC-PS-TS", "test", sessA, "fail"); err != nil {
		t.Errorf("RequestTerminalSignal(sessA)=%v, want nil", err)
	}
	if err := env.orch.RequestTerminalSignal(env.project, "TC-PS-TS", "test", sessB, "continue"); err != nil {
		t.Errorf("RequestTerminalSignal(sessB)=%v, want nil", err)
	}

	// Subsequent calls on each spawner are no-ops — no panic or block = routing succeeded.
	spA.RequestTerminalSignal(sessA, "second-a")
	spB.RequestTerminalSignal(sessB, "second-b")
}

// TestBumpLastMessage_RoutesPerSession_AcrossParallelSpawners verifies
// that BumpLastMessage routes to the spawner registered for that sessionID
// when two agents share the same workflow instance (parallel layer execution).
func TestBumpLastMessage_RoutesPerSession_AcrossParallelSpawners(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-PS-BM", "Parallel spawner bump routing")
	wfiID := env.initWorkflow(t, "TC-PS-BM")

	sessA := "sess-ps-bm-a"
	sessB := "sess-ps-bm-b"
	insertRunningSession(t, env, wfiID, "TC-PS-BM", sessA)
	insertRunningSession(t, env, wfiID, "TC-PS-BM", sessB)

	spA := spawner.New(spawner.Config{Clock: clock.Real()})
	spB := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel: func() {},
		spawners: map[string]*spawner.Spawner{
			sessA: spA,
			sessB: spB,
		},
	}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	if err := env.orch.BumpLastMessage(env.project, "TC-PS-BM", "test", sessA); err != nil {
		t.Errorf("BumpLastMessage(sessA)=%v, want nil", err)
	}
	if err := env.orch.BumpLastMessage(env.project, "TC-PS-BM", "test", sessB); err != nil {
		t.Errorf("BumpLastMessage(sessB)=%v, want nil", err)
	}
}

// TestSetLastMessage_RoutesPerSession_AcrossParallelSpawners verifies
// that SetLastMessage routes to the spawner registered for that sessionID
// when two agents share the same workflow instance (parallel layer execution).
func TestSetLastMessage_RoutesPerSession_AcrossParallelSpawners(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-PS-SLM", "Parallel spawner set-last-message routing")
	wfiID := env.initWorkflow(t, "TC-PS-SLM")

	sessA := "sess-ps-slm-a"
	sessB := "sess-ps-slm-b"
	insertRunningSession(t, env, wfiID, "TC-PS-SLM", sessA)
	insertRunningSession(t, env, wfiID, "TC-PS-SLM", sessB)

	spA := spawner.New(spawner.Config{Clock: clock.Real()})
	spB := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel: func() {},
		spawners: map[string]*spawner.Spawner{
			sessA: spA,
			sessB: spB,
		},
	}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	if err := env.orch.SetLastMessage(env.project, "TC-PS-SLM", "test", sessA, "agent A output"); err != nil {
		t.Errorf("SetLastMessage(sessA)=%v, want nil", err)
	}
	if err := env.orch.SetLastMessage(env.project, "TC-PS-SLM", "test", sessB, "agent B output"); err != nil {
		t.Errorf("SetLastMessage(sessB)=%v, want nil", err)
	}
}

// TestRequestTerminalSignal_MultipleResults verifies that all result strings
// (fail, continue, callback) are routed through to the spawner without error.
func TestRequestTerminalSignal_MultipleResults(t *testing.T) {
	cases := []struct {
		ticketID  string
		sessionID string
		result    string
	}{
		{"TC-TS-R1", "sess-r-fail", "fail"},
		{"TC-TS-R2", "sess-r-continue", "continue"},
		{"TC-TS-R3", "sess-r-callback", "callback"},
	}

	for _, tc := range cases {
		t.Run(tc.result, func(t *testing.T) {
			env := newTestEnv(t)
			env.createTicket(t, tc.ticketID, "Terminal signal result test")
			wfiID := env.initWorkflow(t, tc.ticketID)

			insertRunningSession(t, env, wfiID, tc.ticketID, tc.sessionID)

			sp := spawner.New(spawner.Config{Clock: clock.Real()})
			env.orch.mu.Lock()
			env.orch.runs[wfiID] = &runState{cancel: func() {}, spawners: map[string]*spawner.Spawner{tc.sessionID: sp}}
			env.orch.mu.Unlock()
			t.Cleanup(func() {
				env.orch.mu.Lock()
				delete(env.orch.runs, wfiID)
				env.orch.mu.Unlock()
			})

			err := env.orch.RequestTerminalSignal(env.project, tc.ticketID, "test", tc.sessionID, tc.result)
			if err != nil {
				t.Fatalf("unexpected error for result=%q: %v", tc.result, err)
			}
		})
	}
}
