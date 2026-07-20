package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/ws"
)

// TestProactiveRestartDecision_FiresAndRecordsRestart drives the shared
// entry point end to end against a real config pool: a session over
// threshold at an idle boundary fires, and NoteProactiveRestart is what the
// NEXT decision's min-interval gate sees.
func TestProactiveRestartDecision_FiresAndRecordsRestart(t *testing.T) {
	t.Parallel()
	pool, clk := newRestartConfigPool(t)
	sessionID := "sess-decision-fire-" + t.Name()
	t.Cleanup(func() { DropProactiveRestartState(sessionID) })

	fire, tokensBefore := ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 10, true, false)
	if !fire {
		t.Fatal("ProactiveRestartDecision() fire = false, want true (over threshold, idle, no prior restart)")
	}
	if tokensBefore != 300000 {
		t.Errorf("tokensBefore = %d, want 300000", tokensBefore)
	}

	NoteProactiveRestart(sessionID, clk)

	// Immediately re-deciding must be blocked by the min-interval default
	// (600s) since NoteProactiveRestart just stamped "now".
	fire, _ = ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 10, true, false)
	if fire {
		t.Error("ProactiveRestartDecision() immediately after NoteProactiveRestart = true, want false (min-interval not yet elapsed)")
	}

	clk.Advance(11 * time.Minute)
	fire, _ = ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 10, true, false)
	if !fire {
		t.Error("ProactiveRestartDecision() after min-interval elapsed = false, want true")
	}
}

// TestProactiveRestartDecision_MaxPerSessionConfig verifies the
// proactive_restart_max_per_session config knob caps subsequent restarts.
func TestProactiveRestartDecision_MaxPerSessionConfig(t *testing.T) {
	t.Parallel()
	pool, clk := newRestartConfigPool(t)
	if err := pool.SetConfig("proactive_restart_max_per_session", "1"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := pool.SetConfig("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	sessionID := "sess-decision-max-" + t.Name()
	t.Cleanup(func() { DropProactiveRestartState(sessionID) })

	fire, _ := ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 10, true, false)
	if !fire {
		t.Fatal("first ProactiveRestartDecision() = false, want true")
	}
	NoteProactiveRestart(sessionID, clk)

	fire, _ = ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 10, true, false)
	if fire {
		t.Error("second ProactiveRestartDecision() after max_per_session=1 exhausted = true, want false")
	}
}

// TestDropProactiveRestartState_ResetsBookkeeping verifies dropping a
// session's state clears its restart count, so a session ID reused after a
// relaunch (a fresh spawn under a brand-new session ID never collides in
// practice, but the store must not leak state across drop/reuse) is not
// permanently capped.
func TestDropProactiveRestartState_ResetsBookkeeping(t *testing.T) {
	t.Parallel()
	pool, clk := newRestartConfigPool(t)
	if err := pool.SetConfig("proactive_restart_max_per_session", "1"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := pool.SetConfig("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	sessionID := "sess-decision-drop-" + t.Name()

	NoteProactiveRestart(sessionID, clk)
	fire, _ := ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 10, true, false)
	if fire {
		t.Fatal("decision after 1 restart with max=1 = true, want false")
	}

	DropProactiveRestartState(sessionID)
	t.Cleanup(func() { DropProactiveRestartState(sessionID) })

	fire, _ = ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 10, true, false)
	if !fire {
		t.Error("decision after DropProactiveRestartState = false, want true (bookkeeping reset)")
	}
}

// TestProactiveRestartThresholdDefault_ReadsConfigWithFallback verifies the
// global config knob is read with the hardcoded fallback when unset.
func TestProactiveRestartThresholdDefault_ReadsConfigWithFallback(t *testing.T) {
	t.Parallel()
	pool, _ := newRestartConfigPool(t)

	if got := ProactiveRestartThresholdDefault(pool); got != defaultProactiveRestartThreshold {
		t.Errorf("ProactiveRestartThresholdDefault() unset = %d, want default %d", got, defaultProactiveRestartThreshold)
	}

	if err := pool.SetConfig("proactive_restart_threshold_default", "77000"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := ProactiveRestartThresholdDefault(pool); got != 77000 {
		t.Errorf("ProactiveRestartThresholdDefault() = %d, want 77000", got)
	}
}

// TestProactiveRestartCoordinator_StampsBoundaryOnFindingsUpdated verifies
// the ws.Listener taps findings.updated for a session with a tracked ledger,
// and that boundary turn feeds the boundary-window gate.
func TestProactiveRestartCoordinator_StampsBoundaryOnFindingsUpdated(t *testing.T) {
	t.Parallel()
	pool, clk := newRestartConfigPool(t)
	sessionID := "sess-coordinator-" + t.Name()
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})

	// Ledger at turn 5.
	l := globalLedgerStore.get(sessionID)
	for i := 0; i < 5; i++ {
		l.nextTurn()
	}

	coord := NewProactiveRestartCoordinator(clk)
	coord.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, SessionID: sessionID})

	// Boundary window default is 10 turns; advance the ledger 15 more turns
	// (current turn 20) so a decision at turn 20 exceeds the window relative
	// to the stamped boundary (turn 5) — must NOT fire.
	for i := 0; i < 15; i++ {
		l.nextTurn()
	}
	fire, _ := ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 20, true, false)
	if fire {
		t.Error("decision at turn 20 with boundary stamped at turn 5 (window=10) = true, want false (stale boundary)")
	}

	// Re-stamp at the current turn (20): now within the window.
	coord.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, SessionID: sessionID})
	fire, _ = ProactiveRestartDecision(pool, clk, sessionID, 300000, 250000, 20, true, false)
	if !fire {
		t.Error("decision immediately after a fresh boundary stamp = false, want true")
	}
}

