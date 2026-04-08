package spawner

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/ws"
)

func setupTestDB(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open test pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Seed parent rows for FK constraints
	now := "2025-01-01T00:00:00Z"
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj', 'Test', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, created_at, updated_at) VALUES ('feature', 'proj', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES ('T-1', 'proj', 'Test', ?, ?, 'test')`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-1', 'proj', 'T-1', 'feature', 'active', '{}', 0, ?, ?)`, now, now)

	return pool
}

func mustExec(t *testing.T, pool *db.Pool, query string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(query, args...); err != nil {
		t.Fatalf("mustExec failed: %v\n  query: %s", err, query)
	}
}

func insertSession(t *testing.T, pool *db.Pool, id string, contextLeft int) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, created_at, updated_at)
		VALUES (?, 'proj', 'T-1', 'wfi-1', 'phase1', 'analyzer', 'running', ?, datetime('now'), datetime('now'))`,
		id, contextLeft)
}

func TestReadContextLeftFromDB_UpdatesProcs(t *testing.T) {
	pool := setupTestDB(t)
	insertSession(t, pool, "sess-1", 60)
	insertSession(t, pool, "sess-2", 30)

	procs := []*processInfo{
		{sessionID: "sess-1", contextLeft: 0},
		{sessionID: "sess-2", contextLeft: 0},
	}

	readContextLeftFromDB(pool, procs)

	if procs[0].contextLeft != 60 {
		t.Errorf("sess-1 contextLeft = %d, want 60", procs[0].contextLeft)
	}
	if procs[1].contextLeft != 30 {
		t.Errorf("sess-2 contextLeft = %d, want 30", procs[1].contextLeft)
	}
}

func TestReadContextLeftFromDB_NilPool(t *testing.T) {
	proc := &processInfo{sessionID: "sess-1", contextLeft: 75}
	readContextLeftFromDB(nil, []*processInfo{proc})

	if proc.contextLeft != 75 {
		t.Errorf("contextLeft = %d, want 75 (unchanged)", proc.contextLeft)
	}
}

func TestReadContextLeftFromDB_EmptyProcs(t *testing.T) {
	pool := setupTestDB(t)
	readContextLeftFromDB(pool, nil)
	readContextLeftFromDB(pool, []*processInfo{})
}

func TestReadContextLeftFromDB_SessionNotInDB(t *testing.T) {
	pool := setupTestDB(t)

	proc := &processInfo{sessionID: "nonexistent", contextLeft: 50}
	readContextLeftFromDB(pool, []*processInfo{proc})

	if proc.contextLeft != 50 {
		t.Errorf("contextLeft = %d, want 50 (unchanged)", proc.contextLeft)
	}
}

func TestReadContextLeftFromDB_HigherDBValueNotOverwritten(t *testing.T) {
	pool := setupTestDB(t)
	// DB holds contextLeft=80 (less context used), but proc already recorded 50 (more context used)
	insertSession(t, pool, "sess-higher-db", 80)

	proc := &processInfo{sessionID: "sess-higher-db", contextLeft: 50}
	readContextLeftFromDB(pool, []*processInfo{proc})

	if proc.contextLeft != 50 {
		t.Errorf("contextLeft = %d, want 50 (higher DB value must not overwrite lower in-memory value)", proc.contextLeft)
	}
}

func TestReadContextLeftFromDB_LowerDBValueUpdatesProc(t *testing.T) {
	pool := setupTestDB(t)
	// DB holds contextLeft=30 (more context used than in-memory 50) — should update
	insertSession(t, pool, "sess-lower-db", 30)

	proc := &processInfo{sessionID: "sess-lower-db", contextLeft: 50}
	readContextLeftFromDB(pool, []*processInfo{proc})

	if proc.contextLeft != 30 {
		t.Errorf("contextLeft = %d, want 30 (lower DB value should update proc)", proc.contextLeft)
	}
}

// === updateContextLeft: DB persistence and WS broadcast ===

func TestUpdateContextLeft_PersistsToDB(t *testing.T) {
	pool := setupTestDB(t)
	insertSession(t, pool, "sess-ucl-1", 100)

	s := New(Config{Pool: pool, Clock: clock.Real()})
	proc := &processInfo{
		sessionID:   "sess-ucl-1",
		contextLeft: 42,
	}

	s.updateContextLeft(proc)

	var contextLeft int
	err := pool.QueryRow(`SELECT context_left FROM agent_sessions WHERE id = ?`, "sess-ucl-1").Scan(&contextLeft)
	if err != nil {
		t.Fatalf("failed to query context_left: %v", err)
	}
	if contextLeft != 42 {
		t.Errorf("context_left = %d, want 42", contextLeft)
	}
}

func TestUpdateContextLeft_NilPool_NoError(t *testing.T) {
	s := noPoolSpawner()
	proc := &processInfo{
		sessionID:   "sess-ucl-nil",
		contextLeft: 30,
	}
	// Must not panic
	s.updateContextLeft(proc)
}

func TestUpdateContextLeft_BroadcastsWSEvent(t *testing.T) {
	pool := setupTestDB(t)
	insertSession(t, pool, "sess-ucl-ws", 100)

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	s := New(Config{Pool: pool, WSHub: hub, Clock: clock.Real()})

	client, sendCh := ws.NewTestClient(hub, "client-ucl-ws")
	hub.Register(client)
	hub.Subscribe(client, "proj", "T-1")

	proc := &processInfo{
		sessionID:    "sess-ucl-ws",
		projectID:    "proj",
		ticketID:     "T-1",
		workflowName: "feature",
		contextLeft:  20,
	}

	s.updateContextLeft(proc)

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentContextUpdated {
			t.Errorf("event type = %q, want %q", event.Type, ws.EventAgentContextUpdated)
		}
		sid, _ := event.Data["session_id"].(string)
		if sid != "sess-ucl-ws" {
			t.Errorf("session_id = %q, want %q", sid, "sess-ucl-ws")
		}
		ctxLeft, _ := event.Data["context_left"].(float64)
		if int(ctxLeft) != 20 {
			t.Errorf("context_left = %.0f, want 20", ctxLeft)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.context_updated event")
	}
}
