package refinery

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/spawner/apirun/provider/mock"
)

// TestSlotLock_StartStopSession_LeavesNoEntry drives the refcounted registry
// through the real StartSession/StopSession lifecycle: a single autonomous
// session on one slot must leave mgr.slots empty once stopped.
func TestSlotLock_StartStopSession_LeavesNoEntry(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-slot-1", "proj-slot-1"
	wfiID, nodeID := "wfi-slot-1", "node-slot-1"
	seedAutonomousSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)

	mgr.slotsMu.Lock()
	if len(mgr.slots) != 1 {
		t.Fatalf("len(mgr.slots) right after StartSession = %d, want 1", len(mgr.slots))
	}
	mgr.slotsMu.Unlock()

	mgr.StopSession(sessionID)

	mgr.slotsMu.Lock()
	got := len(mgr.slots)
	mgr.slotsMu.Unlock()
	if got != 0 {
		t.Errorf("len(mgr.slots) after StopSession = %d, want 0", got)
	}
}

// TestSlotLock_TwoSessionsSameSlot_ShareMutexAndRefcount verifies two
// autonomous sessions on the same (wfi, node) slot share one *sync.Mutex
// identity, and stopping the first keeps the entry alive (refs==1) while
// stopping the second empties it.
func TestSlotLock_TwoSessionsSameSlot_ShareMutexAndRefcount(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sid1, sid2, projectID := "sess-slot-2a", "sess-slot-2b", "proj-slot-2"
	wfiID, nodeID := "wfi-slot-2", "node-slot-2"
	seedAutonomousSession(t, pool, sid1, projectID)
	seedAutonomousSession(t, pool, sid2, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))

	mu1 := mgr.acquireSlotLock(wfiID, nodeID)
	mu2 := mgr.acquireSlotLock(wfiID, nodeID)
	if mu1 != mu2 {
		t.Fatal("acquireSlotLock returned different *sync.Mutex identities for the same (wfi, node) slot")
	}

	key := slotKey(wfiID, nodeID)
	mgr.slotsMu.Lock()
	l, ok := mgr.slots[key]
	if !ok {
		t.Fatal("mgr.slots has no entry after two acquires")
	}
	if l.refs != 2 {
		t.Errorf("refs after two acquires = %d, want 2", l.refs)
	}
	mgr.slotsMu.Unlock()

	mgr.releaseSlotLock(wfiID, nodeID)
	mgr.slotsMu.Lock()
	l, ok = mgr.slots[key]
	if !ok {
		t.Fatal("mgr.slots entry dropped after only one release (refs should still be 1)")
	}
	if l.refs != 1 {
		t.Errorf("refs after one release of two = %d, want 1", l.refs)
	}
	mgr.slotsMu.Unlock()

	mgr.releaseSlotLock(wfiID, nodeID)
	mgr.slotsMu.Lock()
	_, ok = mgr.slots[key]
	mgr.slotsMu.Unlock()
	if ok {
		t.Error("mgr.slots still has an entry after both releases")
	}
	_ = sid1
	_ = sid2
}

// TestSlotLock_GatedOffStartSession_AcquiresNothing verifies a StartSession
// call that no-ops on the refinery_autonomous_enabled gate never touches the
// slot registry, so its paired StopSession is a true no-op — no negative
// refcount, no leaked entry.
func TestSlotLock_GatedOffStartSession_AcquiresNothing(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-slot-gated", "proj-slot-gated"
	wfiID, nodeID := "wfi-slot-gated", "node-slot-gated"
	seedAutonomousSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))
	if err := service.NewGlobalSettingsService(pool, clk).Set("refinery_autonomous_enabled", "false"); err != nil {
		t.Fatalf("set refinery_autonomous_enabled=false: %v", err)
	}

	mgr.StartSession(sessionID, projectID, wfiID, nodeID)

	mgr.slotsMu.Lock()
	got := len(mgr.slots)
	mgr.slotsMu.Unlock()
	if got != 0 {
		t.Fatalf("len(mgr.slots) after gated-off StartSession = %d, want 0", got)
	}

	mgr.StopSession(sessionID)

	mgr.slotsMu.Lock()
	got = len(mgr.slots)
	mgr.slotsMu.Unlock()
	if got != 0 {
		t.Errorf("len(mgr.slots) after StopSession following a gated-off StartSession = %d, want 0 (no negative refcount)", got)
	}
}
