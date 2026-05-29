package socket

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
)

// TestRecordEvent_PostToolUse_RecordsResult verifies PostToolUse inserts a
// "[tool] → output" result row (matching api-mode), bumps stall detection, and
// returns status="recorded".
func TestRecordEvent_PostToolUse_RecordsResult(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-POST-1")
	wfiID := queryWFIID(t, env, "RE-POST-1")
	sessionID := "sess-re-post-1"
	insertAgentSession(t, env, "RE-POST-1", sessionID, wfiID)

	sig := &bumpRecordSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-re-post", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Read",
		"tool_response":   "file content here",
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
	if content != "[Read] → file content here" {
		t.Errorf("content = %q, want %q", content, "[Read] → file content here")
	}
	if category != "tool" {
		t.Errorf("category = %q, want %q", category, "tool")
	}
	if len(sig.bumps) == 0 {
		t.Errorf("BumpLastMessage not called; want at least 1")
	}
}

// TestRecordEvent_PostToolUse_MCPContentArray verifies an MCP-style structured
// tool_response ({content:[{type:text,text:...}]}) renders as the extracted text
// rather than the raw JSON envelope, and the name matches the PreToolUse row.
func TestRecordEvent_PostToolUse_MCPContentArray(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-POST-MCP")
	wfiID := queryWFIID(t, env, "RE-POST-MCP")
	sessionID := "sess-re-post-mcp"
	insertAgentSession(t, env, "RE-POST-MCP", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-re-post-mcp", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "mcp__nrflo__emit_findings",
		"tool_response": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "recorded 2 findings"},
			},
			"isError": false,
		},
	})
	if resp := h.Handle(req); resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	content, category := lastAgentMessage(t, env, sessionID)
	want := "[Mcp__nrflo__emit_findings] → recorded 2 findings"
	if content != want {
		t.Errorf("content = %q, want %q", content, want)
	}
	if category != "tool" {
		t.Errorf("category = %q, want tool", category)
	}
}

// TestRecordEvent_PostToolUse_EmptyResultIgnored verifies an absent/empty
// tool_response inserts no row, does not bump, and acks "ignored".
func TestRecordEvent_PostToolUse_EmptyResultIgnored(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-POST-EMPTY")
	wfiID := queryWFIID(t, env, "RE-POST-EMPTY")
	sessionID := "sess-re-post-empty"
	insertAgentSession(t, env, "RE-POST-EMPTY", sessionID, wfiID)

	sig := &bumpRecordSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-re-post-empty", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Read",
	})

	resp := h.Handle(req)
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "ignored" {
		t.Errorf("status = %q, want %q", result["status"], "ignored")
	}
	if n := countAgentMessages(t, env, sessionID); n != 0 {
		t.Errorf("agent_messages count = %d, want 0", n)
	}
	if len(sig.bumps) != 0 {
		t.Errorf("BumpLastMessage call count = %d, want 0 (no row → no bump)", len(sig.bumps))
	}
}
