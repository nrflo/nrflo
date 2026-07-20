package spawner

import (
	"context"
	"testing"
	"time"
)

// TestConsoleHub_ConsoleTurnEnd_EmitsTurnCompletedAndReopensSendUserTurn
// covers the Stop-hook delivery path: ConsoleHub.ConsoleTurnEnd (as called
// from socket's Stop case) must flush the tail, emit turn_completed, clear
// turnActive, and call Sink.OnTurnComplete.
func TestConsoleHub_ConsoleTurnEnd_EmitsTurnCompletedAndReopensSendUserTurn(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})
	e.NotifySessionReady()

	if err := e.SendUserTurn(context.Background(), UserTurn{Text: "first turn"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	if err := e.SendUserTurn(context.Background(), UserTurn{Text: "blocked"}); err != ErrTurnActive {
		t.Fatalf("expected ErrTurnActive while a turn is live, got %v", err)
	}

	if handled := hub.ConsoleTurnEnd(e.spec.SessionID); !handled {
		t.Fatal("ConsoleTurnEnd handled = false, want true for a registered session")
	}
	ev := waitForEventType(t, e.Events(), EventTurnCompleted, time.Second)
	if ev.SessionID != e.spec.SessionID {
		t.Errorf("turn_completed session = %q, want %q", ev.SessionID, e.spec.SessionID)
	}
	if sink.turnCompletes != 1 {
		t.Errorf("Sink.OnTurnComplete calls = %d, want 1", sink.turnCompletes)
	}

	if err := e.SendUserTurn(context.Background(), UserTurn{Text: "second turn"}); err != nil {
		t.Errorf("SendUserTurn after ConsoleTurnEnd should succeed (turn re-opened), got %v", err)
	}
}

func TestConsoleHub_ConsoleTurnEnd_UnknownSession_ReturnsFalse(t *testing.T) {
	hub := NewConsoleHub()
	if handled := hub.ConsoleTurnEnd("no-such-session"); handled {
		t.Error("ConsoleTurnEnd on an unregistered session should return handled=false")
	}
}

// TestConsoleHub_ConsoleSessionReady_UnblocksSendUserTurn covers the
// SessionStart-hook delivery path.
func TestConsoleHub_ConsoleSessionReady_UnblocksSendUserTurn(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, mgr := startTestClaudeEngine(t, sink, hub, EngineSpec{})
	e.sessionStartTimeout = 2 * time.Second // long enough that only the hook, not the timeout, can unblock in time

	done := make(chan error, 1)
	go func() { done <- e.SendUserTurn(context.Background(), UserTurn{Text: "hi"}) }()

	if handled := hub.ConsoleSessionReady(e.spec.SessionID); !handled {
		t.Fatal("ConsoleSessionReady handled = false, want true")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendUserTurn: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SendUserTurn did not unblock after ConsoleSessionReady")
	}
	sess := mgr.sessions[e.spec.SessionID]
	if got := string(sess.writtenBytes); got != "hi\r" {
		t.Errorf("PTY bytes = %q, want %q", got, "hi\r")
	}
}

func TestConsoleHub_ConsoleSessionReady_UnknownSession_ReturnsFalse(t *testing.T) {
	hub := NewConsoleHub()
	if handled := hub.ConsoleSessionReady("no-such-session"); handled {
		t.Error("ConsoleSessionReady on an unregistered session should return handled=false")
	}
}

func TestConsoleHub_ConsoleContextLeft_ForwardsTokenUsageEvent(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})

	if handled := hub.ConsoleContextLeft(e.spec.SessionID, 42); !handled {
		t.Fatal("ConsoleContextLeft handled = false, want true")
	}
	ev := waitForEventType(t, e.Events(), EventTokenUsage, time.Second)
	if ev.ContextLeftPct != 42 {
		t.Errorf("token_usage ContextLeftPct = %d, want 42", ev.ContextLeftPct)
	}
}

func TestConsoleHub_ConsoleContextLeft_UnknownSession_ReturnsFalse(t *testing.T) {
	hub := NewConsoleHub()
	if handled := hub.ConsoleContextLeft("no-such-session", 10); handled {
		t.Error("ConsoleContextLeft on an unregistered session should return handled=false")
	}
}

func TestConsoleHub_ApproveConsoleTool_UnknownSession_ReturnsFalse(t *testing.T) {
	hub := NewConsoleHub()
	decision, reason, handled := hub.ApproveConsoleTool(context.Background(), "no-such-session", "Bash", nil, "tu-x")
	if handled {
		t.Errorf("ApproveConsoleTool on an unregistered session should return handled=false, got decision=%q reason=%q", decision, reason)
	}
}
