package refinery

import (
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// capturingBroadcaster records every ws.Event handed to it, for tests
// asserting the autonomous fold path's SetBroadcaster wiring.
type capturingBroadcaster struct {
	mu     sync.Mutex
	events []*ws.Event
}

func (b *capturingBroadcaster) capture(ev *ws.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *capturingBroadcaster) snapshot() []*ws.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*ws.Event, len(b.events))
	copy(out, b.events)
	return out
}

func TestFoldAutonomous_BroadcastsHandoffDigestAfterUpsert(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-auto-bcast", "proj-auto-bcast"
	wfiID, nodeID := "wfi-bcast", "node-bcast"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello", "world")

	mgr := NewManager(pool, clk)
	bc := &capturingBroadcaster{}
	mgr.SetBroadcaster(bc.capture)
	stubBuildProvider(t, mock.New(mockScript("slot digest for broadcast")))

	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	// Wait on the broadcast itself, not just the DB slot: broadcastHandoffDigest
	// runs in the same fold goroutine right after UpsertSlot commits, so
	// polling only the DB row races the broadcaster call under load.
	waitForCondition(t, 2*time.Second, func() bool {
		return len(bc.snapshot()) > 0
	})

	events := bc.snapshot()
	var handoff []*ws.Event
	for _, ev := range events {
		if ev.Type == ws.EventAgentHandoffDigest {
			handoff = append(handoff, ev)
		}
	}
	if len(handoff) != 1 {
		t.Fatalf("got %d agent.handoff_digest events, want exactly 1 (events=%+v)", len(handoff), events)
	}

	ev := handoff[0]
	if ev.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", ev.ProjectID, projectID)
	}
	data := ev.Data
	if data["session_id"] != sessionID {
		t.Errorf("data[session_id] = %v, want %q", data["session_id"], sessionID)
	}
	if fc, ok := data["fold_count"].(int); !ok || fc != 1 {
		t.Errorf("data[fold_count] = %v, want 1", data["fold_count"])
	}
	content, ok := data["content"].(string)
	if !ok || content == "" {
		t.Errorf("data[content] = %v, want non-empty string", data["content"])
	}
}

// TestFoldAutonomous_NilBroadcaster_FoldsWithoutPanic guards the nil-safe
// default path: a Manager that never calls SetBroadcaster (the default in
// every other autonomous-sidecar test in this package) must still complete
// a fold, not panic on the nil broadcaster call.
func TestFoldAutonomous_NilBroadcaster_FoldsWithoutPanic(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-auto-nobcast", "proj-auto-nobcast"
	wfiID, nodeID := "wfi-nobcast", "node-nobcast"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("slot digest no broadcaster")))

	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
}
