package refinery

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// newTestManager builds a Manager over a fresh migrated pool, a fixed test
// clock, and a mock provider stubbed in for buildProvider (unlimited scripts
// so any number of folds succeeds).
func newTestManager(t *testing.T) (*Manager, *clock.TestClock) {
	t.Helper()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mgr := NewManager(pool, clk)
	scripts := make([]mock.Script, 32)
	for i := range scripts {
		scripts[i] = mockScript("digest")
	}
	stubBuildProvider(t, mock.New(scripts...))
	return mgr, clk
}

func foldCount(t *testing.T, mgr *Manager, sessionID string) int {
	t.Helper()
	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("digestRepo.Get(%s): %v", sessionID, err)
	}
	if d == nil {
		return 0
	}
	return d.FoldCount
}

// TestManager_Debounce_BelowFloorDoesNotFold verifies a single trigger does
// not fold until the 40s debounce floor has elapsed.
func TestManager_Debounce_BelowFloorDoesNotFold(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionID, projectID := "sess-debounce-1", "proj-debounce-1"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID})
	settle(50 * time.Millisecond) // let the sidecar goroutine register its debounce timer
	clk.Advance(39 * time.Second)

	// Give the sidecar goroutine a chance to run if it were (incorrectly)
	// going to fold; then assert it did not.
	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count after 39s = %d, want 0 (below the 40s debounce floor)", got)
	}
}

// TestManager_SelfInflictedEventNeverTriggers verifies an event from the
// fold's own `_refinery-cli` child (its digest findings_add broadcast) never
// re-triggers a fold — the fold → findings.updated → fold feedback loop.
func TestManager_SelfInflictedEventNeverTriggers(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionID, projectID := "sess-selfloop-1", "proj-selfloop-1"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID,
		Data: map[string]interface{}{"agent_type": "_refinery-cli", "action": "add"}})
	settle(50 * time.Millisecond)
	clk.Advance(60 * time.Second)

	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count = %d, want 0 (fold child's own event must not re-trigger)", got)
	}
}

// TestManager_ConsoleFoldGate_ClosedAboveThreshold verifies a barely-used
// chat (context_left above refinery_console_fold_start_context_pct, default
// 75 — here NULL, which reads as 100) never folds, even on an immediate
// trigger.
func TestManager_ConsoleFoldGate_ClosedAboveThreshold(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-gate-1", "proj-gate-1"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	if _, err := mgr.pool.Exec(`UPDATE agent_sessions SET context_left = NULL WHERE id = ?`, sessionID); err != nil {
		t.Fatalf("clear context_left: %v", err)
	}
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventOrchestrationCompleted, ProjectID: projectID})

	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count = %d, want 0 (console fold gate closed above threshold)", got)
	}
}

// TestManager_Debounce_AtFloorFolds verifies crossing the 40s floor triggers
// exactly one fold.
func TestManager_Debounce_AtFloorFolds(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionID, projectID := "sess-debounce-2", "proj-debounce-2"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID})
	settle(50 * time.Millisecond) // let the sidecar goroutine register its debounce timer
	clk.Advance(40 * time.Second)

	waitForCondition(t, 2*time.Second, func() bool { return foldCount(t, mgr, sessionID) == 1 })
}

// TestManager_Debounce_CoalescesTriggersWithinWindow verifies several
// triggers arriving within the same debounce window produce exactly one
// fold, not one per trigger.
func TestManager_Debounce_CoalescesTriggersWithinWindow(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionID, projectID := "sess-debounce-3", "proj-debounce-3"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	for i := 0; i < 5; i++ {
		mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID})
	}
	settle(50 * time.Millisecond) // let the sidecar goroutine drain all 5 triggers
	clk.Advance(40 * time.Second)

	waitForCondition(t, 2*time.Second, func() bool { return foldCount(t, mgr, sessionID) >= 1 })
	// Give any (incorrect) extra folds a chance to land before asserting.
	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionID); got != 1 {
		t.Errorf("fold_count after 5 coalesced triggers = %d, want 1", got)
	}
}

