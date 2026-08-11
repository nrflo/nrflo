package refinery

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// TestFoldGate_CheapModelNeverFolds verifies a session running a cheap-tier
// (haiku-class) model never folds by default — the tier threshold resolves
// to 0 — even at rock-bottom context, and that setting the cheap tier key
// re-opens the gate for the same session.
func TestFoldGate_CheapModelNeverFolds(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-cheap-gate", "proj-cheap-gate"
	wfiID, nodeID := "wfi-cheap-gate", "node-cheap-gate"
	seedAutonomousSession(t, pool, sessionID, projectID) // context_left = 10
	if _, err := pool.Exec(`UPDATE agent_sessions SET model_id = 'haiku' WHERE id = ?`, sessionID); err != nil {
		t.Fatalf("set model_id: %v", err)
	}
	seedMessages(t, pool, clk, sessionID, "cheap-model-message")

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest for the re-enabled fold")
	stubBuildProvider(t, prov)
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)
	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Fatalf("GetSlot for cheap-model session = %+v, want nil (folding disabled by tier)", s)
	}

	// Re-enabling the tier opens the gate for the same session.
	if err := pool.SetConfig("refinery_fold_start_pct_cheap", "60"); err != nil {
		t.Fatalf("set cheap tier key: %v", err)
	}
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	waitForCondition(t, 2*time.Second, func() bool {
		return getSlot(t, mgr, wfiID, nodeID) != nil
	})
}
