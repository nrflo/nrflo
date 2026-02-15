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

func TestHubBroadcastWithEventLog(t *testing.T) {
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

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	hub.Subscribe(client, "proj-1", "ticket-1")

	// Broadcast event
	event := NewEvent(EventMessagesUpdated, "proj-1", "ticket-1", "feature", map[string]interface{}{
		"session_id": "s1",
	})
	hub.Broadcast(event)

	// Wait for event to be processed
	var received Event
	select {
	case msg := <-client.send:
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	// Event should have sequence number assigned
	if received.Sequence == 0 {
		t.Fatal("expected non-zero sequence number")
	}
	if received.Sequence != 1 {
		t.Fatalf("expected sequence=1, got %d", received.Sequence)
	}

	// Event should have protocol version set
	if received.ProtocolVersion != ProtocolVersion {
		t.Fatalf("expected protocol_version=%d, got %d", ProtocolVersion, received.ProtocolVersion)
	}

	// Event should be persisted in log
	entries, err := eventLog.QuerySince("proj-1", "ticket-1", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 event in log, got %d", len(entries))
	}
	if entries[0].Seq != 1 {
		t.Fatalf("expected seq=1 in log, got %d", entries[0].Seq)
	}
	if entries[0].EventType != EventMessagesUpdated {
		t.Fatalf("expected event type %s, got %s", EventMessagesUpdated, entries[0].EventType)
	}
}

func TestHubBroadcastSequentialSequence(t *testing.T) {
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

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, "proj-1", "ticket-1")

	// Broadcast multiple events
	for i := 1; i <= 5; i++ {
		event := NewEvent(EventTestEcho, "proj-1", "ticket-1", "feature", map[string]interface{}{
			"index": i,
		})
		hub.Broadcast(event)
	}

	// Collect all events
	var sequences []int64
	timeout := time.After(2 * time.Second)
	for len(sequences) < 5 {
		select {
		case msg := <-client.send:
			var evt Event
			if err := json.Unmarshal(msg, &evt); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			sequences = append(sequences, evt.Sequence)
		case <-timeout:
			t.Fatalf("timeout waiting for events, got %d/5", len(sequences))
		}
	}

	// Sequences should be 1, 2, 3, 4, 5
	for i := 0; i < 5; i++ {
		expected := int64(i + 1)
		if sequences[i] != expected {
			t.Fatalf("expected seq=%d at index %d, got %d", expected, i, sequences[i])
		}
	}
}

func TestHubBroadcastAcrossProjects(t *testing.T) {
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

	// Broadcast events to different projects
	hub.Broadcast(NewEvent(EventTestEcho, "proj-1", "ticket-1", "feature", nil))
	time.Sleep(50 * time.Millisecond)
	hub.Broadcast(NewEvent(EventTestEcho, "proj-2", "ticket-1", "feature", nil))
	time.Sleep(50 * time.Millisecond)
	hub.Broadcast(NewEvent(EventTestEcho, "proj-1", "ticket-2", "feature", nil))
	time.Sleep(50 * time.Millisecond)

	// Check sequences are global across all scopes
	seq1, _ := eventLog.LatestSeq("proj-1", "ticket-1")
	seq2, _ := eventLog.LatestSeq("proj-2", "ticket-1")
	seq3, _ := eventLog.LatestSeq("proj-1", "ticket-2")

	if seq1 != 1 {
		t.Fatalf("expected seq=1 for proj-1/ticket-1, got %d", seq1)
	}
	if seq2 != 2 {
		t.Fatalf("expected seq=2 for proj-2/ticket-1, got %d", seq2)
	}
	if seq3 != 3 {
		t.Fatalf("expected seq=3 for proj-1/ticket-2, got %d", seq3)
	}
}

func TestHubBroadcastWithoutEventLog(t *testing.T) {
	// Hub without event log should work (backward compatibility)
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, "proj-1", "ticket-1")

	event := NewEvent(EventTestEcho, "proj-1", "ticket-1", "feature", nil)
	hub.Broadcast(event)

	select {
	case msg := <-client.send:
		var received Event
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		// Should receive event but without sequence or protocol version
		if received.Sequence != 0 {
			t.Fatalf("expected seq=0 without event log, got %d", received.Sequence)
		}
		if received.ProtocolVersion != 0 {
			t.Fatalf("expected protocol_version=0 without event log, got %d", received.ProtocolVersion)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestHubBroadcastCaseNormalization(t *testing.T) {
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

	// Broadcast with mixed case
	hub.Broadcast(NewEvent(EventTestEcho, "PROJ-1", "TICKET-1", "feature", nil))
	time.Sleep(50 * time.Millisecond)

	// Should be stored as lowercase
	entries, err := eventLog.QuerySince("proj-1", "ticket-1", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ProjectID != "proj-1" {
		t.Fatalf("expected lowercase project_id, got %s", entries[0].ProjectID)
	}
	if entries[0].TicketID != "ticket-1" {
		t.Fatalf("expected lowercase ticket_id, got %s", entries[0].TicketID)
	}
}
