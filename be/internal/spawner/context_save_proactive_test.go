package spawner

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// trackingFakeBackend is an ExecutionBackend stub for checkProactiveRestart
// tests: TracksContext/SupportsResume are configurable per case, and Kill
// closes killCh (mirroring how a real backend's wait goroutine captures
// proc.doneCh as a closure at spawn time — see backend_interactive.go:173 —
// so it stays correct even after checkProactiveRestart reassigns
// proc.doneCh to a fresh channel before the kill happens).
type trackingFakeBackend struct {
	name           string
	supportsResume bool
	tracksContext  bool

	mu       sync.Mutex
	killed   bool
	killCh   chan struct{}
	killOnce sync.Once
}

func (b *trackingFakeBackend) Name() string                    { return b.name }
func (b *trackingFakeBackend) SupportsResume() bool            { return b.supportsResume }
func (b *trackingFakeBackend) SupportsTakeControl() bool       { return false }
func (b *trackingFakeBackend) RequiresPrompt() bool            { return false }
func (b *trackingFakeBackend) TracksContext() bool             { return b.tracksContext }
func (b *trackingFakeBackend) ParsesStructuredOutput() bool    { return false }
func (b *trackingFakeBackend) NaturalExitGrace() time.Duration { return 0 }
func (b *trackingFakeBackend) Start(_ context.Context, _ *processInfo, _ *prepResult) error {
	return nil
}
func (b *trackingFakeBackend) Kill(_ context.Context, _ *processInfo, _ syscall.Signal) error {
	b.mu.Lock()
	b.killed = true
	b.mu.Unlock()
	b.killOnce.Do(func() { close(b.killCh) })
	return nil
}
func (b *trackingFakeBackend) wasKilled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.killed
}

// newProactiveRestartTestEnv wires a real DB pool (template-copied) plus a
// controllable test clock, so checkProactiveRestart's idle/config reads
// exercise real code without a real CLI process.
func newProactiveRestartTestEnv(t *testing.T) (*Spawner, *contextSaveTestEnv, *clock.TestClock) {
	t.Helper()
	env := setupContextSaveTestEnv(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clk,
	})
	return sp, env, clk
}

// newProactiveTestProc builds a processInfo + backend over budget and idle by
// default; callers tweak fields for each scenario. killCh is proc.doneCh at
// construction time.
func newProactiveTestProc(env *contextSaveTestEnv, clk *clock.TestClock, sessionID, backendName string, supportsResume bool) (*processInfo, *trackingFakeBackend) {
	killCh := make(chan struct{})
	backend := &trackingFakeBackend{name: backendName, supportsResume: supportsResume, tracksContext: true, killCh: killCh}
	proc := &processInfo{
		sessionID:                 sessionID,
		agentType:                 "test-agent",
		modelID:                   "unknown:model",
		projectID:                 env.projectID,
		ticketID:                  env.ticketID,
		workflowName:              "feature",
		workflowInstanceID:        env.wfiID,
		backend:                   backend,
		doneCh:                    killCh,
		lastMessageTime:           clk.Now().Add(-1 * time.Hour),
		hasReceivedMessage:        true,
		idleAfterMessageTimeout:   time.Minute,
		idleStartTimeout:          time.Minute,
		proactiveRestartThreshold: 250000,
	}
	return proc, backend
}

func waitForDoneCh(t *testing.T, ch chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for proactive-restart chain to complete")
	}
}

// TestCheckProactiveRestart_IdleBoundaryCrossing_FiresFullChain covers
// acceptance case (1)/(3): a cli-fake and a codex-shaped fake, both over
// threshold at an idle boundary with no in-flight plan item, fire the
// kill->save->CONTINUE chain and reset the restart's rotation-pending state.
// Both backends are configured SupportsResume=false so the save flow takes
// the agent-save path (contextSaveViaAgent) — the resume path spawns a real
// PTY and is intentionally never exercised here (repo rule: no real CLI).
func TestCheckProactiveRestart_IdleBoundaryCrossing_FiresFullChain(t *testing.T) {
	backends := []struct {
		name        string
		backendName string
	}{
		{name: "cli-shaped backend", backendName: "cli_interactive"},
		{name: "codex-shaped backend", backendName: "codex_appserver"},
	}
	for _, bc := range backends {
		t.Run(bc.name, func(t *testing.T) {
			sp, env, clk := newProactiveRestartTestEnv(t)
			defer env.cleanup()

			sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
			t.Cleanup(func() {
				globalLedgerStore.drop(sessionID)
				DropProactiveRestartState(sessionID)
			})

			proc, backend := newProactiveTestProc(env, clk, sessionID, bc.backendName, false)
			globalLedgerStore.get(sessionID).append(LedgerKindDialog, 300000, "", "", false)

			req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
			originalDoneCh := proc.doneCh

			sp.checkProactiveRestart(context.Background(), proc, req)

			if proc.doneCh == originalDoneCh {
				t.Fatal("checkProactiveRestart did not replace proc.doneCh; policy did not fire")
			}
			if !proc.proactiveRotationPending {
				t.Error("proc.proactiveRotationPending = false, want true after firing")
			}
			if proc.proactiveTokensBefore != 300000 {
				t.Errorf("proc.proactiveTokensBefore = %d, want 300000", proc.proactiveTokensBefore)
			}

			waitForDoneCh(t, proc.doneCh, 5*time.Second)

			if !backend.wasKilled() {
				t.Error("backend.Kill was never called")
			}
			if proc.finalStatus != "CONTINUE" {
				t.Errorf("proc.finalStatus = %q, want CONTINUE", proc.finalStatus)
			}

			st := globalRestartStore.snapshot(sessionID)
			if st.restartsDone != 1 {
				t.Errorf("restartsDone = %d, want 1 (NoteProactiveRestart must fire on trigger)", st.restartsDone)
			}
		})
	}
}

