package spawner

import (
	"context"
	"testing"
	"time"
)

// --- RotateSignals ---

func TestRotateSignals_UnknownSessionReturnsZeroZero(t *testing.T) {
	t.Parallel()
	sp, env, _ := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	tokens, threshold := sp.RotateSignals("no-such-session")
	if tokens != 0 || threshold != 0 {
		t.Errorf("RotateSignals(unknown) = (%d, %d), want (0, 0)", tokens, threshold)
	}
}

func TestRotateSignals_NoLedgerButKnownProcReturnsZeroZero(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	proc, _ := newProactiveTestProc(env, clk, "sess-noledger", "cli_interactive", false)
	proc.proactiveRestartThreshold = 250000
	sp.registerSessionProc(proc.sessionID, proc)

	tokens, threshold := sp.RotateSignals(proc.sessionID)
	if tokens != 0 || threshold != 0 {
		t.Errorf("RotateSignals(no ledger) = (%d, %d), want (0, 0) — a threshold without ledger data must not report as rotate-eligible", tokens, threshold)
	}
}

func TestRotateSignals_ReturnsLedgerTotalAndResolvedThreshold(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	proc, _ := newProactiveTestProc(env, clk, "sess-rotsig", "cli_interactive", false)
	proc.proactiveRestartThreshold = 123456
	sp.registerSessionProc(proc.sessionID, proc)
	t.Cleanup(func() { globalLedgerStore.drop(proc.sessionID) })
	globalLedgerStore.get(proc.sessionID).append(LedgerKindDialog, 77000, "", "", false)

	tokens, threshold := sp.RotateSignals(proc.sessionID)
	if tokens != 77000 {
		t.Errorf("RotateSignals tokens = %d, want 77000", tokens)
	}
	if threshold != 123456 {
		t.Errorf("RotateSignals threshold = %d, want 123456", threshold)
	}
}

// --- NoteStepBoundary ---

func TestNoteStepBoundary_UnknownSessionIsNoOp(t *testing.T) {
	t.Parallel()
	sp, env, _ := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	// Must not panic for a session with no tracked ledger.
	sp.NoteStepBoundary("no-such-session")

	st := globalRestartStore.snapshot("no-such-session")
	if st.lastBoundaryTurn != 0 {
		t.Errorf("lastBoundaryTurn = %d, want 0 (no-op)", st.lastBoundaryTurn)
	}
}

func TestNoteStepBoundary_StampsCurrentLedgerTurn(t *testing.T) {
	t.Parallel()
	sp, env, _ := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sessionID := "sess-boundary"
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})
	globalLedgerStore.get(sessionID).append(LedgerKindDialog, 1000, "", "", false)
	turn, ok := globalLedgerStore.turnNow(sessionID)
	if !ok {
		t.Fatal("turnNow: expected a tracked ledger after append")
	}

	sp.NoteStepBoundary(sessionID)

	st := globalRestartStore.snapshot(sessionID)
	if st.lastBoundaryTurn != turn {
		t.Errorf("lastBoundaryTurn = %d, want %d", st.lastBoundaryTurn, turn)
	}
}

// --- RequestStepRotation ---

func TestRequestStepRotation_NonBlockingSendOntoStepRotateCh(t *testing.T) {
	t.Parallel()
	sp, env, _ := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sp.RequestStepRotation("sess-req-1")

	select {
	case got := <-sp.stepRotateCh:
		if got != "sess-req-1" {
			t.Errorf("stepRotateCh received %q, want sess-req-1", got)
		}
	default:
		t.Fatal("stepRotateCh: expected a buffered value, got none")
	}
}

func TestRequestStepRotation_FullChannelDropsSilently(t *testing.T) {
	t.Parallel()
	sp, env, _ := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	// stepRotateCh is buffered 4 (spawner.go). Filling it and sending one
	// more must not block the calling goroutine.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			sp.RequestStepRotation("sess-full")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RequestStepRotation blocked on a full channel")
	}
}

