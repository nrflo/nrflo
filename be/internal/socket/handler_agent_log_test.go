package socket

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/ws"
)

// lastAgentMessageWithPayload returns the content, category, and payload (COALESCE to "") of the last row.
func lastAgentMessageWithPayload(t *testing.T, env *handlerTestEnv, sessionID string) (content, category, payload string) {
	t.Helper()
	if err := env.pool.QueryRow(
		`SELECT content, category, COALESCE(payload, '') FROM agent_messages WHERE session_id = ? ORDER BY seq DESC LIMIT 1`,
		sessionID,
	).Scan(&content, &category, &payload); err != nil {
		t.Fatalf("lastAgentMessageWithPayload(%q): %v", sessionID, err)
	}
	return content, category, payload
}

// agentMessagePayloadIsNull returns whether payload IS NULL for the last row.
func agentMessagePayloadIsNull(t *testing.T, env *handlerTestEnv, sessionID string) bool {
	t.Helper()
	var isNull bool
	if err := env.pool.QueryRow(
		`SELECT payload IS NULL FROM agent_messages WHERE session_id = ? ORDER BY seq DESC LIMIT 1`,
		sessionID,
	).Scan(&isNull); err != nil {
		t.Fatalf("agentMessagePayloadIsNull(%q): %v", sessionID, err)
	}
	return isNull
}

// buildAgentLogReq constructs a Request for agent.log.
// msgType="" omits the type field; payload=nil omits the payload field.
func buildAgentLogReq(t *testing.T, id, sessionID, msgType, message string, payload interface{}) Request {
	t.Helper()
	params := map[string]interface{}{
		"session_id": sessionID,
		"message":    message,
	}
	if msgType != "" {
		params["type"] = msgType
	}
	if payload != nil {
		params["payload"] = payload
	}
	p, _ := json.Marshal(params)
	return Request{ID: id, Method: "agent.log", Params: p}
}

// TestAgentLog_ValidToolWithPayload verifies that a tool message with JSON payload inserts
// a row with correct content/category/payload and broadcasts messages.updated.
func TestAgentLog_ValidToolWithPayload(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "AL-TOOL-1")
	wfiID := queryWFIID(t, env, "AL-TOOL-1")
	sessionID := "sess-al-tool-1"
	insertAgentSession(t, env, "AL-TOOL-1", sessionID, wfiID)

	client, sendCh := ws.NewTestClient(env.hub, "al-tool-client")
	env.hub.Subscribe(client, env.project, "AL-TOOL-1")

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildAgentLogReq(t, "req-al-tool", sessionID, "tool", "ran bash", map[string]interface{}{"k": "v"})
	resp := h.Handle(req)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "logged" {
		t.Errorf("status = %q, want %q", result["status"], "logged")
	}

	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	content, category, payload := lastAgentMessageWithPayload(t, env, sessionID)
	if content != "ran bash" {
		t.Errorf("content = %q, want %q", content, "ran bash")
	}
	if category != "tool" {
		t.Errorf("category = %q, want %q", category, "tool")
	}
	if payload != `{"k":"v"}` {
		t.Errorf("payload = %q, want %q", payload, `{"k":"v"}`)
	}

	ev := awaitWSEvent(t, sendCh, ws.EventMessagesUpdated)
	if sid, _ := ev.Data["session_id"].(string); sid != sessionID {
		t.Errorf("broadcast session_id = %q, want %q", sid, sessionID)
	}
	if cat, _ := ev.Data["category"].(string); cat != "tool" {
		t.Errorf("broadcast category = %q, want %q", cat, "tool")
	}
}

// TestAgentLog_EmptyType_DefaultsToText verifies that omitting type results in category="text".
func TestAgentLog_EmptyType_DefaultsToText(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "AL-TEXT-1")
	wfiID := queryWFIID(t, env, "AL-TEXT-1")
	sessionID := "sess-al-text-1"
	insertAgentSession(t, env, "AL-TEXT-1", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildAgentLogReq(t, "req-al-text", sessionID, "", "hello world", nil)

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	_, category, _ := lastAgentMessageWithPayload(t, env, sessionID)
	if category != "text" {
		t.Errorf("category = %q, want %q (empty type must default to text)", category, "text")
	}
}

// TestAgentLog_NullPayload_StoredAsNull verifies that payload:null stores NULL in DB.
func TestAgentLog_NullPayload_StoredAsNull(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "AL-NULL-1")
	wfiID := queryWFIID(t, env, "AL-NULL-1")
	sessionID := "sess-al-null-1"
	insertAgentSession(t, env, "AL-NULL-1", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	params, _ := json.Marshal(map[string]interface{}{
		"session_id": sessionID,
		"message":    "no payload here",
		"payload":    nil,
	})
	req := Request{ID: "req-al-null", Method: "agent.log", Params: params}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	if !agentMessagePayloadIsNull(t, env, sessionID) {
		t.Errorf("payload should be NULL for payload:null input")
	}
}

