package spawner

import (
	"context"
	"testing"
)

// TestFinalizePhase_DropsLedger verifies finalizePhase drops the context
// ledger for every completed session — the "drop on session end" rule that
// applies regardless of PASS/FAIL/SKIPPED outcome.
func TestFinalizePhase_DropsLedger(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	sessionID := env.sessionID
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })

	l := globalLedgerStore.get(sessionID)
	l.append(LedgerKindDialog, 10, "", "", false)
	if _, ok := globalLedgerStore.snapshot(sessionID); !ok {
		t.Fatalf("precondition: ledger not tracked before finalizePhase")
	}

	proc := &processInfo{
		sessionID:          sessionID,
		agentID:            "test-agent",
		modelID:            "claude:sonnet-5",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "PASS",
	}

	if err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase"); err != nil {
		t.Fatalf("finalizePhase() error = %v, want nil (PASS)", err)
	}

	if _, ok := globalLedgerStore.snapshot(sessionID); ok {
		t.Errorf("ledger still tracked after finalizePhase, want dropped")
	}
}

// TestCancelRunningProcs_DropsLedger verifies cancelRunningProcs (the
// ctx.Done() cancellation path, which never reaches finalizePhase) also drops
// the cancelled session's context ledger.
func TestCancelRunningProcs_DropsLedger(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	sessionID := env.sessionID
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })

	l := globalLedgerStore.get(sessionID)
	l.append(LedgerKindDialog, 10, "", "", false)

	doneCh := make(chan struct{})
	close(doneCh) // simulate an already-exited process: Kill's grace wait resolves immediately
	proc := &processInfo{
		sessionID:    sessionID,
		agentID:      "test-agent",
		modelID:      "claude:sonnet-5",
		projectID:    env.projectID,
		ticketID:     env.ticketID,
		workflowName: env.workflowID,
		backend:      fakeBackend{name: "claude", supportsResume: true},
		doneCh:       doneCh,
	}

	completed := env.spawner.cancelRunningProcs(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
	})

	if len(completed) != 1 || completed[0].finalStatus != "CANCELLED" {
		t.Fatalf("completed = %+v, want 1 proc marked CANCELLED", completed)
	}
	if _, ok := globalLedgerStore.snapshot(sessionID); ok {
		t.Errorf("ledger still tracked after cancelRunningProcs, want dropped")
	}
}
