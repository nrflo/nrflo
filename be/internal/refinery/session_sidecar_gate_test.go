package refinery

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// intPtr is a small helper for setContextLeft's *int parameter.
func intPtr(v int) *int { return &v }

// TestFoldGate_AboveThreshold_NoFold covers the default-threshold skip
// case: context_left=80 is well above threshold, so a trigger must not fold.
func TestFoldGate_AboveThreshold_NoFold(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-above", "proj-gate-above"
	wfiID, nodeID := "wfi-gate-above", "node-gate-above"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(80))
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)

	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot with context_left=80 (above the default threshold) = %+v, want nil (gate closed)", s)
	}
}

// TestFoldGate_AtOrBelowThreshold_Folds covers the fold-happens case:
// context_left=30 is at/below the default threshold.
func TestFoldGate_AtOrBelowThreshold_Folds(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-below", "proj-gate-below"
	wfiID, nodeID := "wfi-gate-below", "node-gate-below"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(30))
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("digest")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
}

// TestFoldGate_NullContextLeft_ReadsAs100_NoFold proves the COALESCE-to-100
// read: a NULL context_left (fresh/unreported session) must not fold under
// the default threshold.
func TestFoldGate_NullContextLeft_ReadsAs100_NoFold(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-null", "proj-gate-null"
	wfiID, nodeID := "wfi-gate-null", "node-gate-null"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, nil)
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)

	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot with NULL context_left = %+v, want nil (reads as 100, above default threshold)", s)
	}
}

// TestFoldGate_ThresholdSetTo100_AlwaysFolds proves today's always-fold
// behaviour is still reachable by setting the threshold to 100: a NULL (->100)
// or exactly-100 context_left then satisfies left<=threshold.
func TestFoldGate_ThresholdSetTo100_AlwaysFolds(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-100", "proj-gate-100"
	wfiID, nodeID := "wfi-gate-100", "node-gate-100"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, nil)
	seedMessages(t, pool, clk, sessionID, "hello")

	if err := service.NewGlobalSettingsService(pool, clk).Set(service.RefineryFoldStartContextPctKey, "100"); err != nil {
		t.Fatalf("set %s=100: %v", service.RefineryFoldStartContextPctKey, err)
	}

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("digest")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
}

// TestFoldGate_ThresholdSetTo0_NeverFolds verifies threshold=0 only lets a
// literal context_left=0 through; 30 must not fold.
func TestFoldGate_ThresholdSetTo0_NeverFolds(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gate-zero", "proj-gate-zero"
	wfiID, nodeID := "wfi-gate-zero", "node-gate-zero"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setContextLeft(t, pool, sessionID, intPtr(30))
	seedMessages(t, pool, clk, sessionID, "hello")

	if err := service.NewGlobalSettingsService(pool, clk).Set(service.RefineryFoldStartContextPctKey, "0"); err != nil {
		t.Fatalf("set %s=0: %v", service.RefineryFoldStartContextPctKey, err)
	}

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)

	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot with threshold=0, context_left=30 = %+v, want nil", s)
	}
}

// TestFoldGate_GarbageThreshold_FallsBackToDefault verifies an out-of-range
// or unparseable stored threshold value falls back to the in-code default,
// reachable via the same above/below-threshold behaviour as the default case.
func TestFoldGate_GarbageThreshold_FallsBackToDefault(t *testing.T) {
	cases := []struct {
		name        string
		stored      string
		contextLeft int
		wantFold    bool
	}{
		{"non_numeric_above", "abc", 80, false},
		{"non_numeric_below", "abc", 30, true},
		{"negative_above", "-5", 80, false},
		{"negative_below", "-5", 30, true},
		{"over_max_above", "500", 80, false},
		{"over_max_below", "500", 30, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestPool(t)
			clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			sessionID, projectID := "sess-gate-garbage-"+tc.name, "proj-gate-garbage-"+tc.name
			wfiID, nodeID := "wfi-"+tc.name, "node-"+tc.name
			seedAutonomousSession(t, pool, sessionID, projectID)
			setContextLeft(t, pool, sessionID, intPtr(tc.contextLeft))
			seedMessages(t, pool, clk, sessionID, "hello")

			if err := service.NewGlobalSettingsService(pool, clk).Set(service.RefineryFoldStartContextPctKey, tc.stored); err != nil {
				t.Fatalf("set %s=%s: %v", service.RefineryFoldStartContextPctKey, tc.stored, err)
			}

			mgr := NewManager(pool, clk)
			stubBuildProvider(t, mock.New(mockScript("digest")))
			mgr.StartSession(sessionID, projectID, wfiID, nodeID)
			t.Cleanup(func() { mgr.StopSession(sessionID) })

			mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

			if tc.wantFold {
				waitForCondition(t, 2*time.Second, func() bool {
					s := getSlot(t, mgr, wfiID, nodeID)
					return s != nil && s.FoldCount == 1
				})
				return
			}
			settle(200 * time.Millisecond)
			if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
				t.Errorf("stored=%q context_left=%d: GetSlot = %+v, want nil", tc.stored, tc.contextLeft, s)
			}
		})
	}
}
