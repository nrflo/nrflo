package console

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"be/internal/spawner"
	"be/internal/ws"
)

// subscribeChatSession attaches a WS test client to sid's session channel and
// returns the channel its pushes land on.
func subscribeChatSession(t *testing.T, hub *ws.Hub, sid string) <-chan []byte {
	t.Helper()
	client, ch := ws.NewTestClient(hub, "chat-test")
	hub.Register(client)
	hub.SubscribeSession(client, sid)
	return ch
}

// waitForChatTurnState blocks until a console_chat.turn event with the wanted
// state arrives on ch. The pump flips the turn state machine BEFORE pushing
// this event, so receiving it is the signal that the async pumpChatEvents
// goroutine has caught up — no polling, no sleep (root CLAUDE.md rule 4).
func waitForChatTurnState(t *testing.T, ch <-chan []byte, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == ws.EventConsoleChatTurn && ev.Data["state"] == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for console_chat.turn state=%q", want)
		}
	}
}

func TestChatService_SendMessage_UnknownSession_ReturnsErrChatSessionNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	if err := svc.SendMessage("no-such-session", "hi"); err != ErrChatSessionNotFound {
		t.Errorf("SendMessage(unknown) = %v, want ErrChatSessionNotFound", err)
	}
}

// TestChatService_SendMessage_SecondCallWhileTurnActive_RejectsWithoutEngineRoundTrip
// exercises acceptance case 2: a second POST /messages while a turn is in
// flight must be rejected deterministically (spawner.ErrTurnActive) without
// ever reaching the engine a second time. The fake engine never emits
// EventTurnCompleted here, so the turn stays "running" for the whole test.
func TestChatService_SendMessage_SecondCallWhileTurnActive_RejectsWithoutEngineRoundTrip(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	if err := svc.SendMessage(sid, "first turn"); err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}
	if got := eng.turnCount(); got != 1 {
		t.Fatalf("engine turn count after first message = %d, want 1", got)
	}

	err = svc.SendMessage(sid, "second turn")
	if !errors.Is(err, spawner.ErrTurnActive) {
		t.Fatalf("second SendMessage while active = %v, want spawner.ErrTurnActive", err)
	}
	if got := eng.turnCount(); got != 1 {
		t.Errorf("engine turn count after rejected second message = %d, want still 1 (no round trip)", got)
	}
}

// TestChatService_SendMessage_TurnCompleted_AllowsNextMessage verifies the
// state machine returns to idle once the engine reports EventTurnCompleted,
// so a subsequent SendMessage succeeds.
func TestChatService_SendMessage_TurnCompleted_AllowsNextMessage(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	if err := svc.SendMessage(sid, "first"); err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}
	eng.emit(spawner.EngineEvent{Type: spawner.EventTurnCompleted, SessionID: sid})
	waitForChatTurnState(t, ch, "idle", 2*time.Second)

	if err := svc.SendMessage(sid, "second"); err != nil {
		t.Fatalf("second SendMessage after turn completed: %v", err)
	}
	if got := eng.turnCount(); got != 2 {
		t.Errorf("engine turn count after second message = %d, want 2", got)
	}
}

// TestChatService_SendMessage_EngineError_RollsBackTurnState verifies that
// when SendUserTurn itself fails, the local turn state is rolled back to idle
// (via sess.endTurn in the SendMessage error path) so a retry is not
// permanently blocked.
func TestChatService_SendMessage_EngineError_RollsBackTurnState(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	wantErr := errors.New("transport failure")
	eng.setSendErr(wantErr)

	err = svc.SendMessage(sid, "will fail")
	if !errors.Is(err, wantErr) {
		t.Fatalf("SendMessage = %v, want %v", err, wantErr)
	}

	// Turn state must have rolled back to idle; a retry should now succeed.
	if err := svc.SendMessage(sid, "retry"); err != nil {
		t.Fatalf("retry SendMessage after engine error = %v, want nil (turn state must roll back)", err)
	}
}
