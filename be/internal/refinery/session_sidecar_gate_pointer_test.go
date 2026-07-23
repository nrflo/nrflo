package refinery

import (
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// TestFoldGate_SkipDoesNotAdvancePointer proves nextFoldSeq stays put
// across a gated-out fold: message A is seeded while context_left=80
// (skipped), then context_left drops to 20 and message B is seeded; the
// next fold's user text must contain BOTH A and B.
func TestFoldGate_SkipDoesNotAdvancePointer(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-pointer", "proj-gate-pointer"
	wfiID, nodeID := "wfi-gate-pointer", "node-gate-pointer"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(80))
	seedMessages(t, pool, clk, sessionID, "message-A")

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest after gate-open fold")
	stubBuildProvider(t, prov)
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)
	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Fatalf("GetSlot after skipped fold = %+v, want nil", s)
	}

	setContextLeft(t, pool, sessionID, intPtr(20))
	seedMessages(t, pool, clk, sessionID, "message-B")
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
	text := prov.lastUserText()
	if !strings.Contains(text, "message-A") {
		t.Errorf("fold user text = %q, want it to contain message-A (skip must not advance nextFoldSeq)", text)
	}
	if !strings.Contains(text, "message-B") {
		t.Errorf("fold user text = %q, want it to contain message-B", text)
	}
}

// TestFoldGate_StopSessionFinalFold_AlsoGated verifies StopSession's
// synchronous final fold is gated exactly like a live trigger: a session
// stopping at context_left=80 must produce no digest.
func TestFoldGate_StopSessionFinalFold_AlsoGated(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-stop", "proj-gate-stop"
	wfiID, nodeID := "wfi-gate-stop", "node-gate-stop"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(80))
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)

	mgr.StopSession(sessionID)

	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot after StopSession at context_left=80 = %+v, want nil (final fold gated too)", s)
	}
}

// TestFoldGate_SkippedFold_NoCostAttributionOrBroadcast extends the
// SetCostAttributor/SetBroadcaster capture pattern to verify a gate-closed
// fold invokes neither seam.
func TestFoldGate_SkippedFold_NoCostAttributionOrBroadcast(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-noattr", "proj-gate-noattr"
	wfiID, nodeID := "wfi-gate-noattr", "node-gate-noattr"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(80))
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))

	var mu sync.Mutex
	var costCalls int
	mgr.SetCostAttributor(func(sid string, in, out, cacheRead, cacheWrite int) {
		mu.Lock()
		defer mu.Unlock()
		costCalls++
	})
	bc := &capturingBroadcaster{}
	mgr.SetBroadcaster(bc.capture)

	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)

	mu.Lock()
	gotCostCalls := costCalls
	mu.Unlock()
	if gotCostCalls != 0 {
		t.Errorf("cost attributor calls after a skipped fold = %d, want 0", gotCostCalls)
	}
	if got := len(bc.snapshot()); got != 0 {
		t.Errorf("broadcaster calls after a skipped fold = %d, want 0", got)
	}
}
