package ws

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/repo"
)

func TestSnapshotStreamingSuccess(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub()
	eventLog := repo.NewEventLogRepo(pool)
	hub.SetEventLog(eventLog)

	// Create snapshot provider with multiple chunks
	snapshotProvider := &mockSnapshotProvider{
		chunks: []SnapshotChunk{
			{Entity: EntityWorkflowState, Data: map[string]interface{}{"status": "active"}},
			{Entity: EntityAgentSessions, Data: map[string]interface{}{"sessions": []string{"s1", "s2"}}},
			{Entity: EntityFindings, Data: map[string]interface{}{"count": 5}},
		},
	}
	hub.SetSnapshotProvider(snapshotProvider)

	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Stream snapshot
	streamSnapshot(client, "proj-1", "ticket-1", hub)

	// Should receive: snapshot.begin, 3x snapshot.chunk, snapshot.end
	events := collectEvents(client, 5, 2*time.Second)
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Verify snapshot.begin
	if events[0].Type != EventSnapshotBegin {
		t.Fatalf("expected snapshot.begin, got %s", events[0].Type)
	}
	chunkCount := events[0].Data["chunk_count"]
	if chunkCount != float64(3) { // JSON numbers are float64
		t.Fatalf("expected chunk_count=3, got %v", chunkCount)
	}

	// Verify snapshot.chunk events
	expectedEntities := []string{EntityWorkflowState, EntityAgentSessions, EntityFindings}
	for i := 1; i <= 3; i++ {
		if events[i].Type != EventSnapshotChunk {
			t.Fatalf("expected snapshot.chunk at index %d, got %s", i, events[i].Type)
		}
		if events[i].Entity != expectedEntities[i-1] {
			t.Fatalf("expected entity %s at index %d, got %s", expectedEntities[i-1], i, events[i].Entity)
		}
	}

	// Verify snapshot.end
	if events[4].Type != EventSnapshotEnd {
		t.Fatalf("expected snapshot.end, got %s", events[4].Type)
	}
	currentSeq := events[4].Data["current_seq"]
	if currentSeq == nil {
		t.Fatal("expected current_seq in snapshot.end")
	}
}

func TestSnapshotWithCurrentSeq(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub()
	eventLog := repo.NewEventLogRepo(pool)
	hub.SetEventLog(eventLog)

	// Pre-populate event log
	for i := 1; i <= 10; i++ {
		_, err := eventLog.Append("proj-1", "ticket-1", EventTestEcho, "feature", nil)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

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

	// Stream snapshot
	streamSnapshot(client, "proj-1", "ticket-1", hub)

	// Collect snapshot events
	events := collectEvents(client, 3, 2*time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Check current_seq in snapshot.end
	if events[2].Type != EventSnapshotEnd {
		t.Fatalf("expected snapshot.end, got %s", events[2].Type)
	}
	currentSeq := events[2].Data["current_seq"].(float64) // JSON numbers are float64
	if currentSeq != 10 {
		t.Fatalf("expected current_seq=10, got %v", currentSeq)
	}
}

func TestSnapshotProviderError(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub()
	eventLog := repo.NewEventLogRepo(pool)
	hub.SetEventLog(eventLog)

	// Snapshot provider that returns error
	snapshotProvider := &mockSnapshotProvider{
		err: errors.New("snapshot build failed"),
	}
	hub.SetSnapshotProvider(snapshotProvider)

	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Stream snapshot (should send resync.required on error)
	streamSnapshot(client, "proj-1", "ticket-1", hub)

	select {
	case msg := <-client.send:
		var evt Event
		if err := json.Unmarshal(msg, &evt); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if evt.Type != EventResyncRequired {
			t.Fatalf("expected resync.required on error, got %s", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for resync.required")
	}
}

func TestSnapshotWithoutProvider(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	hub := NewHub()
	eventLog := repo.NewEventLogRepo(pool)
	hub.SetEventLog(eventLog)
	// No snapshot provider

	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Stream snapshot without provider should send resync.required
	streamSnapshot(client, "proj-1", "ticket-1", hub)

	select {
	case msg := <-client.send:
		var evt Event
		if err := json.Unmarshal(msg, &evt); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if evt.Type != EventResyncRequired {
			t.Fatalf("expected resync.required without provider, got %s", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for resync.required")
	}
}

func TestSnapshotEmptyChunks(t *testing.T) {
	hub := NewHub()

	// Snapshot provider with no chunks
	snapshotProvider := &mockSnapshotProvider{
		chunks: []SnapshotChunk{},
	}
	hub.SetSnapshotProvider(snapshotProvider)

	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	// Stream snapshot with empty chunks
	streamSnapshot(client, "proj-1", "ticket-1", hub)

	// Should still receive snapshot.begin and snapshot.end
	events := collectEvents(client, 2, time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].Type != EventSnapshotBegin {
		t.Fatalf("expected snapshot.begin, got %s", events[0].Type)
	}
	if events[1].Type != EventSnapshotEnd {
		t.Fatalf("expected snapshot.end, got %s", events[1].Type)
	}
}

func TestSnapshotChunkContent(t *testing.T) {
	hub := NewHub()

	// Snapshot provider with detailed chunk data
	workflowData := map[string]interface{}{
		"status":        "active",
		"current_phase": "implementor",
		"version":       4,
	}
	snapshotProvider := &mockSnapshotProvider{
		chunks: []SnapshotChunk{
			{Entity: EntityWorkflowState, Data: workflowData},
		},
	}
	hub.SetSnapshotProvider(snapshotProvider)

	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	streamSnapshot(client, "proj-1", "ticket-1", hub)

	events := collectEvents(client, 3, time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify chunk data is preserved
	chunkEvent := events[1]
	if chunkEvent.Type != EventSnapshotChunk {
		t.Fatalf("expected snapshot.chunk, got %s", chunkEvent.Type)
	}
	if chunkEvent.Entity != EntityWorkflowState {
		t.Fatalf("expected entity %s, got %s", EntityWorkflowState, chunkEvent.Entity)
	}

	status := chunkEvent.Data["status"]
	if status != "active" {
		t.Fatalf("expected status=active, got %v", status)
	}
	phase := chunkEvent.Data["current_phase"]
	if phase != "implementor" {
		t.Fatalf("expected current_phase=implementor, got %v", phase)
	}
	version := chunkEvent.Data["version"].(float64)
	if version != 4 {
		t.Fatalf("expected version=4, got %v", version)
	}
}
