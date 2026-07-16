package socket

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// fakeConsoleHooks is a scripted ConsoleHooks double: tests configure the
// decision/handled values it returns and inspect the calls it recorded.
type fakeConsoleHooks struct {
	mu sync.Mutex

	approveDecision string
	approveReason   string
	approveHandled  bool
	approveCalls    []approveCall

	turnEndHandled bool
	turnEndCalls   []string

	sessionReadyHandled bool
	sessionReadyCalls   []string

	contextLeftHandled bool
	contextLeftCalls   []contextLeftCall

	userPromptOwn   bool
	userPromptCalls []string
}

type approveCall struct {
	sessionID, toolName, toolUseID string
	toolInput                      map[string]any
}

type contextLeftCall struct {
	sessionID string
	pct       int
}

func (f *fakeConsoleHooks) ApproveConsoleTool(_ context.Context, sessionID, toolName string, toolInput map[string]any, toolUseID string) (string, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approveCalls = append(f.approveCalls, approveCall{sessionID, toolName, toolUseID, toolInput})
	return f.approveDecision, f.approveReason, f.approveHandled
}

func (f *fakeConsoleHooks) ConsoleTurnEnd(sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turnEndCalls = append(f.turnEndCalls, sessionID)
	return f.turnEndHandled
}

func (f *fakeConsoleHooks) ConsoleSessionReady(sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessionReadyCalls = append(f.sessionReadyCalls, sessionID)
	return f.sessionReadyHandled
}

func (f *fakeConsoleHooks) ConsoleContextLeft(sessionID string, pct int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contextLeftCalls = append(f.contextLeftCalls, contextLeftCall{sessionID, pct})
	return f.contextLeftHandled
}

func (f *fakeConsoleHooks) ConsoleUserPrompt(sessionID, prompt string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userPromptCalls = append(f.userPromptCalls, prompt)
	return f.userPromptOwn
}

