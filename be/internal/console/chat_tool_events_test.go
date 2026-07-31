package console

import (
	"testing"
	"time"

	"be/internal/spawner"
	"be/internal/ws"
)

// TestChatService_ToolEvents_PushToolStartedFinished verifies the pump maps
// EventToolInvoke/EventToolResult onto console_chat.tool_started/tool_finished
// session pushes carrying the tool name and the error flag.
func TestChatService_ToolEvents_PushToolStartedFinished(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	client, ch := ws.NewTestClient(hub, "tool-events-client")
	hub.Register(client)
	hub.SubscribeSession(client, sid)

	eng.emit(spawner.EngineEvent{
		Type: spawner.EventToolInvoke, SessionID: sid,
		ToolName: "Bash", ToolInput: map[string]any{"command": "make test"},
	})
	started := waitForSessionEvent(t, ch, ws.EventConsoleChatToolStarted, 2*time.Second)
	if started.Data["tool"] != "Bash" {
		t.Errorf("tool_started tool = %v, want Bash", started.Data["tool"])
	}
	if _, hasDetail := started.Data["detail"]; hasDetail {
		t.Error("tool_started carries detail, want tool name only")
	}

	eng.emit(spawner.EngineEvent{
		Type: spawner.EventToolResult, SessionID: sid, ToolName: "Bash", IsError: true,
	})
	finished := waitForSessionEvent(t, ch, ws.EventConsoleChatToolFinished, 2*time.Second)
	if finished.Data["tool"] != "Bash" {
		t.Errorf("tool_finished tool = %v, want Bash", finished.Data["tool"])
	}
	if finished.Data["is_error"] != true {
		t.Errorf("tool_finished is_error = %v, want true", finished.Data["is_error"])
	}
}
