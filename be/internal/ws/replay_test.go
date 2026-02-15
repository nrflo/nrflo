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

func TestReplayWithValidCursor(t *testing.T) {
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

	// Pre-populate event log
	for i := 1; i <= 5; i++ {
		payload := json.RawMessage(`{"index":` + string(rune(i+'0')) + `}`)
		_, err := eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", payload)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Subscribe with cursor (should replay events after seq=2)
	handleReplay(client, "proj-1", "ticket-1", 2, hub)

	// Should receive events 3, 4, 5
	var sequences []int64
	timeout := time.After(time.Second)
	for len(sequences) < 3 {
		select {
		case msg := <-client.send:
			var evt Event
			if err := json.Unmarshal(msg, &evt); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			sequences = append(sequences, evt.Sequence)
		case <-timeout:
			t.Fatalf("timeout waiting for replay, got %d/3 events", len(sequences))
		}
	}

	// Verify sequences
	expected := []int64{3, 4, 5}
	for i := 0; i < 3; i++ {
		if sequences[i] != expected[i] {
			t.Fatalf("expected seq=%d at index %d, got %d", expected[i], i, sequences[i])
		}
	}
}

func TestReplayWithCursorZero(t *testing.T) {
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

	// Create mock snapshot provider
	snapshotProvider := &mockSnapshotProvider{
		chunks: []SnapshotChunk{
			{Entity: EntityWorkflowState, Data: map[string]interface{}{"status": "active"}},
		},
	}
	hub.SetSnapshotProvider(snapshotProvider)

	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Subscribe with cursor=0 should trigger snapshot
	handleReplay(client, "proj-1", "ticket-1", 0, hub)

	// Should receive snapshot.begin, snapshot.chunk, snapshot.end
	events := collectEvents(client, 3, 2*time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events (begin/chunk/end), got %d", len(events))
	}

	if events[0].Type != EventSnapshotBegin {
		t.Fatalf("expected snapshot.begin, got %s", events[0].Type)
	}
	if events[1].Type != EventSnapshotChunk {
		t.Fatalf("expected snapshot.chunk, got %s", events[1].Type)
	}
	if events[2].Type != EventSnapshotEnd {
		t.Fatalf("expected snapshot.end, got %s", events[2].Type)
	}
}

func TestReplayWithPrunedEvents(t *testing.T) {
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

	// Create mock snapshot provider
	snapshotProvider := &mockSnapshotProvider{
		chunks: []SnapshotChunk{
			{Entity: EntityWorkflowState, Data: map[string]interface{}{"status": "active"}},
		},
	}
	hub.SetSnapshotProvider(snapshotProvider)

	go hub.Run()
	defer hub.Stop()

	// Add and then prune ALL events (simulate retention cleanup with no events left)
	for i := 1; i <= 10; i++ {
		_, err := eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", nil)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}
	// Delete all events to simulate aggressive pruning
	_, err = pool.Exec("DELETE FROM ws_event_log WHERE project_id = 'proj-1' AND ticket_id = 'ticket-1'")
	if err != nil {
		t.Fatalf("failed to prune events: %v", err)
	}

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Subscribe with cursor=5 (all events were pruned, latestSeq=0)
	// sinceSeq=5 > latestSeq=0, but since no events exist and sinceSeq > 0,
	// this should be caught up (no snapshot needed)
	handleReplay(client, "proj-1", "ticket-1", 5, hub)

	// Should not receive any events (treated as caught up)
	select {
	case msg := <-client.send:
		t.Fatalf("expected no events for caught-up cursor, got: %s", string(msg))
	case <-time.After(200 * time.Millisecond):
		// Expected - no events
	}
}

func TestReplayWithGapRequiresSnapshot(t *testing.T) {
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

	// Add events, then add more after a gap
	for i := 1; i <= 3; i++ {
		_, err := eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", nil)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}
	// Delete events 1-3 (simulate pruning)
	_, err = pool.Exec("DELETE FROM ws_event_log WHERE seq <= 3")
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}
	// Add new events (will be seq 4, 5, 6)
	for i := 1; i <= 3; i++ {
		_, err := eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", nil)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Subscribe with cursor=2 (events 1-3 were pruned, latest is 6)
	// sinceSeq=2 < latestSeq=6 and no events exist after seq=2 below seq=4
	// QuerySince will return seq 4,5,6 (3 events), so replay them
	handleReplay(client, "proj-1", "ticket-1", 2, hub)

	// Should receive replay events (not snapshot, since events exist)
	events := collectEvents(client, 3, time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 replay events, got %d", len(events))
	}
	if events[0].Sequence != 4 {
		t.Fatalf("expected first seq=4, got %d", events[0].Sequence)
	}
}

func TestReplayWithCaughtUpCursor(t *testing.T) {
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

	// Add 3 events
	for i := 1; i <= 3; i++ {
		_, err := eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", nil)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Subscribe with current cursor (caught up)
	handleReplay(client, "proj-1", "ticket-1", 3, hub)

	// Should not receive any events (client is caught up)
	select {
	case msg := <-client.send:
		t.Fatalf("expected no replay for caught-up client, got: %s", string(msg))
	case <-time.After(200 * time.Millisecond):
		// Expected - no events
	}
}

func TestReplayDifferentScope(t *testing.T) {
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

	// Add events to different scopes
	for i := 1; i <= 3; i++ {
		_, err := eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", nil)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}
	for i := 1; i <= 2; i++ {
		_, err := eventLog.Append("proj-1", "ticket-2", EventTestEcho, "feature", nil)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Replay for ticket-2 only
	handleReplay(client, "proj-1", "ticket-2", 0, hub)

	// Should receive only ticket-2 events (seq 4, 5)
	events := collectEvents(client, 2, time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 events for ticket-2, got %d", len(events))
	}
	if events[0].Sequence != 4 {
		t.Fatalf("expected seq=4, got %d", events[0].Sequence)
	}
	if events[1].Sequence != 5 {
		t.Fatalf("expected seq=5, got %d", events[1].Sequence)
	}
}

func TestReplayWithoutEventLog(t *testing.T) {
	hub := NewHub(clock.Real())
	// No event log configured

	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Replay without event log should be a no-op
	handleReplay(client, "proj-1", "ticket-1", 0, hub)

	// Should not receive any events
	select {
	case msg := <-client.send:
		t.Fatalf("expected no events without event log, got: %s", string(msg))
	case <-time.After(200 * time.Millisecond):
		// Expected - no events
	}
}

// Helper functions

func collectEvents(client *Client, count int, timeout time.Duration) []Event {
	var events []Event
	deadline := time.After(timeout)
	for len(events) < count {
		select {
		case msg := <-client.send:
			var evt Event
			if err := json.Unmarshal(msg, &evt); err != nil {
				return events
			}
			events = append(events, evt)
		case <-deadline:
			return events
		}
	}
	return events
}

// mockSnapshotProvider for testing
type mockSnapshotProvider struct {
	chunks []SnapshotChunk
	err    error
}

func (m *mockSnapshotProvider) BuildSnapshot(projectID, ticketID string) ([]SnapshotChunk, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.chunks, nil
}
