package socket

import (
	"database/sql"
	"encoding/json"
	"testing"

	"be/internal/clock"
)

// firstMessagePayload returns the payload column of the session's seq-0 row.
func firstMessagePayload(t *testing.T, env *handlerTestEnv, sessionID string) string {
	t.Helper()
	var payload sql.NullString
	err := env.pool.QueryRow(
		`SELECT payload FROM agent_messages WHERE session_id = ? ORDER BY seq LIMIT 1`, sessionID,
	).Scan(&payload)
	if err != nil {
		t.Fatalf("query payload: %v", err)
	}
	return payload.String
}

func sendHook(t *testing.T, h *Handler, sessionID string, event map[string]interface{}) {
	t.Helper()
	resp := h.Handle(buildRecordEventReq(t, "req-span", sessionID, event))
	if resp.Error != nil {
		t.Fatalf("record_event error: %v", resp.Error)
	}
}

// TestRecordEvent_ToolSpan_PreStoresToolUseID verifies PreToolUse persists
// tool_use_id in the payload column for later span closing.
func TestRecordEvent_ToolSpan_PreStoresToolUseID(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-SPAN-1")
	wfiID := queryWFIID(t, env, "RE-SPAN-1")
	sessionID := "sess-re-span-1"
	insertAgentSession(t, env, "RE-SPAN-1", sessionID, wfiID)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	sendHook(t, h, sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "ls"},
		"tool_use_id":     "toolu_span_1",
	})

	var p map[string]string
	if err := json.Unmarshal([]byte(firstMessagePayload(t, env, sessionID)), &p); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if p["tool_use_id"] != "toolu_span_1" {
		t.Errorf("payload tool_use_id = %q, want toolu_span_1", p["tool_use_id"])
	}
}

// TestRecordEvent_ToolSpan_HiddenToolPostClosesSpan verifies that PostToolUse
// for a hidden-result tool stamps ended_at on the invoke row while still
// suppressing the result row.
func TestRecordEvent_ToolSpan_HiddenToolPostClosesSpan(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-SPAN-2")
	wfiID := queryWFIID(t, env, "RE-SPAN-2")
	sessionID := "sess-re-span-2"
	insertAgentSession(t, env, "RE-SPAN-2", sessionID, wfiID)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	sendHook(t, h, sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "make test"},
		"tool_use_id":     "toolu_span_2",
	})
	sendHook(t, h, sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_response":   "ok",
		"tool_use_id":     "toolu_span_2",
	})

	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1 (hidden result suppressed)", n)
	}
	var p map[string]string
	if err := json.Unmarshal([]byte(firstMessagePayload(t, env, sessionID)), &p); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if p["ended_at"] == "" {
		t.Error("ended_at not stamped on invoke row")
	}
}

// TestRecordEvent_ToolSpan_FailureClosesSpan verifies PostToolUseFailure also
// closes the span and still inserts the failure row.
func TestRecordEvent_ToolSpan_FailureClosesSpan(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-SPAN-3")
	wfiID := queryWFIID(t, env, "RE-SPAN-3")
	sessionID := "sess-re-span-3"
	insertAgentSession(t, env, "RE-SPAN-3", sessionID, wfiID)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	sendHook(t, h, sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "WebFetch",
		"tool_input":      map[string]interface{}{"url": "http://x"},
		"tool_use_id":     "toolu_span_3",
	})
	sendHook(t, h, sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUseFailure",
		"tool_name":       "WebFetch",
		"error":           "connection refused",
		"tool_use_id":     "toolu_span_3",
	})

	if n := countAgentMessages(t, env, sessionID); n != 2 {
		t.Fatalf("agent_messages count = %d, want 2 (invoke + failure)", n)
	}
	var p map[string]string
	if err := json.Unmarshal([]byte(firstMessagePayload(t, env, sessionID)), &p); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if p["ended_at"] == "" {
		t.Error("ended_at not stamped on invoke row after failure")
	}
}

// TestRecordEvent_ToolSpan_UnknownIDIsNoop verifies a PostToolUse with an
// unmatched tool_use_id (old sessions, races) is harmless.
func TestRecordEvent_ToolSpan_UnknownIDIsNoop(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-SPAN-4")
	wfiID := queryWFIID(t, env, "RE-SPAN-4")
	sessionID := "sess-re-span-4"
	insertAgentSession(t, env, "RE-SPAN-4", sessionID, wfiID)
	h := NewHandler(env.pool, env.hub, clock.Real(), nil)

	sendHook(t, h, sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "WebFetch",
		"tool_response":   "late result",
		"tool_use_id":     "toolu_never_seen",
	})
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1 (result row only)", n)
	}
}