// TestCheckProactiveRestart_MidTurnCrossing_Defers covers acceptance case
// (2): a session over threshold but NOT idle (recent message activity, i.e.
// mid-tool-chain) must never fire — the mid-tool-chain guard.
func TestCheckProactiveRestart_MidTurnCrossing_Defers(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})

	proc, backend := newProactiveTestProc(env, clk, sessionID, "cli_interactive", false)
	proc.lastMessageTime = clk.Now() // activity "just now" — not idle
	globalLedgerStore.get(sessionID).append(LedgerKindDialog, 300000, "", "", false)

	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.checkProactiveRestart(context.Background(), proc, req)

	if proc.doneCh != originalDoneCh {
		t.Error("checkProactiveRestart fired mid-turn (not idle); doneCh was replaced")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill was called mid-turn; must defer until idle")
	}
	if proc.proactiveRotationPending {
		t.Error("proc.proactiveRotationPending = true after a deferred (non-firing) check")
	}
}

// TestCheckProactiveRestart_ThresholdZero_RegressionNoOp is the required
// regression: with the resolved threshold at 0 (disabled, either via a
// per-def override or the unseeded global default), checkProactiveRestart
// must have zero effect — the existing emergency low-context branch in
// monitorAll is untouched by this feature when disabled.
func TestCheckProactiveRestart_ThresholdZero_RegressionNoOp(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})

	proc, backend := newProactiveTestProc(env, clk, sessionID, "cli_interactive", false)
	proc.proactiveRestartThreshold = 0 // disabled
	globalLedgerStore.get(sessionID).append(LedgerKindDialog, 300000, "", "", false)

	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.checkProactiveRestart(context.Background(), proc, req)

	if proc.doneCh != originalDoneCh {
		t.Error("checkProactiveRestart fired with threshold=0 (disabled); doneCh was replaced")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill was called with threshold=0 (disabled)")
	}
	if proc.lowContextSaving {
		t.Error("proc.lowContextSaving = true with threshold=0 (disabled)")
	}
}

// TestCheckProactiveRestart_BackendNotTrackingContext_NoOp verifies backends
// that don't track context (api/script in some configurations) are skipped
// even when every other gate would otherwise fire.
func TestCheckProactiveRestart_BackendNotTrackingContext_NoOp(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})

	proc, backend := newProactiveTestProc(env, clk, sessionID, "script", false)
	backend.tracksContext = false
	globalLedgerStore.get(sessionID).append(LedgerKindDialog, 300000, "", "", false)

	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.checkProactiveRestart(context.Background(), proc, req)

	if proc.doneCh != originalDoneCh {
		t.Error("checkProactiveRestart fired for a backend that does not track context")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill was called for a backend that does not track context")
	}
}

// TestCheckProactiveRestart_AlreadySaving_NoOp verifies a save already in
// flight (lowContextSaving=true, e.g. the emergency low-context path beat the
// proactive check to it) blocks a second concurrent trigger.
func TestCheckProactiveRestart_AlreadySaving_NoOp(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})

	proc, backend := newProactiveTestProc(env, clk, sessionID, "cli_interactive", false)
	proc.lowContextSaving = true
	globalLedgerStore.get(sessionID).append(LedgerKindDialog, 300000, "", "", false)

	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.checkProactiveRestart(context.Background(), proc, req)

	if proc.doneCh != originalDoneCh {
		t.Error("checkProactiveRestart fired while a save was already in flight")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill was called while a save was already in flight")
	}
}
