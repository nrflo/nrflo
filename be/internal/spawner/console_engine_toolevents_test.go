package spawner

import (
	"encoding/json"
	"testing"
)

// TestClaudeEngine_NotifyToolResult_EmitsEventToolResult verifies the
// PostToolUse hub notification surfaces as EventToolResult on the engine
// event channel.
func TestClaudeEngine_NotifyToolResult_EmitsEventToolResult(t *testing.T) {
	e := &claudeEngine{events: make(chan EngineEvent, 4), stopping: make(chan struct{})}
	e.spec.SessionID = "sess-1"

	e.NotifyToolResult("Bash", true)

	select {
	case ev := <-e.events:
		if ev.Type != EventToolResult || ev.ToolName != "Bash" || !ev.IsError || ev.SessionID != "sess-1" {
			t.Errorf("event = %+v, want EventToolResult Bash isError sess-1", ev)
		}
	default:
		t.Fatal("no event emitted")
	}
}

// TestAPIEngineStream_ToolHooks verifies OnToolStart emits EventToolInvoke
// with the parsed input and OnToolEnd pairs the id back to its name on
// EventToolResult.
func TestAPIEngineStream_ToolHooks(t *testing.T) {
	e := newAPIConsoleEngine(EngineDeps{})
	e.spec.SessionID = "sess-api-1"
	s := &apiEngineStream{e: e}

	s.OnToolStart("tu-1", "web_search", json.RawMessage(`{"query":"nrflo"}`))
	ev := <-e.events
	if ev.Type != EventToolInvoke || ev.ToolName != "web_search" {
		t.Fatalf("start event = %+v, want EventToolInvoke web_search", ev)
	}
	if ev.ToolInput["query"] != "nrflo" {
		t.Errorf("start event input = %+v, want parsed query", ev.ToolInput)
	}

	s.OnToolEnd("tu-1", true)
	ev = <-e.events
	if ev.Type != EventToolResult || ev.ToolName != "web_search" || !ev.IsError {
		t.Fatalf("end event = %+v, want EventToolResult web_search isError", ev)
	}
	if len(s.toolNames) != 0 {
		t.Errorf("toolNames after end = %+v, want drained", s.toolNames)
	}
}
