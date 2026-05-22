package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
)

// TestRecordUserInputFallback verifies all fallback paths for recordUserInputFallback:
// inserts a user_input message, handles nil hub without panic, and handles orchestrator
// with no active spawner or a nil spawner (between phases).
func TestRecordUserInputFallback(t *testing.T) {
	tests := []struct {
		name       string
		nilHub     bool
		useOrch    bool // call via env.orch.RecordUserInput instead of recordUserInputFallback
		nilSpawner bool // register a run with an empty spawners map
		content    string
	}{
		{
			name:    "inserts row with user_input category",
			content: "typed hello",
		},
		{
			name:    "nil hub does not panic, still inserts",
			nilHub:  true,
			content: "nil hub msg",
		},
		{
			name:    "orchestrator fallback: no active run",
			useOrch: true,
			content: "orchestrator fallback text",
		},
		{
			name:       "orchestrator fallback: nil spawner between phases",
			useOrch:    true,
			nilSpawner: true,
			content:    "nil spawner fallback text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.createTicket(t, "RUI-TB", "table test")
			wfiID := env.initWorkflow(t, "RUI-TB")

			sessionID := "sess-rui-tb-" + tt.name
			insertRunningSession(t, env, wfiID, "RUI-TB", sessionID)

			if tt.nilSpawner {
				env.orch.mu.Lock()
				env.orch.runs[wfiID] = &runState{cancel: func() {}, spawners: make(map[string]*spawner.Spawner)}
				env.orch.mu.Unlock()
				t.Cleanup(func() {
					env.orch.mu.Lock()
					delete(env.orch.runs, wfiID)
					env.orch.mu.Unlock()
				})
			}

			if tt.useOrch {
				env.orch.RecordUserInput(sessionID, tt.content)
			} else {
				var hub *ws.Hub
				if !tt.nilHub {
					hub = env.hub
				}
				recordUserInputFallback(env.dbPath, clock.Real(), hub, sessionID, tt.content)
			}

			msgRepo := repo.NewAgentMessageRepo(env.pool, clock.Real())
			msgs, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "user_input", 10, 0)
			if err != nil {
				t.Fatalf("query messages: %v", err)
			}
			if len(msgs) != 1 {
				t.Fatalf("expected 1 user_input message, got %d", len(msgs))
			}
			if msgs[0].Content != tt.content {
				t.Errorf("Content = %q, want %q", msgs[0].Content, tt.content)
			}
			if msgs[0].Category != "user_input" {
				t.Errorf("Category = %q, want user_input", msgs[0].Category)
			}
		})
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
