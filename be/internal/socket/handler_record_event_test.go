package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// bumpRecordSignaler records BumpLastMessage calls; RequestTerminalSignal is a no-op.
type bumpRecordSignaler struct {
	bumps []string // session IDs that were bumped
}

func (b *bumpRecordSignaler) RequestTerminalSignal(_, _, _, _, _ string) error { return nil }
func (b *bumpRecordSignaler) BumpLastMessage(_, _, _, sessionID string) error {
	b.bumps = append(b.bumps, sessionID)
	return nil
}

// buildRecordEventParams marshals params for agent.record_event requests.
func buildRecordEventParams(t *testing.T, sessionID string, event map[string]interface{}) json.RawMessage {
	t.Helper()
	evtBytes, _ := json.Marshal(event)
	params, _ := json.Marshal(map[string]interface{}{
		"session_id": sessionID,
		"event":      json.RawMessage(evtBytes),
	})
	return params
}

// buildRecordEventReq constructs a Request for agent.record_event.
func buildRecordEventReq(t *testing.T, id, sessionID string, event map[string]interface{}) Request {
	t.Helper()
	return Request{
		ID:     id,
		Method: "agent.record_event",
		Params: buildRecordEventParams(t, sessionID, event),
	}
}

// countAgentMessages counts agent_messages rows for a session.
func countAgentMessages(t *testing.T, env *handlerTestEnv, sessionID string) int {
	t.Helper()
	var count int
	if err := env.pool.QueryRow(
		`SELECT COUNT(*) FROM agent_messages WHERE session_id = ?`, sessionID,
	).Scan(&count); err != nil {
		t.Fatalf("countAgentMessages(%q): %v", sessionID, err)
	}
	return count
}

// lastAgentMessage returns the content and category of the most recent message for a session.
func lastAgentMessage(t *testing.T, env *handlerTestEnv, sessionID string) (content, category string) {
	t.Helper()
	if err := env.pool.QueryRow(
		`SELECT content, category FROM agent_messages WHERE session_id = ? ORDER BY seq DESC LIMIT 1`,
		sessionID,
	).Scan(&content, &category); err != nil {
		t.Fatalf("lastAgentMessage(%q): %v", sessionID, err)
	}
	return content, category
}

// awaitWSEvent drains env.hub broadcast events until eventType is received or timeout fires.
// Returns the first matching event.
func awaitWSEvent(t *testing.T, ch <-chan []byte, eventType string) ws.Event {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case msg := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(msg, &ev); err != nil {
				t.Fatalf("unmarshal ws event: %v", err)
			}
			if ev.Type == eventType {
				return ev
			}
		case <-deadline:
			t.Fatalf("timeout waiting for WS event %q", eventType)
		}
	}
}

// TestRecordEvent_PreToolUse_InsertsMsgAndBroadcasts verifies that PreToolUse inserts an
// agent_messages row with category=tool and content="[Bash] ls", then broadcasts messages.updated.
func TestRecordEvent_PreToolUse_InsertsMsgAndBroadcasts(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-PRE-1")
	wfiID := queryWFIID(t, env, "RE-PRE-1")
	sessionID := "sess-re-pre-1"
	insertAgentSession(t, env, "RE-PRE-1", sessionID, wfiID)

	client, sendCh := ws.NewTestClient(env.hub, "re-pre-client")
	env.hub.Subscribe(client, env.project, "RE-PRE-1")

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-re-pre", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "ls"},
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "recorded" {
		t.Errorf("status = %q, want %q", result["status"], "recorded")
	}

	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	content, category := lastAgentMessage(t, env, sessionID)
	if content != "[Bash] ls" {
		t.Errorf("content = %q, want %q", content, "[Bash] ls")
	}
	if category != "tool" {
		t.Errorf("category = %q, want %q", category, "tool")
	}

	ev := awaitWSEvent(t, sendCh, ws.EventMessagesUpdated)
	if sid, _ := ev.Data["session_id"].(string); sid != sessionID {
		t.Errorf("broadcast session_id = %q, want %q", sid, sessionID)
	}
}