// TestManager_ImmediateFoldOnOrchestrationCompleted verifies a completion
// event folds right away, without waiting out the debounce floor.
func TestManager_ImmediateFoldOnOrchestrationCompleted(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-immediate-1", "proj-immediate-1"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventOrchestrationCompleted, ProjectID: projectID})

	waitForCondition(t, 2*time.Second, func() bool { return foldCount(t, mgr, sessionID) == 1 })
}

// TestManager_ImmediateFoldOnOrchestrationFailed mirrors the completed case
// for the failed event type.
func TestManager_ImmediateFoldOnOrchestrationFailed(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-immediate-2", "proj-immediate-2"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventOrchestrationFailed, ProjectID: projectID})

	waitForCondition(t, 2*time.Second, func() bool { return foldCount(t, mgr, sessionID) == 1 })
}

// TestManager_NoFoldWithoutAnyTrigger verifies the sidecar never folds on its
// own — it is purely event-driven, never a polling timer. Advancing the
// clock far past the debounce floor with zero triggers delivered must never
// produce a fold.
func TestManager_NoFoldWithoutAnyTrigger(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionID, projectID := "sess-nopoll-1", "proj-nopoll-1"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	clk.Advance(10 * time.Minute)

	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count after 10 minutes with no OnEvent trigger = %d, want 0 (no polling)", got)
	}
}

// TestManager_OnEvent_IgnoresIrrelevantEventTypes verifies an event type the
// sidecar does not fold on (e.g. agent.started) never triggers a fold, even
// with a live session subscribed to the project.
func TestManager_OnEvent_IgnoresIrrelevantEventTypes(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionID, projectID := "sess-irrelevant-1", "proj-irrelevant-1"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventAgentStarted, ProjectID: projectID})
	clk.Advance(1 * time.Minute)

	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count after an irrelevant event type = %d, want 0", got)
	}
}

// TestManager_OnEvent_RoutesByProjectID verifies OnEvent only wakes sidecars
// for sessions scoped to the event's project, never a different project's
// session sharing no relationship to it.
func TestManager_OnEvent_RoutesByProjectID(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionA, projectA := "sess-route-a", "proj-route-a"
	sessionB, projectB := "sess-route-b", "proj-route-b"
	seedConsoleChatSession(t, mgr.pool, sessionA, projectA)
	seedConsoleChatSession(t, mgr.pool, sessionB, projectB)
	mgr.Start(sessionA, projectA)
	mgr.Start(sessionB, projectB)
	t.Cleanup(func() { mgr.Stop(sessionA) })
	t.Cleanup(func() { mgr.Stop(sessionB) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectA})
	settle(50 * time.Millisecond) // let the sidecar goroutine register its debounce timer
	clk.Advance(40 * time.Second)

	waitForCondition(t, 2*time.Second, func() bool { return foldCount(t, mgr, sessionA) == 1 })
	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionB); got != 0 {
		t.Errorf("sessionB (different project) fold_count = %d, want 0", got)
	}
}

// TestManager_Stop_IsIdempotentForUnknownSession verifies Stop on a session
// that was never Started (or already stopped) is a safe no-op.
func TestManager_Stop_IsIdempotentForUnknownSession(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Stop("no-such-session")
}

// TestManager_Start_IsIdempotentForLiveSession verifies a second Start for an
// already-live session id does not spin up a duplicate sidecar (which would
// otherwise double-fold on a single trigger).
func TestManager_Start_IsIdempotentForLiveSession(t *testing.T) {
	mgr, clk := newTestManager(t)
	sessionID, projectID := "sess-double-start", "proj-double-start"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventOrchestrationCompleted, ProjectID: projectID})
	waitForCondition(t, 2*time.Second, func() bool { return foldCount(t, mgr, sessionID) >= 1 })
	settle(200 * time.Millisecond)
	if got := foldCount(t, mgr, sessionID); got != 1 {
		t.Errorf("fold_count with a duplicate Start = %d, want 1 (idempotent)", got)
	}
	_ = clk
}
