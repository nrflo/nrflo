package spawner

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// --- Helpers ---

// minProc creates a minimal processInfo for output parsing unit tests.
func minProc(sessionID string) *processInfo {
	return &processInfo{
		sessionID:       sessionID,
		agentType:       "test-agent",
		modelID:         "codex:codex_gpt_high",
		pendingMessages: make([]repo.MessageEntry, 0),
		pendingTasks:    make(map[string]taskInfo),
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

// pendingMessages is a test helper that returns message content strings under the lock.
func pendingMessages(proc *processInfo) []string {
	proc.messagesMutex.Lock()
	defer proc.messagesMutex.Unlock()
	out := make([]string, len(proc.pendingMessages))
	for i, m := range proc.pendingMessages {
		out[i] = m.Content
	}
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
}