// TestRecordEvent_PostToolUse_InsertsResult verifies PostToolUse inserts "[Tool result] response".
func TestRecordEvent_PostToolUse_InsertsResult(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-POST-1")
	wfiID := queryWFIID(t, env, "RE-POST-1")
	sessionID := "sess-re-post-1"
	insertAgentSession(t, env, "RE-POST-1", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-re-post", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Read",
		"tool_response":   "file content here",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	content, category := lastAgentMessage(t, env, sessionID)
	if content != "[Read result] file content here" {
		t.Errorf("content = %q, want %q", content, "[Read result] file content here")
	}
	if category != "tool" {
		t.Errorf("category = %q, want %q", category, "tool")
	}
}

// TestRecordEvent_PostToolUse_LongResponseTruncated verifies tool_response > 200 chars is truncated.
func TestRecordEvent_PostToolUse_LongResponseTruncated(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-TRUNC-1")
	wfiID := queryWFIID(t, env, "RE-TRUNC-1")
	sessionID := "sess-re-trunc-1"
	insertAgentSession(t, env, "RE-TRUNC-1", sessionID, wfiID)

	longResp := make([]byte, 250)
	for i := range longResp {
		longResp[i] = 'x'
	}

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-re-trunc", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_response":   string(longResp),
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	content, _ := lastAgentMessage(t, env, sessionID)
	// Expected: "[Bash result] " + 200 x's + "..."
	const prefix = "[Bash result] "
	wantLen := len(prefix) + 200 + len("...")
	if len(content) != wantLen {
		t.Errorf("content length = %d, want %d\ncontent prefix: %q", len(content), wantLen, content[:min(40, len(content))])
	}
	if content[len(content)-3:] != "..." {
		t.Errorf("expected content to end with '...', got: %q", content[max(0, len(content)-10):])
	}
}

// TestRecordEvent_PreToolUse_UsageFieldIgnored verifies that a PreToolUse event
// containing a usage block still records the tool message successfully.
// Context is updated via the statusLine hook path, not PreToolUse payloads.
func TestRecordEvent_PreToolUse_UsageFieldIgnored(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-NOCTX-1")
	wfiID := queryWFIID(t, env, "RE-NOCTX-1")
	sessionID := "sess-re-noctx-1"
	insertAgentSession(t, env, "RE-NOCTX-1", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-re-noctx", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "pwd"},
		"usage": map[string]interface{}{
			"input_tokens":                50000.0,
			"cache_read_input_tokens":     10000.0,
			"cache_creation_input_tokens": 5000.0,
			"output_tokens":               1000.0,
		},
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "recorded" {
		t.Errorf("status = %q, want %q", result["status"], "recorded")
	}

	// Tool message is still inserted
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}

	// context_left stays at NULL/0 — PreToolUse no longer updates it
	var ctxLeft int
	if err := env.pool.QueryRow(`SELECT COALESCE(context_left, 0) FROM agent_sessions WHERE id = ?`, sessionID).Scan(&ctxLeft); err != nil {
		t.Fatalf("query context_left: %v", err)
	}
	if ctxLeft != 0 {
		t.Errorf("context_left = %d, want 0 (PreToolUse no longer updates context)", ctxLeft)
	}
}

// TestRecordEvent_IgnoredEvents verifies Stop/SessionEnd/UserPromptSubmit return status=ignored
// and insert no DB rows.
func TestRecordEvent_IgnoredEvents(t *testing.T) {
	for _, hookName := range []string{"Stop", "SessionEnd", "UserPromptSubmit"} {
		hookName := hookName
		t.Run(hookName, func(t *testing.T) {
			env := newHandlerTestEnv(t)
			h := NewHandler(env.pool, env.hub, clock.Real(), nil)

			req := buildRecordEventReq(t, "req-ign-"+hookName, "any-session", map[string]interface{}{
				"hook_event_name": hookName,
			})
			resp := h.Handle(req)

			if resp.Error != nil {
				t.Errorf("%s: expected no error, got: %v", hookName, resp.Error)
			}
			var result map[string]string
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("%s: unmarshal result: %v", hookName, err)
			}
			if result["status"] != "ignored" {
				t.Errorf("%s: status = %q, want %q", hookName, result["status"], "ignored")
			}
			// No DB rows should be inserted
			var count int
			_ = env.pool.QueryRow(`SELECT COUNT(*) FROM agent_messages WHERE session_id = ?`, "any-session").Scan(&count)
			if count != 0 {
				t.Errorf("%s: expected 0 agent_messages rows, got %d", hookName, count)
			}
		})
	}
}