// --- dispatchStepRotation ---

func TestDispatchStepRotation_FiresFullChain(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})

	proc, backend := newProactiveTestProc(env, clk, sessionID, "cli_interactive", false)
	globalLedgerStore.get(sessionID).append(LedgerKindDialog, 300000, "", "", false)

	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.dispatchStepRotation(context.Background(), []*processInfo{proc}, req, sessionID)

	if proc.doneCh == originalDoneCh {
		t.Fatal("dispatchStepRotation did not replace proc.doneCh")
	}
	if !proc.proactiveRotationPending {
		t.Error("proc.proactiveRotationPending = false, want true")
	}
	if !proc.lowContextSaving {
		t.Error("proc.lowContextSaving = false, want true")
	}
	if proc.proactiveTokensBefore != 300000 {
		t.Errorf("proc.proactiveTokensBefore = %d, want 300000", proc.proactiveTokensBefore)
	}

	waitForDoneCh(t, proc.doneCh, 5*time.Second)
	if !backend.wasKilled() {
		t.Error("backend.Kill was never called")
	}

	st := globalRestartStore.snapshot(sessionID)
	if st.restartsDone != 1 {
		t.Errorf("restartsDone = %d, want 1 (NoteProactiveRestart must fire)", st.restartsDone)
	}
}

func TestDispatchStepRotation_UnknownSessionNoOp(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	proc, backend := newProactiveTestProc(env, clk, "sess-known", "cli_interactive", false)
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.dispatchStepRotation(context.Background(), []*processInfo{proc}, req, "sess-not-running")

	if proc.doneCh != originalDoneCh {
		t.Error("dispatchStepRotation mutated an unrelated proc for an unknown sessionID")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill called for an unknown sessionID")
	}
}

func TestDispatchStepRotation_AlreadySavingNoOp(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	proc, backend := newProactiveTestProc(env, clk, "sess-saving", "cli_interactive", false)
	proc.lowContextSaving = true
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.dispatchStepRotation(context.Background(), []*processInfo{proc}, req, proc.sessionID)

	if proc.doneCh != originalDoneCh {
		t.Error("dispatchStepRotation fired while a save was already in flight")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill called while a save was already in flight")
	}
}

func TestDispatchStepRotation_NonTrackingBackendNoOp(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	proc, backend := newProactiveTestProc(env, clk, "sess-notrack", "script", false)
	backend.tracksContext = false
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.dispatchStepRotation(context.Background(), []*processInfo{proc}, req, proc.sessionID)

	if proc.doneCh != originalDoneCh {
		t.Error("dispatchStepRotation fired for a backend that does not track context")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill called for a backend that does not track context")
	}
}

// TestDispatchStepRotation_MutatesOnlyMatchingProc verifies a second,
// unrelated running proc is left completely untouched.
func TestDispatchStepRotation_MutatesOnlyMatchingProc(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	target := env.createSessionWithFindings(t, map[string]interface{}{})
	t.Cleanup(func() {
		globalLedgerStore.drop(target)
		DropProactiveRestartState(target)
	})
	targetProc, targetBackend := newProactiveTestProc(env, clk, target, "cli_interactive", false)
	globalLedgerStore.get(target).append(LedgerKindDialog, 300000, "", "", false)

	otherProc, otherBackend := newProactiveTestProc(env, clk, "sess-other", "cli_interactive", false)
	otherDoneCh := otherProc.doneCh

	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}

	sp.dispatchStepRotation(context.Background(), []*processInfo{otherProc, targetProc}, req, target)

	waitForDoneCh(t, targetProc.doneCh, 5*time.Second)
	if !targetBackend.wasKilled() {
		t.Error("target proc's backend was never killed")
	}
	if otherProc.doneCh != otherDoneCh {
		t.Error("unrelated proc's doneCh was replaced")
	}
	if otherBackend.wasKilled() {
		t.Error("unrelated proc's backend was killed")
	}
}
