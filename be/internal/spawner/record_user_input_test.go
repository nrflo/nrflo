package spawner

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/ws"
)

// TestRecordUserInput_NoProcReturnsFalse verifies that RecordUserInput returns
// false when no proc is registered for the given sessionID.
func TestRecordUserInput_NoProcReturnsFalse(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	got := sp.RecordUserInput("nonexistent-session-xyz", "hello")
	if got {
		t.Error("RecordUserInput should return false when no proc is registered")
	}
}

// TestRecordUserInput_ProcHitReturnsTrue verifies that RecordUserInput returns
// true when a proc is registered for the sessionID.
func TestRecordUserInput_ProcHitReturnsTrue(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "RUI-HIT-1"
	wfiID := env.initWorkflow(t, ticketID)

	testClock := clock.NewTest(time.Now())
	hub := ws.NewHub(testClock)
	go hub.Run()
	t.Cleanup(hub.Stop)

	sp := New(Config{DataPath: env.dbPath, Pool: env.pool, WSHub: hub, Clock: testClock})

	sessionID := "sess-rui-hit-1"
	insertRUISession(t, env, wfiID, ticketID, sessionID)

	proc := newRUIProc(env.project, ticketID, sessionID)
	sp.registerSessionProc(sessionID, proc)

	got := sp.RecordUserInput(sessionID, "hello world")
	if !got {
		t.Error("RecordUserInput should return true when proc is registered")
	}
}

// TestRecordUserInput_ProcHitWritesToDB verifies that the message is persisted
// to agent_messages with category="user_input".
func TestRecordUserInput_ProcHitWritesToDB(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "RUI-DB-1"
	wfiID := env.initWorkflow(t, ticketID)

	testClock := clock.NewTest(time.Now())
	hub := ws.NewHub(testClock)
	go hub.Run()
	t.Cleanup(hub.Stop)

	sp := New(Config{DataPath: env.dbPath, Pool: env.pool, WSHub: hub, Clock: testClock})

	sessionID := "sess-rui-db-1"
	insertRUISession(t, env, wfiID, ticketID, sessionID)

	proc := newRUIProc(env.project, ticketID, sessionID)
	sp.registerSessionProc(sessionID, proc)

	sp.RecordUserInput(sessionID, "typed text")

	msgRepo := repo.NewAgentMessageRepo(env.pool, testClock)
	msgs, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "user_input", 10, 0)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 user_input message, got %d", len(msgs))
	}
	if msgs[0].Content != "typed text" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "typed text")
	}
	if msgs[0].Category != "user_input" {
		t.Errorf("Category = %q, want user_input", msgs[0].Category)
	}
}

// TestRecordUserInput_SeqIncrementsOnMultipleCalls verifies that successive
// RecordUserInput calls produce sequential DB rows.
func TestRecordUserInput_SeqIncrementsOnMultipleCalls(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "RUI-SEQ-1"
	wfiID := env.initWorkflow(t, ticketID)

	testClock := clock.NewTest(time.Now())
	hub := ws.NewHub(testClock)
	go hub.Run()
	t.Cleanup(hub.Stop)

	sp := New(Config{DataPath: env.dbPath, Pool: env.pool, WSHub: hub, Clock: testClock})

	sessionID := "sess-rui-seq-1"
	insertRUISession(t, env, wfiID, ticketID, sessionID)

	proc := newRUIProc(env.project, ticketID, sessionID)
	sp.registerSessionProc(sessionID, proc)

	sp.RecordUserInput(sessionID, "first")
	sp.RecordUserInput(sessionID, "second")
	sp.RecordUserInput(sessionID, "third")

	msgRepo := repo.NewAgentMessageRepo(env.pool, testClock)
	msgs, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "user_input", 10, 0)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 user_input messages, got %d", len(msgs))
	}
	want := []string{"first", "second", "third"}
	for i, w := range want {
		if msgs[i].Content != w {
			t.Errorf("msgs[%d].Content = %q, want %q", i, msgs[i].Content, w)
		}
	}
}

// TestRecordUserInput_BroadcastFired verifies that a messages.updated WS event
// is broadcast after a user_input message is recorded.
func TestRecordUserInput_BroadcastFired(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "RUI-BC-1"
	wfiID := env.initWorkflow(t, ticketID)

	testClock := clock.NewTest(time.Now())
	hub := ws.NewHub(testClock)
	go hub.Run()
	t.Cleanup(hub.Stop)

	sp := New(Config{DataPath: env.dbPath, Pool: env.pool, WSHub: hub, Clock: testClock})

	sessionID := "sess-rui-bc-1"
	insertRUISession(t, env, wfiID, ticketID, sessionID)

	client, sendCh := ws.NewTestClient(hub, "rui-bc-client")
	hub.Register(client)
	hub.Subscribe(client, env.project, ticketID)

	proc := newRUIProc(env.project, ticketID, sessionID)
	sp.registerSessionProc(sessionID, proc)

	sp.RecordUserInput(sessionID, "broadcast test")

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventMessagesUpdated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventMessagesUpdated)
		}
		sid, _ := event.Data["session_id"].(string)
		if sid != sessionID {
			t.Errorf("event.Data.session_id = %q, want %q", sid, sessionID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: expected messages.updated broadcast after RecordUserInput")
	}
}

// TestRecordUserInput_UnregisterAfterRemoval verifies that after
// unregisterSessionProcs the same session returns false.
func TestRecordUserInput_UnregisterAfterRemoval(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})

	proc := &processInfo{
		sessionID:       "sess-rui-unreg-1",
		pendingMessages: make([]repo.MessageEntry, 0),
	}
	sp.registerSessionProc(proc.sessionID, proc)

	// First call hits the proc.
	if !sp.RecordUserInput(proc.sessionID, "before") {
		t.Error("expected true before unregister")
	}

	// Unregister the proc.
	sp.unregisterSessionProcs([]*processInfo{proc})

	// Second call must return false.
	if sp.RecordUserInput(proc.sessionID, "after") {
		t.Error("expected false after unregister")
	}
}

// insertRUISession inserts a minimal agent_session row so saveMessages can write.
func insertRUISession(t *testing.T, env *spawnerTestEnv, wfiID, ticketID, sessionID string) {
	t.Helper()
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			 model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'phase1', 'test-agent', 'claude:sonnet', 'running',
		        datetime('now'), datetime('now'))`,
		sessionID, env.project, ticketID, wfiID)
	if err != nil {
		t.Fatalf("insertRUISession: %v", err)
	}
}

// newRUIProc creates a minimal processInfo for RecordUserInput tests.
func newRUIProc(projectID, ticketID, sessionID string) *processInfo {
	return &processInfo{
		sessionID:       sessionID,
		projectID:       projectID,
		ticketID:        ticketID,
		agentType:       "test-agent",
		workflowName:    "test",
		modelID:         "claude:sonnet",
		pendingMessages: make([]repo.MessageEntry, 0),
		nextSeq:         0,
	}
}