// TestRecordEvent_PreToolUse_ConsoleApprovalHandled_AddsPermissionDecision
// verifies a handled=true console approval both records the tool row (the
// autonomous recorder runs unconditionally first) and adds permission_decision
// to the response.
func TestRecordEvent_PreToolUse_ConsoleApprovalHandled_AddsPermissionDecision(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "CONSOLE-PRE-1")
	wfiID := queryWFIID(t, env, "CONSOLE-PRE-1")
	sessionID := "sess-console-pre-1"
	insertAgentSession(t, env, "CONSOLE-PRE-1", sessionID, wfiID)

	fake := &fakeConsoleHooks{approveHandled: true, approveDecision: "allow", approveReason: "human ok"}
	env.handler.consoleHooks = fake

	req := buildRecordEventReq(t, "req-console-pre-1", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "ls"},
		"tool_use_id":     "tu-1",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	var result struct {
		Status             string `json:"status"`
		PermissionDecision *struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		} `json:"permission_decision"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Status != "recorded" {
		t.Errorf("status = %q, want recorded", result.Status)
	}
	if result.PermissionDecision == nil || result.PermissionDecision.Decision != "allow" || result.PermissionDecision.Reason != "human ok" {
		t.Errorf("permission_decision = %+v, want {allow human ok}", result.PermissionDecision)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Errorf("agent_messages count = %d, want 1 (tool row must still be recorded)", n)
	}

	fake.mu.Lock()
	calls := fake.approveCalls
	fake.mu.Unlock()
	if len(calls) != 1 || calls[0].sessionID != sessionID || calls[0].toolName != "Bash" || calls[0].toolUseID != "tu-1" {
		t.Errorf("ApproveConsoleTool call = %+v, want session=%s tool=Bash toolUseID=tu-1", calls, sessionID)
	}
}

// TestRecordEvent_PreToolUse_ConsoleHooksUnhandled_RegressionByteIdentical
// asserts that a handled=false response (the common autonomous-session case)
// is byte-identical to the nil-consoleHooks baseline — a regression guard for
// handler_record_event_test.go's existing behavior.
func TestRecordEvent_PreToolUse_ConsoleHooksUnhandled_RegressionByteIdentical(t *testing.T) {
	event := map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "pwd"},
	}

	baseEnv := newHandlerTestEnv(t)
	baseEnv.createTicketAndWorkflow(t, "CONSOLE-UNH-BASE")
	baseWFI := queryWFIID(t, baseEnv, "CONSOLE-UNH-BASE")
	insertAgentSession(t, baseEnv, "CONSOLE-UNH-BASE", "sess-unh-base", baseWFI)
	baseResp := baseEnv.handler.Handle(buildRecordEventReq(t, "req-unh-base", "sess-unh-base", event))

	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "CONSOLE-UNH-1")
	wfiID := queryWFIID(t, env, "CONSOLE-UNH-1")
	sessionID := "sess-unh-1"
	insertAgentSession(t, env, "CONSOLE-UNH-1", sessionID, wfiID)
	fake := &fakeConsoleHooks{approveHandled: false}
	env.handler.consoleHooks = fake
	resp := env.handler.Handle(buildRecordEventReq(t, "req-unh-1", sessionID, event))

	if baseResp.Error != nil || resp.Error != nil {
		t.Fatalf("unexpected errors: base=%v console=%v", baseResp.Error, resp.Error)
	}
	if string(resp.Result) != string(baseResp.Result) {
		t.Errorf("response with unhandled consoleHooks differs from the nil-consoleHooks baseline:\n got:  %s\n want: %s", resp.Result, baseResp.Result)
	}

	fake.mu.Lock()
	calls := len(fake.approveCalls)
	fake.mu.Unlock()
	if calls != 1 {
		t.Errorf("ApproveConsoleTool call count = %d, want 1 (must still be consulted even though unhandled)", calls)
	}
}

// TestRecordEvent_PreToolUse_NilConsoleHooks_NoPermissionDecision covers the
// plain nil-consoleHooks case explicitly (env.handler.consoleHooks is nil by
// default — no permission_decision key at all).
func TestRecordEvent_PreToolUse_NilConsoleHooks_NoPermissionDecision(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "CONSOLE-NIL-1")
	wfiID := queryWFIID(t, env, "CONSOLE-NIL-1")
	sessionID := "sess-console-nil-1"
	insertAgentSession(t, env, "CONSOLE-NIL-1", sessionID, wfiID)

	req := buildRecordEventReq(t, "req-console-nil-1", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "echo hi"},
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, has := raw["permission_decision"]; has {
		t.Errorf("nil consoleHooks must not add permission_decision, got %s", resp.Result)
	}
}

// TestRecordEvent_Stop_NotifiesConsoleHubWithoutChangingStopDecision verifies
// the Stop-hook path calls ConsoleTurnEnd while the stop_decision response
// shape stays exactly what handler_stop_test.go already expects.
func TestRecordEvent_Stop_NotifiesConsoleHubWithoutChangingStopDecision(t *testing.T) {
	env := newHandlerTestEnv(t)
	insertSessionForStop(t, env, "CONSOLE-STOP-1", "sess-console-stop-1", "running", "pass")

	fake := &fakeConsoleHooks{turnEndHandled: true}
	env.handler.consoleHooks = fake

	resp := callStopHook(t, env, "sess-console-stop-1")
	block, _ := stopBlocked(t, resp)
	if block {
		t.Fatal("expected allow (result already set) — console wiring must not change stop_decision behavior")
	}

	fake.mu.Lock()
	calls := append([]string(nil), fake.turnEndCalls...)
	fake.mu.Unlock()
	if len(calls) != 1 || calls[0] != "sess-console-stop-1" {
		t.Errorf("ConsoleTurnEnd calls = %v, want [sess-console-stop-1]", calls)
	}
}

// TestRecordEvent_SessionStart_NotifiesConsoleHub verifies SessionStart still
// returns status=ready (SignalSessionReady behavior unchanged) and additionally
// calls ConsoleSessionReady.
func TestRecordEvent_SessionStart_NotifiesConsoleHub(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "CONSOLE-READY-1")
	wfiID := queryWFIID(t, env, "CONSOLE-READY-1")
	sessionID := "sess-console-ready-1"
	insertAgentSession(t, env, "CONSOLE-READY-1", sessionID, wfiID)

	fake := &fakeConsoleHooks{sessionReadyHandled: true}
	env.handler.consoleHooks = fake

	req := buildRecordEventReq(t, "req-ready-1", sessionID, map[string]interface{}{
		"hook_event_name": "SessionStart",
		"source":          "startup",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "ready" {
		t.Errorf("status = %q, want ready", result["status"])
	}

	fake.mu.Lock()
	calls := append([]string(nil), fake.sessionReadyCalls...)
	fake.mu.Unlock()
	if len(calls) != 1 || calls[0] != sessionID {
		t.Errorf("ConsoleSessionReady calls = %v, want [%s]", calls, sessionID)
	}
}

// TestRecordEvent_UserPromptSubmit_ConsoleSessionLive_SkipsUserInputRow
// verifies the hook echo of a console user turn is NOT persisted when a live
// console engine owns the session (SendUserTurn already wrote the user_input
// row), and IS persisted for sessions with no live engine.
func TestRecordEvent_UserPromptSubmit_ConsoleSessionLive_SkipsUserInputRow(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "CONSOLE-UPS-1")
	wfiID := queryWFIID(t, env, "CONSOLE-UPS-1")
	sessionID := "sess-console-ups-1"
	insertAgentSession(t, env, "CONSOLE-UPS-1", sessionID, wfiID)

	fake := &fakeConsoleHooks{userPromptOwn: true}
	env.handler.consoleHooks = fake

	req := buildRecordEventReq(t, "req-console-ups-1", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello there",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 0 {
		t.Errorf("agent_messages count = %d, want 0 (engine owns the user_input row)", n)
	}

	fake.mu.Lock()
	fake.userPromptOwn = false
	fake.mu.Unlock()
	resp = env.handler.Handle(buildRecordEventReq(t, "req-console-ups-2", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello again",
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Errorf("agent_messages count = %d, want 1 (no live engine -> hook records)", n)
	}
}
