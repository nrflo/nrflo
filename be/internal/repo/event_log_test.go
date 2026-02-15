package repo

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func TestEventLogAppend(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewEventLogRepo(pool, clock.Real())

	// Append first event
	payload1 := json.RawMessage(`{"session_id":"s1"}`)
	seq1, err := repo.Append("proj-1", "ticket-1", "agent.started", "feature", payload1)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if seq1 != 1 {
		t.Fatalf("expected seq=1, got %d", seq1)
	}

	// Append second event (should increment seq)
	payload2 := json.RawMessage(`{"session_id":"s2"}`)
	seq2, err := repo.Append("proj-1", "ticket-1", "agent.completed", "feature", payload2)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if seq2 != 2 {
		t.Fatalf("expected seq=2, got %d", seq2)
	}

	// Append to different scope (should still increment global seq)
	seq3, err := repo.Append("proj-1", "ticket-2", "agent.started", "bugfix", nil)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if seq3 != 3 {
		t.Fatalf("expected seq=3, got %d", seq3)
	}

	// Append with nil payload (should default to {})
	seq4, err := repo.Append("proj-2", "", "workflow.updated", "", nil)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if seq4 != 4 {
		t.Fatalf("expected seq=4, got %d", seq4)
	}
}

func TestEventLogQuerySince(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewEventLogRepo(pool, clock.Real())

	// Insert multiple events in same scope
	for i := 1; i <= 5; i++ {
		payload := json.RawMessage(`{"index":` + string(rune(i+'0')) + `}`)
		_, err := repo.Append("proj-1", "ticket-1", "test.echo", "feature", payload)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Insert event in different scope
	_, err = repo.Append("proj-1", "ticket-2", "test.echo", "feature", nil)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Query since seq=2
	entries, err := repo.QuerySince("proj-1", "ticket-1", 2, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Should get seq 3, 4, 5
	if entries[0].Seq != 3 || entries[1].Seq != 4 || entries[2].Seq != 5 {
		t.Fatalf("expected seq 3,4,5, got %d,%d,%d", entries[0].Seq, entries[1].Seq, entries[2].Seq)
	}

	// Query since seq=0 (should get all events for this scope)
	entries, err = repo.QuerySince("proj-1", "ticket-1", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Query with limit
	entries, err = repo.QuerySince("proj-1", "ticket-1", 0, 2)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (limit), got %d", len(entries))
	}

	// Query since current seq (should get empty result)
	entries, err = repo.QuerySince("proj-1", "ticket-1", 5, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}

	// Query different scope
	entries, err = repo.QuerySince("proj-1", "ticket-2", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for ticket-2, got %d", len(entries))
	}
	if entries[0].Seq != 6 {
		t.Fatalf("expected seq=6 for ticket-2, got %d", entries[0].Seq)
	}

	// Query non-existent scope
	entries, err = repo.QuerySince("proj-99", "ticket-99", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for non-existent scope, got %d", len(entries))
	}
}

func TestEventLogLatestSeq(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewEventLogRepo(pool, clock.Real())

	// Empty log returns 0
	seq, err := repo.LatestSeq("proj-1", "ticket-1")
	if err != nil {
		t.Fatalf("LatestSeq failed: %v", err)
	}
	if seq != 0 {
		t.Fatalf("expected seq=0 for empty log, got %d", seq)
	}

	// Add events
	_, err = repo.Append("proj-1", "ticket-1", "test.echo", "feature", nil)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	_, err = repo.Append("proj-1", "ticket-1", "test.echo", "feature", nil)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Should return latest seq
	seq, err = repo.LatestSeq("proj-1", "ticket-1")
	if err != nil {
		t.Fatalf("LatestSeq failed: %v", err)
	}
	if seq != 2 {
		t.Fatalf("expected seq=2, got %d", seq)
	}

	// Add event in different scope
	_, err = repo.Append("proj-1", "ticket-2", "test.echo", "feature", nil)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Original scope should still return 2
	seq, err = repo.LatestSeq("proj-1", "ticket-1")
	if err != nil {
		t.Fatalf("LatestSeq failed: %v", err)
	}
	if seq != 2 {
		t.Fatalf("expected seq=2 for ticket-1, got %d", seq)
	}

	// New scope should return 3
	seq, err = repo.LatestSeq("proj-1", "ticket-2")
	if err != nil {
		t.Fatalf("LatestSeq failed: %v", err)
	}
	if seq != 3 {
		t.Fatalf("expected seq=3 for ticket-2, got %d", seq)
	}
}

func TestEventLogCleanup(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewEventLogRepo(pool, clock.Real())

	// Insert an old event by manually setting created_at
	cutoff := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	_, err = pool.Exec(
		`INSERT INTO ws_event_log (project_id, ticket_id, event_type, workflow, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"proj-1", "ticket-1", "test.echo", "feature", "{}", cutoff,
	)
	if err != nil {
		t.Fatalf("failed to insert old event: %v", err)
	}

	// Insert recent events
	for i := 0; i < 3; i++ {
		_, err := repo.Append("proj-1", "ticket-1", "test.echo", "feature", nil)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Should have 4 events total
	entries, err := repo.QuerySince("proj-1", "ticket-1", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 events before cleanup, got %d", len(entries))
	}

	// Cleanup events older than 24h
	deleted, err := repo.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted row, got %d", deleted)
	}

	// Should have 3 events remaining
	entries, err = repo.QuerySince("proj-1", "ticket-1", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 events after cleanup, got %d", len(entries))
	}

	// Cleanup with negative duration (should remove all events including recent ones)
	deleted, err = repo.Cleanup(-1 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted rows, got %d", deleted)
	}

	// Should have 0 events
	entries, err = repo.QuerySince("proj-1", "ticket-1", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 events after full cleanup, got %d", len(entries))
	}
}
