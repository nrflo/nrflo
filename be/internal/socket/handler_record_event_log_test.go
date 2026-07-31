package socket

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/logger"
)

// captureRecordEventLog redirects logger output to a fresh buffer for the
// test duration. The original writer is restored via t.Cleanup. Must not be
// used from t.Parallel() tests — it mutates a package-global.
func captureRecordEventLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	orig := logger.GetWriter()
	buf := &bytes.Buffer{}
	logger.SetWriter(buf)
	t.Cleanup(func() {
		logger.SetWriter(orig)
	})
	return buf
}

// TestRecordEvent_LogsOneLinePerEvent_PromptToolStopSequence exercises a full
// UserPromptSubmit -> PreToolUse -> PostToolUse -> Stop sequence and asserts
// exactly one correctly-leveled log line per hook, carrying session_id and
// (for the tool hooks) the tool name.
func TestRecordEvent_LogsOneLinePerEvent_PromptToolStopSequence(t *testing.T) {
	buf := captureRecordEventLog(t)

	env := newHandlerTestEnv(t)
	insertSessionForStop(t, env, "LOG-1", "sess-log-1", "running", "pass")

	promptResp := env.handler.Handle(buildRecordEventReq(t, "req-log-prompt", "sess-log-1", map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "please do the thing",
	}))
	if promptResp.Error != nil {
		t.Fatalf("UserPromptSubmit: unexpected error: %v", promptResp.Error)
	}

	preResp := env.handler.Handle(buildRecordEventReq(t, "req-log-pre", "sess-log-1", map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "echo hi"},
		"tool_use_id":     "tu-log-1",
	}))
	if preResp.Error != nil {
		t.Fatalf("PreToolUse: unexpected error: %v", preResp.Error)
	}

	postResp := env.handler.Handle(buildRecordEventReq(t, "req-log-post", "sess-log-1", map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_response":   "hi",
		"tool_use_id":     "tu-log-1",
	}))
	if postResp.Error != nil {
		t.Fatalf("PostToolUse: unexpected error: %v", postResp.Error)
	}
	var postResult map[string]string
	if err := json.Unmarshal(postResp.Result, &postResult); err != nil {
		t.Fatalf("unmarshal PostToolUse result: %v", err)
	}
	if postResult["status"] != "ignored" {
		t.Errorf("PostToolUse status = %q, want ignored (Bash is a hidden-result tool)", postResult["status"])
	}

	stopResp := callStopHook(t, env, "sess-log-1")
	if block, _ := stopBlocked(t, stopResp); block {
		t.Fatal("expected Stop to allow (result already pass)")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 log lines, got %d:\n%s", len(lines), buf.String())
	}

	checks := []struct {
		line     string
		level    string
		event    string
		wantTool bool
	}{
		{lines[0], "DEBUG", "UserPromptSubmit", false},
		{lines[1], "DEBUG", "PreToolUse", true},
		{lines[2], "DEBUG", "PostToolUse", true},
		{lines[3], "INFO", "Stop", false},
	}
	for _, c := range checks {
		if !strings.Contains(c.line, c.level) {
			t.Errorf("%s line missing %s: %s", c.event, c.level, c.line)
		}
		if !strings.Contains(c.line, c.event) {
			t.Errorf("line missing event name %s: %s", c.event, c.line)
		}
		if !strings.Contains(c.line, "session_id=sess-log-1") {
			t.Errorf("%s line missing session_id=sess-log-1: %s", c.event, c.line)
		}
		if c.wantTool && !strings.Contains(c.line, "tool=Bash") {
			t.Errorf("%s line missing tool=Bash: %s", c.event, c.line)
		}
	}
}

