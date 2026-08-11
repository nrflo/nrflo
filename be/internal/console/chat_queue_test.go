package console

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"be/internal/spawner"
	"be/internal/ws"
)

// waitForChatQueued blocks until a console_chat.queued event with the wanted
// count arrives on ch.
func waitForChatQueued(t *testing.T, ch <-chan []byte, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == ws.EventConsoleChatQueued {
				if count, ok := ev.Data["count"].(float64); ok && int(count) == want {
					return
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for console_chat.queued count=%d", want)
		}
	}
}

// Prompts queued mid-turn are delivered as the NEXT turn once the engine
// reports EventTurnCompleted, joined in submission order.
func TestChatService_QueuedPrompts_FlushOnTurnCompleted(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	eng.setSteerUnsupported(true) // pin the queue path; steering is covered in chat_turn_test.go
	ch := subscribeChatSession(t, hub, sid)

	if _, err := svc.SendMessage(sid, "first"); err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}
	for _, text := range []string{"queued one", "queued two"} {
		queued, err := svc.SendMessage(sid, text)
		if err != nil || !queued {
			t.Fatalf("SendMessage(%q) = (%v, %v), want (true, nil)", text, queued, err)
		}
	}
	waitForChatQueued(t, ch, 2, 2*time.Second)

	eng.emit(spawner.EngineEvent{Type: spawner.EventTurnCompleted, SessionID: sid})
	// The flush goroutine pushes the emptied queue AFTER dispatching the turn.
	waitForChatQueued(t, ch, 0, 2*time.Second)

	turns := eng.turnTexts()
	if len(turns) != 2 || turns[1] != "queued one\n\nqueued two" {
		t.Errorf("engine turns = %q, want [first, queued one\\n\\nqueued two]", turns)
	}
	snap, _ := svc.Snapshot(sid)
	if len(snap.QueuedPrompts) != 0 {
		t.Errorf("queued prompts after flush = %v, want empty", snap.QueuedPrompts)
	}
	if snap.Turn != "running" {
		t.Errorf("turn after flush = %q, want running (the flushed prompt started a turn)", snap.Turn)
	}
}

// A turn that ends in EventError never flushes; the leftovers ride ahead of
// the user's next message instead of being lost.
func TestChatService_QueuedPrompts_FoldIntoNextMessageAfterError(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	eng.setSteerUnsupported(true) // pin the queue path; steering is covered in chat_turn_test.go
	ch := subscribeChatSession(t, hub, sid)

	if _, err := svc.SendMessage(sid, "first"); err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}
	if queued, err := svc.SendMessage(sid, "stranded"); err != nil || !queued {
		t.Fatalf("mid-turn SendMessage = (%v, %v), want (true, nil)", queued, err)
	}

	eng.emit(spawner.EngineEvent{Type: spawner.EventError, SessionID: sid, Text: "boom", IsError: true})
	waitForChatTurnState(t, ch, "idle", 2*time.Second)

	// EventError carries no flush; the queue drains into the next message.
	if queued, err := svc.SendMessage(sid, "new message"); err != nil || queued {
		t.Fatalf("SendMessage after error = (%v, %v), want (false, nil)", queued, err)
	}
	turns := eng.turnTexts()
	last := turns[len(turns)-1]
	if last != "stranded\n\nnew message" {
		t.Errorf("last turn = %q, want stranded folded ahead of the new text", last)
	}
}

func TestChatSession_EnqueuePrompt_CapEnforced(t *testing.T) {
	t.Parallel()
	sess := newChatSession("s", "p", "codex", "", "", "", "", "", 0, nil)
	for i := 0; i < maxQueuedPrompts; i++ {
		if !sess.enqueuePrompt("x") {
			t.Fatalf("enqueue %d rejected below cap", i)
		}
	}
	if sess.enqueuePrompt("overflow") {
		t.Error("enqueue past cap must report false")
	}
}

// AnswerQuestion routes to the engine and the pump resolves the pending
// approval with decision "answer" on the session channel.
func TestChatService_AnswerQuestion_ResolvesPendingApproval(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	eng.setSteerUnsupported(true) // pin the queue path; steering is covered in chat_turn_test.go
	ch := subscribeChatSession(t, hub, sid)

	eng.emit(spawner.EngineEvent{
		Type:      spawner.EventApprovalRequest,
		SessionID: sid,
		Approval: &spawner.ApprovalRequest{
			ID:   "q-1",
			Kind: "PreToolUse",
			Tool: spawner.AskUserQuestionTool,
			Raw:  json.RawMessage(`{"questions":[{"question":"pick?","options":[{"label":"A"}]}]}`),
		},
	})
	waitForApprovalRequest(t, ch, "q-1", 2*time.Second)

	if err := svc.AnswerQuestion(sid, "q-1", "A — because"); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	calls := eng.answerCalls()
	if len(calls) != 1 || calls[0].id != "q-1" || calls[0].answer != "A — because" {
		t.Fatalf("engine answer calls = %v, want [(q-1, A — because)]", calls)
	}
	waitForApprovalResolved(t, ch, "q-1", "answer", 2*time.Second)

	if err := svc.AnswerQuestion("nope", "q-1", "x"); !errors.Is(err, ErrChatSessionNotFound) {
		t.Errorf("AnswerQuestion(unknown session) = %v, want ErrChatSessionNotFound", err)
	}
}

// waitForApprovalRequest blocks until console_chat.approval_request for id
// arrives, asserting the question payload fields ride along.
func waitForApprovalRequest(t *testing.T, ch <-chan []byte, id string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == ws.EventConsoleChatApprovalRequest && ev.Data["approval_id"] == id {
				if tool, _ := ev.Data["tool"].(string); tool != spawner.AskUserQuestionTool {
					t.Errorf("approval_request tool = %v, want AskUserQuestion", ev.Data["tool"])
				}
				if input, _ := ev.Data["input"].(string); input == "" {
					t.Error("approval_request input must carry the questions JSON")
				}
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for approval_request %s", id)
		}
	}
}

// waitForApprovalResolved blocks until console_chat.approval_resolved for id
// arrives with the wanted decision.
func waitForApprovalResolved(t *testing.T, ch <-chan []byte, id, decision string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == ws.EventConsoleChatApprovalResolved && ev.Data["approval_id"] == id {
				if got, _ := ev.Data["decision"].(string); got != decision {
					t.Errorf("approval_resolved decision = %q, want %q", got, decision)
				}
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for approval_resolved %s", id)
		}
	}
}
