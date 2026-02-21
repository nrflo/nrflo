package ws

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// TestE2ESubscribeReplayLiveEvents tests the complete flow:
// 1. Client subscribes with cursor after some events already logged
// 2. Client receives replay of missed events
// 3. Client receives new live events after replay
func TestE2ESubscribeReplayLiveEvents(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub(clock.Real())
	eventLog := repo.NewEventLogRepo(pool, clock.Real())
	hub.SetEventLog(eventLog)

	go hub.Run()
	defer hub.Stop()

	// Phase 1: Log some events before client connects
	for i := 1; i <= 3; i++ {
		payload, _ := json.Marshal(map[string]interface{}{"phase": i})
		eventLog.Append("proj-1", "ticket-1", EventAgentStarted, "feature", payload)
	}

	// Phase 2: Client connects and subscribes with cursor=1 (should replay 2, 3)
	client := newTestClient(hub, "test-1")
	hub.Register(client)
	hub.Subscribe(client, "proj-1", "ticket-1")

	// Trigger replay
	handleReplay(client, "proj-1", "ticket-1", 1, hub)

	// Collect replay events (should get seq 2, 3)
	replayEvents := collectEvents(client, 2, time.Second)
	if len(replayEvents) != 2 {
		t.Fatalf("expected 2 replay events, got %d", len(replayEvents))
	}
	if replayEvents[0].Sequence != 2 || replayEvents[1].Sequence != 3 {
		t.Fatalf("expected seq 2,3, got %d,%d", replayEvents[0].Sequence, replayEvents[1].Sequence)
	}

	// Phase 3: Broadcast new live events (should be delivered to subscribed client)
	for i := 4; i <= 6; i++ {
		event := NewEvent(EventAgentCompleted, "proj-1", "ticket-1", "feature", map[string]interface{}{
			"phase": i,
		})
		hub.Broadcast(event)
	}

	// Collect live events (should get seq 4, 5, 6)
	liveEvents := collectEvents(client, 3, time.Second)
	if len(liveEvents) != 3 {
		t.Fatalf("expected 3 live events, got %d", len(liveEvents))
	}
	for i := 0; i < 3; i++ {
		expected := int64(i + 4)
		if liveEvents[i].Sequence != expected {
			t.Fatalf("expected seq=%d at index %d, got %d", expected, i, liveEvents[i].Sequence)
		}
	}

	// Verify all events have protocol version
	allEvents := append(replayEvents, liveEvents...)
	for i, evt := range allEvents {
		if evt.ProtocolVersion != ProtocolVersion {
			t.Fatalf("expected protocol_version=%d at index %d, got %d", ProtocolVersion, i, evt.ProtocolVersion)
		}
	}
}

// TestE2EMultipleClientsReplay tests that multiple clients can independently replay
func TestE2EMultipleClientsReplay(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub(clock.Real())
	eventLog := repo.NewEventLogRepo(pool, clock.Real())
	hub.SetEventLog(eventLog)

	go hub.Run()
	defer hub.Stop()

	// Log 10 events
	for i := 1; i <= 10; i++ {
		payload, _ := json.Marshal(map[string]interface{}{"index": i})
		eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", payload)
	}

	// Client 1 subscribes with cursor=5 (should get 6-10)
	client1 := newTestClient(hub, "client-1")
	hub.Register(client1)
	hub.Subscribe(client1, "proj-1", "ticket-1")
	handleReplay(client1, "proj-1", "ticket-1", 5, hub)

	events1 := collectEvents(client1, 5, time.Second)
	if len(events1) != 5 {
		t.Fatalf("client1: expected 5 events, got %d", len(events1))
	}
	if events1[0].Sequence != 6 || events1[4].Sequence != 10 {
		t.Fatalf("client1: expected seq 6-10, got %d-%d", events1[0].Sequence, events1[4].Sequence)
	}

	// Client 2 subscribes with cursor=8 (should get 9-10)
	client2 := newTestClient(hub, "client-2")
	hub.Register(client2)
	hub.Subscribe(client2, "proj-1", "ticket-1")
	handleReplay(client2, "proj-1", "ticket-1", 8, hub)

	events2 := collectEvents(client2, 2, time.Second)
	if len(events2) != 2 {
		t.Fatalf("client2: expected 2 events, got %d", len(events2))
	}
	if events2[0].Sequence != 9 || events2[1].Sequence != 10 {
		t.Fatalf("client2: expected seq 9-10, got %d-%d", events2[0].Sequence, events2[1].Sequence)
	}

	// Client 3 subscribes caught up (should get no replay)
	client3 := newTestClient(hub, "client-3")
	hub.Register(client3)
	hub.Subscribe(client3, "proj-1", "ticket-1")
	handleReplay(client3, "proj-1", "ticket-1", 10, hub)

	// Should not receive any events
	select {
	case msg := <-client3.send:
		t.Fatalf("client3: expected no replay, got: %s", string(msg))
	case <-time.After(200 * time.Millisecond):
		// Expected - no events
	}

	// Broadcast new event - all three clients should receive it
	event := NewEvent(EventTestEcho, "proj-1", "ticket-1", "feature", map[string]interface{}{
		"index": 11,
	})
	hub.Broadcast(event)

	// All clients should receive seq=11
	for i, client := range []*Client{client1, client2, client3} {
		select {
		case msg := <-client.send:
			var evt Event
			if err := json.Unmarshal(msg, &evt); err != nil {
				t.Fatalf("client%d: failed to unmarshal: %v", i+1, err)
			}
			if evt.Sequence != 11 {
				t.Fatalf("client%d: expected seq=11, got %d", i+1, evt.Sequence)
			}
		case <-time.After(time.Second):
			t.Fatalf("client%d: timeout waiting for live event", i+1)
		}
	}
}

