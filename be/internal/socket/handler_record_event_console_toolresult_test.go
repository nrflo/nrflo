package socket

import "testing"

type toolResultCall struct {
	sessionID, toolName string
	isError             bool
}

func (f *fakeConsoleHooks) ConsoleToolResult(sessionID, toolName string, isError bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toolResultCalls = append(f.toolResultCalls, toolResultCall{sessionID: sessionID, toolName: toolName, isError: isError})
	return true
}

// TestRecordEvent_PostToolUse_NotifiesConsoleToolResult verifies both
// PostToolUse (isError=false, including hidden-result tools like Bash whose
// row is suppressed) and PostToolUseFailure (isError=true) reach
// ConsoleHooks.ConsoleToolResult.
func TestRecordEvent_PostToolUse_NotifiesConsoleToolResult(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "CONSOLE-POST-1")
	wfiID := queryWFIID(t, env, "CONSOLE-POST-1")
	sessionID := "sess-console-post-1"
	insertAgentSession(t, env, "CONSOLE-POST-1", sessionID, wfiID)

	fake := &fakeConsoleHooks{}
	env.handler.consoleHooks = fake

	success := buildRecordEventReq(t, "req-post-ok", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_response":   "done",
	})
	if resp := env.handler.Handle(success); resp.Error != nil {
		t.Fatalf("PostToolUse: %v", resp.Error)
	}
	failure := buildRecordEventReq(t, "req-post-fail", sessionID, map[string]interface{}{
		"hook_event_name": "PostToolUseFailure",
		"tool_name":       "WebFetch",
		"error":           "boom",
	})
	if resp := env.handler.Handle(failure); resp.Error != nil {
		t.Fatalf("PostToolUseFailure: %v", resp.Error)
	}

	fake.mu.Lock()
	calls := append([]toolResultCall(nil), fake.toolResultCalls...)
	fake.mu.Unlock()
	want := []toolResultCall{
		{sessionID: sessionID, toolName: "Bash", isError: false},
		{sessionID: sessionID, toolName: "WebFetch", isError: true},
	}
	if len(calls) != len(want) {
		t.Fatalf("ConsoleToolResult calls = %+v, want %+v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call[%d] = %+v, want %+v", i, calls[i], want[i])
		}
	}
}
