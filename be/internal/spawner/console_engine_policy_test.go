package spawner

import (
	"encoding/json"
	"testing"
)

// TestThreadStartParams_AutonomousLiterals pins the wire shape produced for
// the autonomous codexAppServerBackend's call site (codex_appserver_backend.go:138),
// which passes sandbox="danger-full-access" approvalPolicy="never" — this must
// stay byte-for-byte unchanged now that threadStartParams is shared with the
// console engine.
func TestThreadStartParams_AutonomousLiterals(t *testing.T) {
	params := threadStartParams("gpt-5-codex", "/work", "danger-full-access", "never")
	want := map[string]any{
		"model":          "gpt-5-codex",
		"cwd":            "/work",
		"sandbox":        "danger-full-access",
		"approvalPolicy": "never",
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("threadStartParams()[%q] = %v, want %v", k, params[k], v)
		}
	}
}

// TestThreadStartParams_ConsoleEngineDefaults pins the console engine's
// default wire shape (sandbox=workspace-write, approvalPolicy=on-request),
// distinct from the autonomous backend's never/danger-full-access.
func TestThreadStartParams_ConsoleEngineDefaults(t *testing.T) {
	params := threadStartParams("gpt-5-codex", "/work", "workspace-write", "on-request")
	if params["sandbox"] != "workspace-write" || params["approvalPolicy"] != "on-request" {
		t.Errorf("threadStartParams() = %+v, want sandbox=workspace-write approvalPolicy=on-request", params)
	}
}

// TestDispatchAppServerEvent_NilEmitter_NoRowsHeartbeatOnly is the
// AUTONOMOUS UNCHANGED acceptance test: with emit==nil (the autonomous spawn
// path), item/agentMessage/delta records zero Sink rows and only bumps the
// heartbeat — dispatchAppServerEvent's behavior must be byte-for-byte
// unchanged from before EngineEvent existed.
func TestDispatchAppServerEvent_NilEmitter_NoRowsHeartbeatOnly(t *testing.T) {
	sink := &testSink{}
	env := rpcEnvelope{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"itemId":"m1","delta":"hi"}`)}
	dispatchAppServerEvent("s", env, sink, 200000, nil)

	if len(sink.recordedMsgs) != 0 {
		t.Errorf("nil-emitter delta recorded %d Sink rows, want 0: %+v", len(sink.recordedMsgs), sink.recordedMsgs)
	}
	if sink.bumpCount != 1 {
		t.Errorf("nil-emitter delta bumpCount = %d, want 1", sink.bumpCount)
	}
}

// TestDispatchAppServerEvent_NilEmitter_ReasoningDelta covers the other delta
// branch (item/reasoning/textDelta) that newly calls into an emit helper —
// with nil it must still be a pure heartbeat bump, no panic, no Sink row.
func TestDispatchAppServerEvent_NilEmitter_ReasoningDelta(t *testing.T) {
	sink := &testSink{}
	env := rpcEnvelope{Method: "item/reasoning/textDelta", Params: json.RawMessage(`{"itemId":"r1","delta":"thinking"}`)}
	dispatchAppServerEvent("s", env, sink, 200000, nil)

	if len(sink.recordedMsgs) != 0 {
		t.Errorf("nil-emitter reasoning delta recorded %d Sink rows, want 0", len(sink.recordedMsgs))
	}
	if sink.bumpCount != 1 {
		t.Errorf("nil-emitter reasoning delta bumpCount = %d, want 1", sink.bumpCount)
	}
}

// TestGetConsoleEngine_Codex asserts the one provider switch returns a codex
// engine for "codex".
func TestGetConsoleEngine_Codex(t *testing.T) {
	eng, err := GetConsoleEngine("codex", EngineDeps{Sink: &testSink{}})
	if err != nil {
		t.Fatalf("GetConsoleEngine(codex): %v", err)
	}
	if eng.Name() != "codex" {
		t.Errorf("Name() = %q, want codex", eng.Name())
	}
	if _, ok := eng.(*codexEngine); !ok {
		t.Errorf("GetConsoleEngine(codex) returned %T, want *codexEngine", eng)
	}
}

// TestGetConsoleEngine_Claude asserts the same provider switch returns a
// claude engine for "claude" (console-7's addition).
func TestGetConsoleEngine_Claude(t *testing.T) {
	eng, err := GetConsoleEngine("claude", EngineDeps{Sink: &testSink{}})
	if err != nil {
		t.Fatalf("GetConsoleEngine(claude): %v", err)
	}
	if eng.Name() != "claude" {
		t.Errorf("Name() = %q, want claude", eng.Name())
	}
	if _, ok := eng.(*claudeEngine); !ok {
		t.Errorf("GetConsoleEngine(claude) returned %T, want *claudeEngine", eng)
	}
}

// TestGetConsoleEngine_Unknown asserts an unregistered engine name errors
// instead of returning a nil/zero-value engine.
func TestGetConsoleEngine_Unknown(t *testing.T) {
	eng, err := GetConsoleEngine("nonexistent", EngineDeps{Sink: &testSink{}})
	if err == nil {
		t.Error("expected an error for an unknown console engine name")
	}
	if eng != nil {
		t.Errorf("expected nil engine on error, got %+v", eng)
	}
}