// TestRecordEvent_UnknownHookEvent_Ignored verifies unrecognised hook_event_name returns status=ignored.
func TestRecordEvent_UnknownHookEvent_Ignored(t *testing.T) {
	env := newHandlerTestEnv(t)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	req := buildRecordEventReq(t, "req-unk", "any-session", map[string]interface{}{
		"hook_event_name": "Foobar",
	})
	resp := h.Handle(req)

	if resp.Error != nil {
		t.Fatalf("expected no error for unknown hook event, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "ignored" {
		t.Errorf("status = %q, want %q", result["status"], "ignored")
	}
}

// TestRecordEvent_EmptySessionID_ReturnsValidationError verifies empty session_id is rejected.
func TestRecordEvent_EmptySessionID_ReturnsValidationError(t *testing.T) {
	env := newHandlerTestEnv(t)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	params, _ := json.Marshal(map[string]interface{}{
		"session_id": "",
		"event":      map[string]interface{}{"hook_event_name": "PreToolUse", "tool_name": "Bash"},
	})
	req := Request{ID: "req-empty-sid", Method: "agent.record_event", Params: params}

	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error for empty session_id")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
}

// TestRecordEvent_UnknownSession_ReturnsInternalError verifies that inserting a message for a
// non-existent session_id fails (FK constraint) and returns an internal error.
func TestRecordEvent_UnknownSession_ReturnsInternalError(t *testing.T) {
	env := newHandlerTestEnv(t)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	req := buildRecordEventReq(t, "req-nosess", "nonexistent-session-id", map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "echo hi"},
	})
	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error for nonexistent session_id (FK constraint)")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("error code = %d, want %d (internal)", resp.Error.Code, ErrCodeInternal)
	}
}

// TestRecordEvent_BumpLastMessage_CalledOnPreToolUse verifies BumpLastMessage is called after a
// successful PreToolUse record.
func TestRecordEvent_BumpLastMessage_CalledOnPreToolUse(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-BUMP-1")
	wfiID := queryWFIID(t, env, "RE-BUMP-1")
	sessionID := "sess-re-bump-1"
	insertAgentSession(t, env, "RE-BUMP-1", sessionID, wfiID)

	sig := &bumpRecordSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-bump", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]interface{}{"file_path": "/tmp/foo"},
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if len(sig.bumps) != 1 {
		t.Fatalf("BumpLastMessage call count = %d, want 1", len(sig.bumps))
	}
	if sig.bumps[0] != sessionID {
		t.Errorf("bumped session_id = %q, want %q", sig.bumps[0], sessionID)
	}
}

// TestRecordEvent_BumpLastMessage_CalledOnPostToolUse verifies BumpLastMessage is called after a
// successful PostToolUse record.
func TestRecordEvent_BumpLastMessage_CalledOnPostToolUse(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-BUMP-2")
	wfiID := queryWFIID(t, env, "RE-BUMP-2")
	sessionID := "sess-re-bump-2"
	insertAgentSession(t, env, "RE-BUMP-2", sessionID, wfiID)

	sig := &bumpRecordSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-bump2", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_response":   "done",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if len(sig.bumps) != 1 {
		t.Fatalf("BumpLastMessage call count = %d, want 1", len(sig.bumps))
	}
}

// TestRecordEvent_NilSignaler_NoPanic verifies the handler nil-guards the signaler.
func TestRecordEvent_NilSignaler_NoPanic(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-NILSIG")
	wfiID := queryWFIID(t, env, "RE-NILSIG")
	sessionID := "sess-re-nilsig"
	insertAgentSession(t, env, "RE-NILSIG", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-nilsig", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "echo"},
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Errorf("nil signaler should not cause error, got: %v", resp.Error)
	}
}
