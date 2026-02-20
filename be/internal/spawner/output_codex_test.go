package spawner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// --- Helpers ---

// minProc creates a minimal processInfo for output parsing unit tests.
func minProc(sessionID string) *processInfo {
	return &processInfo{
		sessionID:       sessionID,
		agentType:       "test-agent",
		modelID:         "codex:codex_gpt_high",
		pendingMessages: make([]string, 0),
	}
}

// noPoolSpawner creates a Spawner with no DB pool and no WS hub.
func noPoolSpawner() *Spawner {
	return New(Config{Clock: clock.Real()})
}

// processJSON is a test helper that marshals data and calls processOutput.
func processJSON(s *Spawner, proc *processInfo, data map[string]interface{}) {
	line, _ := json.Marshal(data)
	s.processOutput(proc, string(line))
}

// pendingMessages is a test helper that returns a copy of proc.pendingMessages under the lock.
func pendingMessages(proc *processInfo) []string {
	proc.messagesMutex.Lock()
	defer proc.messagesMutex.Unlock()
	out := make([]string, len(proc.pendingMessages))
	copy(out, proc.pendingMessages)
	return out
}

// === thread.started ===

func TestProcessOutput_ThreadStarted_SetsExternalSessionID(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ts-1")

	processJSON(s, proc, map[string]interface{}{
		"type":      "thread.started",
		"thread_id": "019c7aa2-8427-7850-bfc9-c5539d7937a0",
	})

	if proc.externalSessionID != "019c7aa2-8427-7850-bfc9-c5539d7937a0" {
		t.Errorf("externalSessionID = %q, want %q", proc.externalSessionID, "019c7aa2-8427-7850-bfc9-c5539d7937a0")
	}
}

func TestProcessOutput_ThreadStarted_EmptyThreadIDIgnored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ts-2")
	proc.externalSessionID = "existing"

	processJSON(s, proc, map[string]interface{}{
		"type":      "thread.started",
		"thread_id": "",
	})

	if proc.externalSessionID != "existing" {
		t.Errorf("externalSessionID = %q, want %q (unchanged)", proc.externalSessionID, "existing")
	}
}

func TestProcessOutput_ThreadStarted_MissingThreadIDIgnored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ts-3")
	proc.externalSessionID = "existing"

	processJSON(s, proc, map[string]interface{}{
		"type": "thread.started",
	})

	if proc.externalSessionID != "existing" {
		t.Errorf("externalSessionID = %q, want %q (unchanged)", proc.externalSessionID, "existing")
	}
}

// === item.completed: reasoning ===

