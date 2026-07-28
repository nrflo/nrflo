package refinery

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// TestAutonomousTrigger_ContextUpdated_FoldsWithoutAnyFindings is the
// regression guard for a workflow session folding nothing for its entire life.
//
// findings.updated was once the only autonomous trigger, and every workflow
// node emits exactly one finding immediately before agent_finished — so the
// sole trigger fired as the session exited. Sessions ran for minutes, burned
// from full context down to the relaunch threshold, and folded zero times;
// the kill-time path then found no digest and spawned a context-saver, and
// the one fold it forced had the whole session to chew at once. This asserts
// the during-the-run trigger: no findings are ever emitted here, and a fold
// must still happen.
func TestAutonomousTrigger_ContextUpdated_FoldsWithoutAnyFindings(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-ctx-trigger", "proj-ctx-trigger"
	wfiID, nodeID := "wfi-ctx-trigger", "node-ctx-trigger"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(30))
	seedMessages(t, pool, clk, sessionID, "working")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("digest")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventAgentContextUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(50 * time.Millisecond) // let the sidecar goroutine register its debounce timer
	clk.Advance(30 * time.Second)

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
}

// TestAutonomousTrigger_ContextUpdated_RespectsDebounce pins context updates
// to the debounce floor rather than the immediate path. They arrive roughly
// per assistant turn, so folding on each one would run the local fold model
// continuously; only a task-boundary findings.updated folds immediately.
func TestAutonomousTrigger_ContextUpdated_RespectsDebounce(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-ctx-debounce", "proj-ctx-debounce"
	wfiID, nodeID := "wfi-ctx-debounce", "node-ctx-debounce"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(30))
	seedMessages(t, pool, clk, sessionID, "working")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("digest")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventAgentContextUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(50 * time.Millisecond)
	clk.Advance(29 * time.Second)

	settle(200 * time.Millisecond)
	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot after 29s = %+v, want nil (context updates debounce, they do not fold immediately)", s)
	}
}

// TestAutonomousTrigger_RequiresSessionID documents the plumbing contract the
// route depends on: it is keyed on Event.SessionID, so an emitter that leaves
// that field empty — carrying the id only inside the payload map — is
// silently not a trigger at all. Both context-update emitters did exactly
// that, which is why adding the event type alone would have fixed nothing.
func TestAutonomousTrigger_RequiresSessionID(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-ctx-nosid", "proj-ctx-nosid"
	wfiID, nodeID := "wfi-ctx-nosid", "node-ctx-nosid"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(30))
	seedMessages(t, pool, clk, sessionID, "working")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("digest")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	// SessionID deliberately empty; the id lives only in the payload.
	mgr.OnEvent(&ws.Event{
		Type:      ws.EventAgentContextUpdated,
		ProjectID: projectID,
		Data:      map[string]interface{}{"session_id": sessionID, "context_left": 30},
	})
	settle(50 * time.Millisecond)
	clk.Advance(30 * time.Second)

	settle(200 * time.Millisecond)
	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot = %+v, want nil (no Event.SessionID means no autonomous trigger)", s)
	}
}
