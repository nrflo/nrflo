package console

import (
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// TestChatSink_UpdateContextLeft_PersistsWithoutItsOwnBroadcast exercises
// chatSink's direct spawner.Sink implementation (called by a real engine on a
// context hook, never through pumpChatEvents): it must persist context_left
// on the agent_sessions row but must NOT push a WS event itself — that push
// is owned solely by pumpChatEvents' EventTokenUsage case (chat_events.go),
// the only path that covers both engines. A push here would double-fire for
// codex, the only engine that reaches this sink.
func TestChatSink_UpdateContextLeft_PersistsWithoutItsOwnBroadcast(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	client, ch := ws.NewTestClient(hub, "ctx-client")
	hub.Register(client)
	hub.SubscribeSession(client, sid)

	projectID, ticketID, workflow, err := eng.sink.UpdateContextLeft(sid, 42)
	if err != nil {
		t.Fatalf("UpdateContextLeft: %v", err)
	}
	if projectID != chatTestProjectID {
		t.Errorf("projectID = %q, want %q", projectID, chatTestProjectID)
	}
	if ticketID != "" || workflow != "" {
		t.Errorf("ticketID/workflow = %q/%q, want empty (chat sessions are unbound)", ticketID, workflow)
	}

	row, err := repo.NewAgentSessionRepo(pool, svc.deps.Clock).GetConsoleChat(sid)
	if err != nil {
		t.Fatalf("GetConsoleChat: %v", err)
	}
	if !row.ContextLeft.Valid || row.ContextLeft.Int64 != 42 {
		t.Errorf("row.ContextLeft = %+v, want valid 42", row.ContextLeft)
	}

	select {
	case msg := <-ch:
		t.Fatalf("chatSink.UpdateContextLeft must not push its own WS event, got: %s", string(msg))
	case <-time.After(150 * time.Millisecond):
		// expected: no push from the sink itself.
	}
}

// TestChatSink_RecordError_NilErrorSvc_NoOp verifies the documented no-op
// branch when ChatDeps.ErrorSvc is unset (the default in newChatTestService).
func TestChatSink_RecordError_NilErrorSvc_NoOp(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	// Must not panic despite ErrorSvc being nil.
	eng.sink.RecordError(chatTestProjectID, "tool_error", sid, "boom")
}

// TestChatSink_RecordError_WithErrorSvc_Persists wires a real ErrorService
// into ChatDeps and verifies chatSink.RecordError forwards to it.
func TestChatSink_RecordError_WithErrorSvc_Persists(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)
	svc.deps.ErrorSvc = service.NewErrorService(pool, svc.deps.Clock, hub)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	eng.sink.RecordError(chatTestProjectID, "tool_error", sid, "something broke")

	errRepo := repo.NewErrorLogRepo(pool, svc.deps.Clock)
	entries, err := errRepo.List(chatTestProjectID, "", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("error log entries = %d, want 1", len(entries))
	}
	if entries[0].Message != "something broke" {
		t.Errorf("Message = %q, want %q", entries[0].Message, "something broke")
	}
	if entries[0].InstanceID != sid {
		t.Errorf("InstanceID = %q, want %q", entries[0].InstanceID, sid)
	}
}