// TestE2ESnapshotToLiveTransition tests snapshot followed by live events
func TestE2ESnapshotToLiveTransition(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub(clock.Real())
	eventLog := repo.NewEventLogRepo(pool, clock.Real())
	hub.SetEventLog(eventLog)

	snapshotProvider := &mockSnapshotProvider{
		chunks: []SnapshotChunk{
			{Entity: EntityWorkflowState, Data: map[string]interface{}{"status": "active"}},
		},
	}
	hub.SetSnapshotProvider(snapshotProvider)

	go hub.Run()
	defer hub.Stop()

	// Client subscribes with cursor=0 when no events exist (triggers snapshot)
	client := newTestClient(hub, "test-1")
	hub.Register(client)
	hub.Subscribe(client, "proj-1", "ticket-1")

	handleReplay(client, "proj-1", "ticket-1", 0, hub)

	// Should receive snapshot sequence
	snapshotEvents := collectEvents(client, 3, time.Second)
	if len(snapshotEvents) != 3 {
		t.Fatalf("expected 3 snapshot events, got %d", len(snapshotEvents))
	}
	if snapshotEvents[0].Type != EventSnapshotBegin {
		t.Fatalf("expected snapshot.begin, got %s", snapshotEvents[0].Type)
	}
	if snapshotEvents[2].Type != EventSnapshotEnd {
		t.Fatalf("expected snapshot.end, got %s", snapshotEvents[2].Type)
	}

	// Verify current_seq in snapshot.end is 0 (no events yet)
	currentSeq := snapshotEvents[2].Data["current_seq"].(float64)
	if currentSeq != 0 {
		t.Fatalf("expected current_seq=0 in snapshot.end, got %v", currentSeq)
	}

	// Now broadcast live events
	for i := 1; i <= 3; i++ {
		event := NewEvent(EventTestEcho, "proj-1", "ticket-1", "feature", nil)
		hub.Broadcast(event)
	}

	// Should receive all 3 live events with seq=1,2,3
	liveEvents := collectEvents(client, 3, time.Second)
	if len(liveEvents) != 3 {
		t.Fatalf("expected 3 live events, got %d", len(liveEvents))
	}
	for i := 0; i < 3; i++ {
		expected := int64(i + 1)
		if liveEvents[i].Sequence != expected {
			t.Fatalf("expected seq=%d at index %d, got %d", expected, i, liveEvents[i].Sequence)
		}
	}
}

// TestE2ECleanupDoesNotAffectActiveCursors tests that cleanup preserves recent events
func TestE2ECleanupDoesNotAffectActiveCursors(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub(clock.Real())
	eventLog := repo.NewEventLogRepo(pool, clock.Real())
	hub.SetEventLog(eventLog)

	go hub.Run()
	defer hub.Stop()

	// Log recent events
	for i := 1; i <= 10; i++ {
		eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", nil)
	}

	// Run cleanup with very long retention (should delete nothing)
	deleted, err := eventLog.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted (all events recent), got %d", deleted)
	}

	// Client can still replay
	client := newTestClient(hub, "test-1")
	hub.Register(client)
	hub.Subscribe(client, "proj-1", "ticket-1")
	handleReplay(client, "proj-1", "ticket-1", 5, hub)

	events := collectEvents(client, 5, time.Second)
	if len(events) != 5 {
		t.Fatalf("expected 5 events after cleanup, got %d", len(events))
	}
}

// TestE2EProjectWideSubscription tests project-wide subscriptions receive all ticket events
func TestE2EProjectWideSubscription(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub(clock.Real())
	eventLog := repo.NewEventLogRepo(pool, clock.Real())
	hub.SetEventLog(eventLog)

	go hub.Run()
	defer hub.Stop()

	// Client subscribes project-wide (empty ticketID)
	client := newTestClient(hub, "test-1")
	hub.Register(client)
	hub.Subscribe(client, "proj-1", "")

	// Broadcast events to different tickets
	hub.Broadcast(NewEvent(EventTestEcho, "proj-1", "ticket-1", "feature", map[string]interface{}{"id": 1}))
	hub.Broadcast(NewEvent(EventTestEcho, "proj-1", "ticket-2", "feature", map[string]interface{}{"id": 2}))
	hub.Broadcast(NewEvent(EventTestEcho, "proj-1", "ticket-3", "feature", map[string]interface{}{"id": 3}))

	// Should receive all three events
	events := collectEvents(client, 3, time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events for project-wide subscription, got %d", len(events))
	}

	// Verify different ticket IDs
	tickets := []string{events[0].TicketID, events[1].TicketID, events[2].TicketID}
	expected := []string{"ticket-1", "ticket-2", "ticket-3"}
	for i := 0; i < 3; i++ {
		if tickets[i] != expected[i] {
			t.Fatalf("expected ticket_id=%s at index %d, got %s", expected[i], i, tickets[i])
		}
	}
}
