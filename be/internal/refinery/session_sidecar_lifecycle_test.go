package refinery

import (
	"errors"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/types"
	"be/internal/ws"
)

func TestStopSession_NoLeakAndIdempotent(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-stop-1", "proj-stop-1"
	wfiID, nodeID := "wfi-stop", "node-stop"
	seedAutonomousSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)

	mgr.autonomousMu.Lock()
	_, registered := mgr.autonomous[sessionID]
	mgr.autonomousMu.Unlock()
	if !registered {
		t.Fatal("autonomous map has no entry right after StartSession")
	}

	mgr.StopSession(sessionID)

	mgr.autonomousMu.Lock()
	_, stillRegistered := mgr.autonomous[sessionID]
	mgr.autonomousMu.Unlock()
	if stillRegistered {
		t.Error("autonomous map still has an entry after StopSession")
	}

	mgr.slotsMu.Lock()
	slotCount := len(mgr.slots)
	mgr.slotsMu.Unlock()
	if slotCount != 0 {
		t.Errorf("len(mgr.slots) after StopSession = %d, want 0 (refcount released to zero)", slotCount)
	}

	// Second StopSession for the same (now-unknown) id, and StopSession for
	// a completely unknown id, must both be safe no-ops.
	mgr.StopSession(sessionID)
	mgr.StopSession("never-started")
}

func TestStartSession_GatedOff_RegistersNoSidecar(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-gated-1", "proj-gated-1"
	wfiID, nodeID := "wfi-gated", "node-gated"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello")

	if err := service.NewGlobalSettingsService(pool, clk).Set("refinery_autonomous_enabled", "false"); err != nil {
		t.Fatalf("set refinery_autonomous_enabled=false: %v", err)
	}

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.autonomousMu.Lock()
	_, registered := mgr.autonomous[sessionID]
	mgr.autonomousMu.Unlock()
	if registered {
		t.Error("StartSession registered a sidecar while refinery_autonomous_enabled=false")
	}

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)
	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot after a trigger while gated off = %+v, want nil", s)
	}
}

func TestFoldAutonomous_CostAttributedOncePerFold(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-cost-1", "proj-cost-1"
	wfiID, nodeID := "wfi-cost", "node-cost"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest")
	stubBuildProvider(t, prov)

	type call struct {
		sessionID                         string
		in, out, cacheRead, cacheCreation int
	}
	var mu sync.Mutex
	var calls []call
	mgr.SetCostAttributor(func(sid string, in, out, cacheRead, cacheWrite int) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, call{sid, in, out, cacheRead, cacheWrite})
	})

	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
	settle(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("cost attributor calls = %d, want 1 (got %+v)", len(calls), calls)
	}
	got := calls[0]
	if got.sessionID != sessionID {
		t.Errorf("attributed sessionID = %q, want %q", got.sessionID, sessionID)
	}
	if got.in != 11 || got.out != 22 || got.cacheRead != 3 || got.cacheCreation != 4 {
		t.Errorf("attributed usage = %+v, want {in:11 out:22 cacheRead:3 cacheCreation:4}", got)
	}
}

// TestFoldAutonomous_CLILandingSkipsCostAttribution verifies a fold that
// lands on a cli_interactive chain entry (the api entry fails at
// buildProvider, the fake CLIFolder lands) does NOT call costAttributor —
// the one-off `_refinery-cli` child owns its own agent_sessions cost row, so
// attributing here too would double-charge.
func TestFoldAutonomous_CLILandingSkipsCostAttribution(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-cli-cost", "proj-cli-cost"
	wfiID, nodeID := "wfi-cli-cost", "node-cli-cost"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	stubBuildProviderErr(t, errors.New("no anthropic API key"))
	mgr.SetCLIFolder(&fakeCLIFolder{result: types.RefineryFoldResult{Content: "cli landed digest", InputTokens: 7, OutputTokens: 9}})

	var mu sync.Mutex
	called := false
	mgr.SetCostAttributor(func(sid string, in, out, cacheRead, cacheWrite int) {
		mu.Lock()
		defer mu.Unlock()
		called = true
	})

	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.Content == "cli landed digest"
	})
	settle(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Error("costAttributor was called on a cli_interactive landing, want skipped")
	}
}

func TestFoldAutonomous_MissingRefineryDef_NoOpsWithoutPanicking(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-nodef-1", "proj-nodef-1"
	wfiID, nodeID := "wfi-nodef", "node-nodef"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello")
	if _, err := pool.Exec(`DELETE FROM system_agent_definitions WHERE id = '_refinery'`); err != nil {
		t.Fatalf("delete _refinery def: %v", err)
	}

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)

	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot with missing _refinery def = %+v, want nil (fold should have skipped)", s)
	}
}

func TestFoldAutonomous_EmptyDelta_NoOpsWithoutPanicking(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-emptydelta-1", "proj-emptydelta-1"
	wfiID, nodeID := "wfi-emptydelta", "node-emptydelta"
	seedAutonomousSession(t, pool, sessionID, projectID)
	// No messages seeded at all.

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	settle(200 * time.Millisecond)

	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("GetSlot with an empty message delta = %+v, want nil (fold should have no-oped)", s)
	}
}