func TestProcessOutput_ItemCompleted_Reasoning_TracksThinkingMessage(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ic-r")

	processJSON(s, proc, map[string]interface{}{
		"type": "item.completed",
		"item": map[string]interface{}{
			"id":   "item_0",
			"type": "reasoning",
			"text": "**Thinking...**",
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.HasPrefix(msgs[0], "[thinking] ") {
		t.Errorf("reasoning message should start with '[thinking] ', got: %q", msgs[0])
	}
	if !strings.Contains(msgs[0], "**Thinking...**") {
		t.Errorf("reasoning message should contain the text, got: %q", msgs[0])
	}
}

func TestProcessOutput_ItemCompleted_Reasoning_EmptyTextIgnored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ic-r2")

	processJSON(s, proc, map[string]interface{}{
		"type": "item.completed",
		"item": map[string]interface{}{
			"id":   "item_0",
			"type": "reasoning",
			"text": "",
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for empty reasoning text, got %d: %v", len(msgs), msgs)
	}
}

// === item.completed: agent_message ===

func TestProcessOutput_ItemCompleted_AgentMessage_TracksMessage(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ic-am")

	processJSON(s, proc, map[string]interface{}{
		"type": "item.completed",
		"item": map[string]interface{}{
			"id":   "item_1",
			"type": "agent_message",
			"text": "hello",
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0] != "hello" {
		t.Errorf("agent_message = %q, want %q", msgs[0], "hello")
	}
}

// === item.completed: command_execution ===

func TestProcessOutput_ItemCompleted_CommandExecution_TracksAsBashToolUse(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ic-ce")

	processJSON(s, proc, map[string]interface{}{
		"type": "item.completed",
		"item": map[string]interface{}{
			"id":               "item_2",
			"type":             "command_execution",
			"command":          "/bin/zsh -lc \"ls\"",
			"aggregated_output": "file1\nfile2",
			"exit_code":        0,
			"status":           "completed",
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "[Bash]") {
		t.Errorf("command_execution should track as Bash tool use, got: %q", msgs[0])
	}
	if !strings.Contains(msgs[0], "/bin/zsh") {
		t.Errorf("Bash message should contain command, got: %q", msgs[0])
	}
}

func TestProcessOutput_ItemCompleted_CommandExecution_EmptyCommandIgnored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ic-ce2")

	processJSON(s, proc, map[string]interface{}{
		"type": "item.completed",
		"item": map[string]interface{}{
			"id":      "item_2",
			"type":    "command_execution",
			"command": "",
			"status":  "completed",
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for empty command, got %d: %v", len(msgs), msgs)
	}
}

// === item.started ===

func TestProcessOutput_ItemStarted_CommandExecution_DoesNotTrackMessage(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-is-ce")

	processJSON(s, proc, map[string]interface{}{
		"type": "item.started",
		"item": map[string]interface{}{
			"id":      "item_2",
			"type":    "command_execution",
			"command": "/bin/zsh -lc \"ls\"",
			"status":  "in_progress",
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 0 {
		t.Errorf("item.started should not track any DB messages, got %d: %v", len(msgs), msgs)
	}
}

// === turn.completed: context left calculation ===

func TestProcessOutput_TurnCompleted_ContextLeft(t *testing.T) {
	tests := []struct {
		name        string
		input       float64
		output      float64
		wantPctLeft int
	}{
		// 150000+10000=160000 → 100 - (160000*100/200000) = 100-80 = 20
		{"normal", 150000, 10000, 20},
		// 195712+2966=198678 → 100 - (198678*100/200000) = 100-99 = 1
		{"near-full", 195712, 2966, 1},
		// 100000+0=100000 → 100 - (100000*100/200000) = 100-50 = 50
		{"half", 100000, 0, 50},
		// 0+0=0 → 100 - 0 = 100
		{"empty", 0, 0, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := noPoolSpawner()
			proc := minProc("sess-tc-" + tt.name)

			processJSON(s, proc, map[string]interface{}{
				"type": "turn.completed",
				"usage": map[string]interface{}{
					"input_tokens":        tt.input,
					"cached_input_tokens": 0,
					"output_tokens":       tt.output,
				},
			})

			if proc.contextLeft != tt.wantPctLeft {
				t.Errorf("contextLeft = %d, want %d (input=%.0f output=%.0f)", proc.contextLeft, tt.wantPctLeft, tt.input, tt.output)
			}
		})
	}
}

func TestProcessOutput_TurnCompleted_ExceedsMax_ClampsToZero(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tc-over")

	processJSON(s, proc, map[string]interface{}{
		"type": "turn.completed",
		"usage": map[string]interface{}{
			"input_tokens":  210000.0,
			"output_tokens": 5000.0,
		},
	})

	if proc.contextLeft != 0 {
		t.Errorf("contextLeft = %d, want 0 (tokens > max context)", proc.contextLeft)
	}
}

func TestProcessOutput_TurnCompleted_NilUsage_NoUpdate(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tc-nil")
	proc.contextLeft = 75

	processJSON(s, proc, map[string]interface{}{
		"type": "turn.completed",
	})

	if proc.contextLeft != 75 {
		t.Errorf("contextLeft = %d, want 75 (unchanged when no usage)", proc.contextLeft)
	}
}

// === updateContextLeft: DB persistence ===

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

	// Subscribe a test client
	client, sendCh := ws.NewTestClient(hub, "client-ucl-ws")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
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

// === Full Codex output sequence ===

func TestProcessOutput_CodexFullSequence(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-full")

	lines := []map[string]interface{}{
		{"type": "thread.started", "thread_id": "abc-123"},
		{"type": "turn.started"},
		{"type": "item.completed", "item": map[string]interface{}{
			"id": "item_0", "type": "reasoning", "text": "Analyzing..."}},
		{"type": "item.completed", "item": map[string]interface{}{
			"id": "item_1", "type": "agent_message", "text": "Done!"}},
		{"type": "item.started", "item": map[string]interface{}{
			"id": "item_2", "type": "command_execution", "command": "ls", "status": "in_progress"}},
		{"type": "item.completed", "item": map[string]interface{}{
			"id": "item_2", "type": "command_execution", "command": "ls", "status": "completed"}},
		{"type": "turn.completed", "usage": map[string]interface{}{
			"input_tokens": 150000.0, "output_tokens": 10000.0}},
	}

	for _, line := range lines {
		processJSON(s, proc, line)
	}

	// thread_id captured
	if proc.externalSessionID != "abc-123" {
		t.Errorf("externalSessionID = %q, want %q", proc.externalSessionID, "abc-123")
	}

	// messages: reasoning, agent_message, command_execution (not item.started)
	msgs := pendingMessages(proc)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d: %v", len(msgs), msgs)
	}

	if !strings.HasPrefix(msgs[0], "[thinking] ") {
		t.Errorf("msg[0] should be reasoning, got: %q", msgs[0])
	}
	if msgs[1] != "Done!" {
		t.Errorf("msg[1] should be agent_message, got: %q", msgs[1])
	}
	if !strings.Contains(msgs[2], "[Bash]") {
		t.Errorf("msg[2] should be Bash tool use, got: %q", msgs[2])
	}

	// contextLeft: 100 - (160000*100/200000) = 20
	if proc.contextLeft != 20 {
		t.Errorf("contextLeft = %d, want 20", proc.contextLeft)
	}
}
