package console

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
)

// TestChatT0Decider_ContextStaysUnderBudget_AcrossManyTurns drives >=20 turns
// through the real EventTurnCompleted->pumpChatEvents->maybeRotate path with
// a pre-folded digest present: whenever reported usage crosses the profile's
// 50k budget (well under opus-4-8's 200k window, so the pct-of-window
// default ceiling never governs — ProactiveRestartConsoleThreshold caps at
// budget), the session rotates in place and resets to 0 tokens used, so
// currentTokens() is always observed under the 50k budget between turns.
func TestChatT0Decider_ContextStaysUnderBudget_AcrossManyTurns(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newT0DeciderTestService(t, nil)
	if err := pool.SetConfig("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("SetConfig min interval: %v", err)
	}

	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create(t0-decider): %v", err)
	}
	digestRepo := repo.NewRefineryDigestRepo(pool, svc.deps.Clock)
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "carried-forward T0 working set"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}

	ch := subscribeChatSession(t, hub, sid)
	const budget = 50000
	for i := 0; i < 20; i++ {
		sess, ok := svc.get(sid)
		if !ok {
			t.Fatalf("turn %d: session missing", i)
		}
		if _, err := svc.SendMessage(sid, "continue"); err != nil {
			t.Fatalf("turn %d: SendMessage: %v", i, err)
		}
		eng := factory.last()

		// Simulate steadily climbing usage: contextLeftPct steps down each
		// turn so currentTokens grows toward (and past) the 50k budget.
		pctLeft := 100 - ((i%10)+1)*8 // ranges from 92 down to 12, cycling
		eng.emit(spawner.EngineEvent{Type: spawner.EventTokenUsage, SessionID: sid, ContextLeftPct: pctLeft})
		eng.emit(spawner.EngineEvent{Type: spawner.EventTurnCompleted, SessionID: sid})

		// pumpChatEvents processes the boundary (possibly rotating)
		// asynchronously; a rotated turn pushes console.context_rotated
		// instead of console_chat.turn state=idle (chat_events.go), so wait
		// for either — whichever arrives proves the boundary is fully
		// processed before the next SendMessage/factory.last() read.
		waitForIdleOrRotated(t, ch, 2*time.Second)

		if tokens, ok := sess.currentTokens(); ok && tokens > budget {
			t.Fatalf("turn %d: currentTokens=%d exceeds budget=%d", i, tokens, budget)
		}
	}

	factory.mu.Lock()
	rotations := len(factory.engines) - 1
	factory.mu.Unlock()
	if rotations < 1 {
		t.Error("expected at least one rotation across 20 turns of climbing usage, got 0")
	}
	drainEvents(ch)
}

// waitForIdleOrRotated blocks until either a console_chat.turn state=idle or
// a console.context_rotated event arrives — the two mutually-exclusive ways
// pumpChatEvents signals it has finished processing one EventTurnCompleted
// boundary (chat_events.go: a rotated turn skips the idle push entirely).
func waitForIdleOrRotated(t *testing.T, ch <-chan []byte, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == ws.EventConsoleContextRotated {
				return
			}
			if ev.Type == ws.EventConsoleChatTurn && ev.Data["state"] == "idle" {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for console_chat.turn idle or console.context_rotated")
		}
	}
}

// drainEvents empties ch without blocking, so a test's WS subscription
// doesn't leak a goroutine writing to a channel nobody reads anymore.
func drainEvents(ch <-chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
