package spawner

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestClaudeEngine_SendUserTurn_WaitsForHookBeforePersistAndStart(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{SessionID: "sess-ack"})
	e.turnAckTimeout = time.Second
	e.NotifySessionReady()

	done := make(chan error, 1)
	go func() {
		done <- e.SendUserTurn(context.Background(), UserTurn{Text: "ack me"})
	}()
	sess := mgr.sessions[e.spec.SessionID]
	waitForPTYText(t, sess, "ack me\r")

	if n := countCategory(sink, "user_input"); n != 0 {
		t.Fatalf("user_input rows before hook = %d, want 0", n)
	}
	if err := e.SteerUserTurn(context.Background(), UserTurn{Text: "too early"}); err != ErrNoActiveTurn {
		t.Fatalf("SteerUserTurn before acknowledgement = %v, want ErrNoActiveTurn", err)
	}
	select {
	case ev := <-e.Events():
		t.Fatalf("event before hook = %+v, want none", ev)
	default:
	}
	if own := e.NotifyUserPrompt("ack me"); !own {
		t.Fatal("UserPromptSubmit echo was not claimed")
	}
	if err := <-done; err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	if n := countCategory(sink, "user_input"); n != 1 {
		t.Fatalf("user_input rows after hook = %d, want 1", n)
	}
	waitForEventType(t, e.Events(), EventTurnStarted, time.Second)
}

func TestClaudeEngine_SendUserTurn_MissingHookRetriesThenRollsBack(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{SessionID: "sess-no-ack"})
	e.turnAckTimeout = time.Millisecond
	e.NotifySessionReady()

	err := e.SendUserTurn(context.Background(), UserTurn{Text: "lost turn"})
	if err == nil || !strings.Contains(err.Error(), "did not acknowledge") {
		t.Fatalf("SendUserTurn error = %v, want acknowledgement failure", err)
	}
	sess := mgr.sessions[e.spec.SessionID]
	sess.mu.Lock()
	written := append([]byte(nil), sess.writtenBytes...)
	sess.mu.Unlock()
	if got := strings.Count(string(written), "\r"); got != maxPromptSubmits {
		t.Errorf("submit CR count = %d, want %d", got, maxPromptSubmits)
	}
	if !strings.Contains(string(written), string([]byte{0x1b, 0x15})) {
		t.Error("retry did not close the palette and clear abandoned input")
	}
	if n := countCategory(sink, "user_input"); n != 0 {
		t.Errorf("user_input rows = %d, want 0 for an unaccepted turn", n)
	}
	e.mu.Lock()
	active := e.turnActive
	e.mu.Unlock()
	if active {
		t.Error("turn remained active after acknowledgement retries exhausted")
	}
	if err := e.SendUserTurn(context.Background(), UserTurn{Text: "next turn"}); err == ErrTurnActive {
		t.Error("next turn rejected as active after delivery rollback")
	}
}

func waitForPTYText(t *testing.T, sess *mockPtySession, want string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		sess.mu.Lock()
		got := string(sess.writtenBytes)
		sess.mu.Unlock()
		if strings.Contains(got, want) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("PTY bytes = %q, want to contain %q", got, want)
		default:
		}
	}
}