// TestAgentLog_OmittedPayload_StoredAsNull verifies that omitting payload stores NULL in DB.
func TestAgentLog_OmittedPayload_StoredAsNull(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "AL-NOPAY-1")
	wfiID := queryWFIID(t, env, "AL-NOPAY-1")
	sessionID := "sess-al-nopay-1"
	insertAgentSession(t, env, "AL-NOPAY-1", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	params, _ := json.Marshal(map[string]interface{}{
		"session_id": sessionID,
		"message":    "no payload field",
	})
	req := Request{ID: "req-al-nopay", Method: "agent.log", Params: params}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if !agentMessagePayloadIsNull(t, env, sessionID) {
		t.Errorf("payload should be NULL when payload field is omitted")
	}
}

// TestAgentLog_MissingSessionID_ReturnsValidationError verifies missing session_id is rejected.
func TestAgentLog_MissingSessionID_ReturnsValidationError(t *testing.T) {
	env := newHandlerTestEnv(t)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	params, _ := json.Marshal(map[string]interface{}{"message": "some message"})
	req := Request{ID: "req-al-nosess", Method: "agent.log", Params: params}

	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected validation error for missing session_id")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
	if n := countAgentMessages(t, env, ""); n != 0 {
		t.Errorf("agent_messages count = %d, want 0 on validation error", n)
	}
}

// TestAgentLog_EmptyMessage_ReturnsValidationError verifies empty message is rejected.
func TestAgentLog_EmptyMessage_ReturnsValidationError(t *testing.T) {
	env := newHandlerTestEnv(t)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	params, _ := json.Marshal(map[string]interface{}{
		"session_id": "any-session",
		"message":    "",
	})
	req := Request{ID: "req-al-nomsg", Method: "agent.log", Params: params}

	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected validation error for empty message")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
}

// TestAgentLog_NonExistentSession_ReturnsInternalError verifies FK violation returns internal error.
func TestAgentLog_NonExistentSession_ReturnsInternalError(t *testing.T) {
	env := newHandlerTestEnv(t)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	req := buildAgentLogReq(t, "req-al-nosessid", "nonexistent-session-xyz", "text", "hello", nil)

	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error for nonexistent session_id (FK constraint)")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("error code = %d, want %d (internal)", resp.Error.Code, ErrCodeInternal)
	}
}

// TestAgentLog_ErrorAndResultTypes_Accepted verifies error and result category types are stored verbatim.
func TestAgentLog_ErrorAndResultTypes_Accepted(t *testing.T) {
	cases := []string{"error", "result"}
	for _, msgType := range cases {
		msgType := msgType
		t.Run(msgType, func(t *testing.T) {
			env := newHandlerTestEnv(t)
			ticketID := "AL-TYPE-" + msgType
			env.createTicketAndWorkflow(t, ticketID)
			wfiID := queryWFIID(t, env, ticketID)
			sessionID := "sess-al-type-" + msgType
			insertAgentSession(t, env, ticketID, sessionID, wfiID)

			h := NewHandler(env.pool, env.hub, clock.Real(), nil)
			req := buildAgentLogReq(t, "req-al-type-"+msgType, sessionID, msgType, "content", nil)

			resp := h.Handle(req)
			if resp.Error != nil {
				t.Fatalf("type=%q: expected no error, got: %v", msgType, resp.Error)
			}
			_, category, _ := lastAgentMessageWithPayload(t, env, sessionID)
			if category != msgType {
				t.Errorf("type=%q: category = %q, want %q", msgType, category, msgType)
			}
		})
	}
}

// TestAgentLog_BumpLastMessage_CalledOnSuccess verifies BumpLastMessage fires after a successful log.
func TestAgentLog_BumpLastMessage_CalledOnSuccess(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "AL-BUMP-1")
	wfiID := queryWFIID(t, env, "AL-BUMP-1")
	sessionID := "sess-al-bump-1"
	insertAgentSession(t, env, "AL-BUMP-1", sessionID, wfiID)

	sig := &bumpRecordSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildAgentLogReq(t, "req-al-bump", sessionID, "text", "bump me", nil)

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

// TestAgentLog_NilSignaler_NoPanic verifies nil signaler does not panic.
func TestAgentLog_NilSignaler_NoPanic(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "AL-NILSIG")
	wfiID := queryWFIID(t, env, "AL-NILSIG")
	sessionID := "sess-al-nilsig"
	insertAgentSession(t, env, "AL-NILSIG", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildAgentLogReq(t, "req-al-nilsig", sessionID, "text", "no signaler", nil)

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Errorf("nil signaler should not cause error, got: %v", resp.Error)
	}
}