// TestRecordEvent_WarnOnRejectedOrUnknown covers every WARN-logging path:
// malformed params, empty session_id, an unresolvable session (FK error), and
// an unrecognized hook_event_name.
func TestRecordEvent_WarnOnRejectedOrUnknown(t *testing.T) {
	t.Run("malformed params json", func(t *testing.T) {
		buf := captureRecordEventLog(t)
		env := newHandlerTestEnv(t)

		req := Request{ID: "req-bad-params", Method: "agent.record_event", Params: json.RawMessage(`{"session_id":"sess-bad-params","event":"oops"}`)}
		resp := env.handler.Handle(req)
		if resp.Error == nil {
			t.Fatal("expected error for non-object event field")
		}
		if resp.Error.Code != ErrCodeInvalidParams {
			t.Errorf("error code = %d, want %d (invalid params)", resp.Error.Code, ErrCodeInvalidParams)
		}
		if !strings.Contains(buf.String(), "WARN") {
			t.Errorf("expected WARN log line, got:\n%s", buf.String())
		}
	})

	t.Run("empty session_id", func(t *testing.T) {
		buf := captureRecordEventLog(t)
		env := newHandlerTestEnv(t)

		params, _ := json.Marshal(map[string]interface{}{
			"session_id": "",
			"event":      map[string]interface{}{"hook_event_name": "PreToolUse", "tool_name": "Bash"},
		})
		resp := env.handler.Handle(Request{ID: "req-empty-sid", Method: "agent.record_event", Params: params})
		if resp.Error == nil {
			t.Fatal("expected error for empty session_id")
		}
		if resp.Error.Code != ErrCodeValidation {
			t.Errorf("error code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
		}
		if !strings.Contains(buf.String(), "WARN") {
			t.Errorf("expected WARN log line, got:\n%s", buf.String())
		}
	})

	t.Run("unknown session FK error", func(t *testing.T) {
		buf := captureRecordEventLog(t)
		env := newHandlerTestEnv(t)

		req := buildRecordEventReq(t, "req-nosess", "nonexistent-session-id", map[string]interface{}{
			"hook_event_name": "PreToolUse",
			"tool_name":       "Bash",
			"tool_input":      map[string]interface{}{"command": "echo hi"},
		})
		resp := env.handler.Handle(req)
		if resp.Error == nil {
			t.Fatal("expected error for nonexistent session_id")
		}
		if resp.Error.Code != ErrCodeInternal {
			t.Errorf("error code = %d, want %d (internal)", resp.Error.Code, ErrCodeInternal)
		}
		out := buf.String()
		if !strings.Contains(out, "WARN") {
			t.Errorf("expected WARN log line, got:\n%s", out)
		}
		if !strings.Contains(out, "nonexistent-session-id") {
			t.Errorf("expected WARN line to carry the session id, got:\n%s", out)
		}
	})

	t.Run("unknown hook event name", func(t *testing.T) {
		buf := captureRecordEventLog(t)
		env := newHandlerTestEnv(t)

		req := buildRecordEventReq(t, "req-unk", "any-session", map[string]interface{}{
			"hook_event_name": "Foobar",
		})
		resp := env.handler.Handle(req)
		if resp.Error != nil {
			t.Fatalf("expected no error for unknown hook event, got: %v", resp.Error)
		}
		var result map[string]string
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if result["status"] != "ignored" {
			t.Errorf("status = %q, want ignored", result["status"])
		}
		out := buf.String()
		if !strings.Contains(out, "WARN") {
			t.Errorf("expected WARN log line for unknown hook event, got:\n%s", out)
		}
		if !strings.Contains(out, "unknown hook event") {
			t.Errorf("expected WARN reason 'unknown hook event', got:\n%s", out)
		}
	})
}

// TestRecordEvent_LogLine_NeverDumpsPromptPayload guards against a payload
// dump: a UserPromptSubmit's prompt text (however long or sensitive) must
// never appear verbatim in the log line, only its length.
func TestRecordEvent_LogLine_NeverDumpsPromptPayload(t *testing.T) {
	buf := captureRecordEventLog(t)

	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "LOG-NODUMP-1")
	wfiID := queryWFIID(t, env, "LOG-NODUMP-1")
	sessionID := "sess-log-nodump-1"
	insertAgentSession(t, env, "LOG-NODUMP-1", sessionID, wfiID)

	sentinel := "SENTINEL-PROMPT-CONTENT-DO-NOT-LOG-9f3c1a"
	resp := env.handler.Handle(buildRecordEventReq(t, "req-nodump", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          sentinel,
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	out := buf.String()
	if strings.Contains(out, sentinel) {
		t.Errorf("log line must not contain the raw prompt text, got:\n%s", out)
	}
	if !strings.Contains(out, "prompt_len=") {
		t.Errorf("expected prompt_len detail in log line, got:\n%s", out)
	}
}
