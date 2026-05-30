package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/ws"
)

// thinkingLine returns a JSONL-encoded assistant message containing one or more
// thinking blocks. It does NOT include a trailing newline.
func thinkingLine(texts ...string) []byte {
	var blocks []map[string]interface{}
	for _, txt := range texts {
		blocks = append(blocks, map[string]interface{}{"type": "thinking", "thinking": txt})
	}
	line, _ := json.Marshal(map[string]interface{}{
		"type":    "assistant",
		"message": map[string]interface{}{"content": blocks},
	})
	return line
}

// enableThinking sets capture_thinking_enabled=true for env.project.
func enableThinking(t *testing.T, env *handlerTestEnv) {
	t.Helper()
	svc := service.NewGlobalSettingsService(env.pool, clock.Real())
	enabled := true
	if err := svc.SetCaptureThinkingEnabledForProject(env.project, &enabled); err != nil {
		t.Fatalf("enableThinking: %v", err)
	}
}

// setupThinkingSession creates a ticket+workflow+session and enables thinking.
// Returns the test env, a fresh Handler, and the sessionID.
func setupThinkingSession(t *testing.T, ticketID string) (*handlerTestEnv, *Handler, string) {
	t.Helper()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, ticketID)
	wfiID := queryWFIID(t, env, ticketID)
	sessionID := "sess-think-" + ticketID
	insertAgentSession(t, env, ticketID, sessionID, wfiID)
	enableThinking(t, env)
	return env, NewHandler(env.pool, env.hub, clock.Real(), nil), sessionID
}

// TestTailThinking_EmptyPath verifies that an empty transcript_path is a no-op.
func TestTailThinking_EmptyPath(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-EMPTY-PATH")
	h.tailThinking(context.Background(), sessionID, "")
	if n := countAgentMessages(t, env, sessionID); n != 0 {
		t.Errorf("countAgentMessages = %d, want 0 after empty path", n)
	}
}

// TestTailThinking_MissingFile verifies that a non-existent transcript file is a no-op.
func TestTailThinking_MissingFile(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-MISS-FILE")
	h.tailThinking(context.Background(), sessionID, "/nonexistent/path/transcript.jsonl")
	if n := countAgentMessages(t, env, sessionID); n != 0 {
		t.Errorf("countAgentMessages = %d, want 0 after missing file", n)
	}
}

// TestTailThinking_DisabledGate verifies that when capture_thinking_enabled=false
// no rows are inserted and no messages.updated event is broadcast.
func TestTailThinking_DisabledGate(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TH-DISABLED")
	wfiID := queryWFIID(t, env, "TH-DISABLED")
	sessionID := "sess-think-disabled"
	insertAgentSession(t, env, "TH-DISABLED", sessionID, wfiID)
	// Do NOT enable thinking — default is false.

	client, sendCh := ws.NewTestClient(env.hub, "th-disabled-client")
	env.hub.Subscribe(client, env.project, "TH-DISABLED")

	f, err := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	fmt.Fprintf(f, "%s\n", thinkingLine("a thought"))
	f.Close()

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	h.tailThinking(context.Background(), sessionID, f.Name())

	if n := countAgentMessages(t, env, sessionID); n != 0 {
		t.Errorf("countAgentMessages = %d, want 0 when gate disabled", n)
	}
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	select {
	case msg := <-sendCh:
		var ev ws.Event
		_ = json.Unmarshal(msg, &ev)
		if ev.Type == ws.EventMessagesUpdated {
			t.Errorf("unexpected messages.updated broadcast when thinking disabled")
		}
	case <-timer.C:
		// Expected: no broadcast fired.
	}
}

// TestTailThinking_FirstCallInsertsRows verifies the first tailThinking call
// on a file with N thinking lines inserts exactly N rows.
func TestTailThinking_FirstCallInsertsRows(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-FIRST-N")
	f, _ := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	fmt.Fprintf(f, "%s\n%s\n%s\n", thinkingLine("t1"), thinkingLine("t2"), thinkingLine("t3"))
	f.Close()

	h.tailThinking(context.Background(), sessionID, f.Name())

	if n := countAgentMessages(t, env, sessionID); n != 3 {
		t.Errorf("countAgentMessages = %d, want 3 after first call with 3 thinking lines", n)
	}
}

// TestTailThinking_SecondCallNoDuplicates verifies that a second tailThinking
// call on an unchanged file inserts 0 additional rows.
func TestTailThinking_SecondCallNoDuplicates(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-NO-DUP")
	f, _ := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	fmt.Fprintf(f, "%s\n%s\n", thinkingLine("alpha"), thinkingLine("beta"))
	f.Close()

	h.tailThinking(context.Background(), sessionID, f.Name())
	if n := countAgentMessages(t, env, sessionID); n != 2 {
		t.Fatalf("first call: countAgentMessages = %d, want 2", n)
	}

	h.tailThinking(context.Background(), sessionID, f.Name())
	if n := countAgentMessages(t, env, sessionID); n != 2 {
		t.Errorf("second call: countAgentMessages = %d, want still 2 (no duplicates)", n)
	}
}