// TestProactiveRestartCoordinator_IgnoresUnrelatedEventsAndMissingSession
// verifies OnEvent is a no-op for event types outside the watched set, for
// events with no session id, and for a session id with no tracked ledger.
func TestProactiveRestartCoordinator_IgnoresUnrelatedEventsAndMissingSession(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	coord := NewProactiveRestartCoordinator(clk)

	// Unrelated event type.
	otherSession := "sess-coordinator-unrelated-" + t.Name()
	t.Cleanup(func() { DropProactiveRestartState(otherSession) })
	coord.OnEvent(&ws.Event{Type: "agent.nudged", SessionID: otherSession})
	if st := globalRestartStore.snapshot(otherSession); !st.lastBoundaryAt.IsZero() {
		t.Error("OnEvent stamped a boundary for an unrelated event type")
	}

	// No session id on an otherwise-watched event type.
	coord.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, SessionID: ""})

	// findings.updated for a session with no tracked ledger — cannot stamp.
	untracked := "sess-coordinator-untracked-" + t.Name()
	t.Cleanup(func() { DropProactiveRestartState(untracked) })
	coord.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, SessionID: untracked})
	if st := globalRestartStore.snapshot(untracked); !st.lastBoundaryAt.IsZero() {
		t.Error("OnEvent stamped a boundary for a session with no tracked ledger")
	}
}

// TestProactiveRestartCoordinator_IgnoresPrunedBoundaryEventTypes documents
// the proactiveBoundaryEventTypes prune: orchestration/plan events that used
// to be members (kept "for parity/future-proofing") never carried a
// SessionID in practice, so dropping them from the set is a runtime no-op —
// verified here by driving OnEvent with a SessionID attached anyway and
// confirming no boundary gets stamped, plus a direct membership check on the
// map itself.
func TestProactiveRestartCoordinator_IgnoresPrunedBoundaryEventTypes(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	coord := NewProactiveRestartCoordinator(clk)

	pruned := []string{
		ws.EventOrchestrationCompleted,
		ws.EventOrchestrationFailed,
		ws.EventPlanMaterialized,
	}
	for _, evType := range pruned {
		if proactiveBoundaryEventTypes[evType] {
			t.Errorf("proactiveBoundaryEventTypes[%q] = true, want false (pruned)", evType)
		}
	}
	if !proactiveBoundaryEventTypes[ws.EventFindingsUpdated] {
		t.Errorf("proactiveBoundaryEventTypes[%q] = false, want true (only remaining member)", ws.EventFindingsUpdated)
	}
	if len(proactiveBoundaryEventTypes) != 1 {
		t.Errorf("len(proactiveBoundaryEventTypes) = %d, want 1", len(proactiveBoundaryEventTypes))
	}

	for _, evType := range pruned {
		sessionID := "sess-pruned-" + evType + "-" + t.Name()
		t.Cleanup(func() { DropProactiveRestartState(sessionID) })

		// Give it a tracked ledger too, so a false-positive membership check
		// would actually stamp a boundary rather than bailing earlier on the
		// "no tracked ledger" branch.
		l := globalLedgerStore.get(sessionID)
		l.nextTurn()
		t.Cleanup(func() { globalLedgerStore.drop(sessionID) })

		coord.OnEvent(&ws.Event{Type: evType, SessionID: sessionID})
		if st := globalRestartStore.snapshot(sessionID); !st.lastBoundaryAt.IsZero() {
			t.Errorf("OnEvent stamped a boundary for pruned event type %q", evType)
		}
	}
}

// newRestartConfigPool builds a real, per-test migrated DB pool (never
// migrated more than once per package — copies the shared template) so
// ProactiveRestartDecision's contextConfigInt reads exercise the real
// config table, not a nil-pool fallback.
func newRestartConfigPool(t *testing.T) (*db.Pool, *clock.TestClock) {
	t.Helper()
	dbPath := t.TempDir() + "/restart_config.db"
	copyTemplateDB(t, dbPath)
	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("OpenPathExisting: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return db.WrapAsPool(database), clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}
