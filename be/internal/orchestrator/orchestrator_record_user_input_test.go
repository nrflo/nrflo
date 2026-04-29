package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/ws"
)

// TestRecordUserInputFallback_InsertsRowWithUserInputCategory verifies that
// recordUserInputFallback writes a message to agent_messages with
// category="user_input".
func TestRecordUserInputFallback_InsertsRowWithUserInputCategory(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RUI-FB-1", "fallback category test")
	wfiID := env.initWorkflow(t, "RUI-FB-1")

	sessionID := "sess-rui-fb-1"
	insertRunningSession(t, env, wfiID, "RUI-FB-1", sessionID)

	recordUserInputFallback(env.dbPath, clock.Real(), env.hub, sessionID, "typed hello")

	msgRepo := repo.NewAgentMessageRepo(env.pool, clock.Real())
	msgs, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "user_input", 10, 0)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 user_input message, got %d", len(msgs))
	}
	if msgs[0].Content != "typed hello" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "typed hello")
	}
	if msgs[0].Category != "user_input" {
		t.Errorf("Category = %q, want user_input", msgs[0].Category)
	}
}

// TestRecordUserInputFallback_BroadcastsMessagesUpdated verifies that
// recordUserInputFallback broadcasts an EventMessagesUpdated WS event after
// inserting the message.
func TestRecordUserInputFallback_BroadcastsMessagesUpdated(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RUI-FB-2", "fallback broadcast test")
	wfiID := env.initWorkflow(t, "RUI-FB-2")

	sessionID := "sess-rui-fb-2"
	insertRunningSession(t, env, wfiID, "RUI-FB-2", sessionID)

	client, sendCh := ws.NewTestClient(env.hub, "rui-fb-client")
	env.hub.Register(client)
	env.hub.Subscribe(client, env.project, "RUI-FB-2")

	recordUserInputFallback(env.dbPath, clock.Real(), env.hub, sessionID, "broadcast check")

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
		t.Fatal("timeout: expected messages.updated broadcast from recordUserInputFallback")
	}
}

// TestRecordUserInputFallback_NilHub verifies that recordUserInputFallback with
// a nil hub still inserts the message and does not panic.
func TestRecordUserInputFallback_NilHub(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RUI-NILHUB-1", "nil hub test")
	wfiID := env.initWorkflow(t, "RUI-NILHUB-1")

	sessionID := "sess-rui-nilhub-1"
	insertRunningSession(t, env, wfiID, "RUI-NILHUB-1", sessionID)

	// Must not panic.
	recordUserInputFallback(env.dbPath, clock.Real(), nil, sessionID, "nil hub msg")

	msgRepo := repo.NewAgentMessageRepo(env.pool, clock.Real())
	msgs, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "user_input", 10, 0)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "nil hub msg" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "nil hub msg")
	}
}

// TestOrchestrator_RecordUserInput_Fallback verifies that the orchestrator's
// RecordUserInput falls back to direct DB insert when no active spawner owns
// the session (e.g. user_interactive / resume-session flows).
func TestOrchestrator_RecordUserInput_Fallback(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RUI-ORCH-1", "orchestrator fallback test")
	wfiID := env.initWorkflow(t, "RUI-ORCH-1")

	sessionID := "sess-rui-orch-1"
	insertRunningSession(t, env, wfiID, "RUI-ORCH-1", sessionID)

	// No runs registered — orchestrator has no active spawner.
	env.orch.RecordUserInput(sessionID, "orchestrator fallback text")

	msgRepo := repo.NewAgentMessageRepo(env.pool, clock.Real())
	msgs, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "user_input", 10, 0)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 user_input message, got %d", len(msgs))
	}
	if msgs[0].Content != "orchestrator fallback text" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "orchestrator fallback text")
	}
}

// TestOrchestrator_RecordUserInput_NilSpawnerFallback verifies that when an
// active run has a nil spawner (between phases), RecordUserInput still falls
// back to the direct DB path.
func TestOrchestrator_RecordUserInput_NilSpawnerFallback(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RUI-NILSP-1", "nil spawner fallback test")
	wfiID := env.initWorkflow(t, "RUI-NILSP-1")

	sessionID := "sess-rui-nilsp-1"
	insertRunningSession(t, env, wfiID, "RUI-NILSP-1", sessionID)

	// Register a run state with nil spawner (between phases).
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}, spawner: nil}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	// With nil spawner the fallback path must insert into DB.
	env.orch.RecordUserInput(sessionID, "nil spawner fallback text")

	msgRepo := repo.NewAgentMessageRepo(env.pool, clock.Real())
	msgs, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "user_input", 10, 0)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 user_input message via fallback, got %d", len(msgs))
	}
	if msgs[0].Content != "nil spawner fallback text" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "nil spawner fallback text")
	}
}
