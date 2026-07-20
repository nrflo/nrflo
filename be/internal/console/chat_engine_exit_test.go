package console

import (
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
)

// TestChatService_EngineError_UnpinsTurnState covers the engine-death path a
// turn at a time: an engine that reports an error mid-turn (codex app-server
// EOF, claude's CLI process dying) never sends turn/completed, so without the
// pump ending the turn on EventError the state machine would stay "running"
// and every later SendMessage would be rejected with ErrTurnActive forever.
func TestChatService_EngineError_UnpinsTurnState(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	if err := svc.SendMessage(sid, "first"); err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}
	eng.emit(spawner.EngineEvent{
		Type:      spawner.EventError,
		SessionID: sid,
		Text:      "app-server connection closed",
		IsError:   true,
	})
	waitForChatTurnState(t, ch, "idle", 2*time.Second)

	if err := svc.SendMessage(sid, "retry after error"); err != nil {
		t.Fatalf("SendMessage after engine error = %v, want nil (turn must not stay pinned)", err)
	}
}

// TestChatService_EngineExit_ClosesSessionAndKillsToken covers the whole
// engine dying (Events() closing without a user-initiated Close): the service
// must drop the session and close the row, so the bearer token dies rather
// than staying valid against a session whose engine no longer exists.
func TestChatService_EngineExit_ClosesSessionAndKillsToken(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	if err := svc.SendMessage(sid, "first"); err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}

	// The engine dies on its own — nobody called ChatService.Close.
	eng.Stop()

	// The pump pushes turn=idle only after tearing the session down, so this is
	// the signal that the row close has committed.
	waitForChatTurnState(t, ch, "idle", 2*time.Second)

	if _, ok := svc.get(sid); ok {
		t.Error("session still held by ChatService after its engine exited")
	}
	if err := svc.SendMessage(sid, "after engine death"); err != ErrChatSessionNotFound {
		t.Errorf("SendMessage after engine death = %v, want ErrChatSessionNotFound", err)
	}

	row, err := repo.NewAgentSessionRepo(pool, svc.deps.Clock).GetConsoleChat(sid)
	if err != nil || row == nil {
		t.Fatalf("load row: row=%v err=%v", row, err)
	}
	if row.Status != model.AgentSessionInteractiveCompleted {
		t.Errorf("row status after engine death = %q, want %q (bearer token must die)",
			row.Status, model.AgentSessionInteractiveCompleted)
	}
}

// TestChatService_TurnStarted_PushesRunningState asserts the engine's own
// turn-start event reaches the session channel: a subscriber that did not
// issue the POST /messages (a second tab, the trace view) learns a turn is
// running from this push, not from the REST response.
func TestChatService_TurnStarted_PushesRunningState(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	eng.emit(spawner.EngineEvent{Type: spawner.EventTurnStarted, SessionID: sid})
	waitForChatTurnState(t, ch, "running", 2*time.Second)
}
