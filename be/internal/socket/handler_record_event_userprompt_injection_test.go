package socket

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeContextInjector is a scripted ContextInjector double: tests configure
// the fixed string it returns and inspect the calls it recorded.
type fakeContextInjector struct {
	injected string
	calls    []fakeInjectorCall
}

type fakeInjectorCall struct {
	sessionID, prompt string
}

func (f *fakeContextInjector) InjectUserPromptContext(_ context.Context, sessionID, prompt string) string {
	f.calls = append(f.calls, fakeInjectorCall{sessionID, prompt})
	return f.injected
}

// TestRecordEvent_UserPromptSubmit_Injector_EngineOwnedPath verifies a wired
// ContextInjector attaches additional_context on the engine-owned
// recorded:false branch (fakeConsoleHooks.userPromptOwn=true) without
// recording a duplicate agent_messages row.
func TestRecordEvent_UserPromptSubmit_Injector_EngineOwnedPath(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "UPS-INJ-OWN")
	wfiID := queryWFIID(t, env, "UPS-INJ-OWN")
	sessionID := "sess-inj-own"
	insertAgentSession(t, env, "UPS-INJ-OWN", sessionID, wfiID)

	env.handler.consoleHooks = &fakeConsoleHooks{userPromptOwn: true}
	injector := &fakeContextInjector{injected: "digest: working set here"}
	env.handler.contextInjector = injector

	req := buildRecordEventReq(t, "req-inj-own", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello there",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["additional_context"] != "digest: working set here" {
		t.Errorf("additional_context = %v, want %q", result["additional_context"], "digest: working set here")
	}
	if result["recorded"] != false {
		t.Errorf("recorded = %v, want false (engine-owned path)", result["recorded"])
	}
	if n := countAgentMessages(t, env, sessionID); n != 0 {
		t.Errorf("agent_messages count = %d, want 0 (engine owns the row)", n)
	}
	if len(injector.calls) != 1 || injector.calls[0].sessionID != sessionID || injector.calls[0].prompt != "hello there" {
		t.Errorf("InjectUserPromptContext calls = %+v, want one call for session=%s prompt=hello there", injector.calls, sessionID)
	}
}

// TestRecordEvent_UserPromptSubmit_Injector_RecordSimpleEventPath verifies a
// wired ContextInjector attaches additional_context on the recordSimpleEvent
// branch (no live console engine — the common case) while still recording
// the user_input row exactly once.
func TestRecordEvent_UserPromptSubmit_Injector_RecordSimpleEventPath(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "UPS-INJ-REC")
	wfiID := queryWFIID(t, env, "UPS-INJ-REC")
	sessionID := "sess-inj-rec"
	insertAgentSession(t, env, "UPS-INJ-REC", sessionID, wfiID)

	injector := &fakeContextInjector{injected: "digest: recorded path"}
	env.handler.contextInjector = injector

	req := buildRecordEventReq(t, "req-inj-rec", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "human typed this",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["additional_context"] != "digest: recorded path" {
		t.Errorf("additional_context = %v, want %q", result["additional_context"], "digest: recorded path")
	}
	if result["status"] != "recorded" {
		t.Errorf("status = %v, want recorded", result["status"])
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Errorf("agent_messages count = %d, want 1", n)
	}
}

// TestRecordEvent_UserPromptSubmit_NilInjector_ByteIdenticalToBaseline
// asserts that a nil contextInjector (the zero value — no wiring needed in
// most tests) produces a response with no additional_context key at all,
// byte-identical to the pre-injector baseline.
func TestRecordEvent_UserPromptSubmit_NilInjector_ByteIdenticalToBaseline(t *testing.T) {
	event := map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello there",
	}

	baseEnv := newHandlerTestEnv(t)
	baseEnv.createTicketAndWorkflow(t, "UPS-NIL-BASE")
	baseWFI := queryWFIID(t, baseEnv, "UPS-NIL-BASE")
	insertAgentSession(t, baseEnv, "UPS-NIL-BASE", "sess-nil-base", baseWFI)
	baseResp := baseEnv.handler.Handle(buildRecordEventReq(t, "req-nil-base", "sess-nil-base", event))

	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "UPS-NIL-1")
	wfiID := queryWFIID(t, env, "UPS-NIL-1")
	sessionID := "sess-nil-1"
	insertAgentSession(t, env, "UPS-NIL-1", sessionID, wfiID)
	// env.handler.contextInjector is nil by default — no wiring.
	resp := env.handler.Handle(buildRecordEventReq(t, "req-nil-1", sessionID, event))

	if baseResp.Error != nil || resp.Error != nil {
		t.Fatalf("unexpected errors: base=%v injected=%v", baseResp.Error, resp.Error)
	}
	if string(resp.Result) != string(baseResp.Result) {
		t.Errorf("response with nil contextInjector differs from baseline:\n got:  %s\n want: %s", resp.Result, baseResp.Result)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, has := raw["additional_context"]; has {
		t.Errorf("nil contextInjector must not add additional_context, got %s", resp.Result)
	}
}

// TestRecordEvent_UserPromptSubmit_EmptyInjection_NoAdditionalContextKey
// verifies an injector that returns "" (no context to attach — the default
// backward-silent state) leaves the response without an additional_context key.
func TestRecordEvent_UserPromptSubmit_EmptyInjection_NoAdditionalContextKey(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "UPS-EMPTY-1")
	wfiID := queryWFIID(t, env, "UPS-EMPTY-1")
	sessionID := "sess-empty-1"
	insertAgentSession(t, env, "UPS-EMPTY-1", sessionID, wfiID)

	env.handler.contextInjector = &fakeContextInjector{injected: ""}

	req := buildRecordEventReq(t, "req-empty-1", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello there",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, has := raw["additional_context"]; has {
		t.Errorf("empty injection must not add additional_context, got %s", resp.Result)
	}
}

// TestAddAdditionalContext_NonObjectResult_ReturnsRespUnchanged covers the
// defensive branch in addAdditionalContext: a Result that doesn't unmarshal
// into a JSON object (e.g. malformed/non-object payload) is returned as-is
// rather than dropped or panicking.
func TestAddAdditionalContext_NonObjectResult_ReturnsRespUnchanged(t *testing.T) {
	resp := MakeResponse("req-1", []int{1, 2, 3}) // Result is a JSON array, not an object
	got := addAdditionalContext("req-1", resp, "some context")
	if string(got.Result) != string(resp.Result) {
		t.Errorf("addAdditionalContext(non-object result) = %s, want unchanged %s", got.Result, resp.Result)
	}
}
