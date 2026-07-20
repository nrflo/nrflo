package console

import (
	"time"

	"testing"

	"be/internal/spawner"
	"be/internal/ws"
)

// TestChatService_Thinking_PushesConsoleChatThinking asserts EventThinking has
// no persistence path (codex thinking is never persisted; claude thinking is
// event-only) — pumpChatEvents is the only writer, and it must push
// console_chat.thinking with item_id/text intact.
func TestChatService_Thinking_PushesConsoleChatThinking(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	eng.emit(spawner.EngineEvent{Type: spawner.EventThinking, SessionID: sid, ItemID: "think-1", Text: "considering the approach"})

	ev := waitForSessionEvent(t, ch, ws.EventConsoleChatThinking, 2*time.Second)
	if ev.Data["item_id"] != "think-1" {
		t.Errorf("thinking event item_id = %v, want think-1", ev.Data["item_id"])
	}
	if ev.Data["text"] != "considering the approach" {
		t.Errorf("thinking event text = %v, want %q", ev.Data["text"], "considering the approach")
	}
}

// TestChatService_ApprovalResolved_NormalizesDecisionToClientVocabulary asserts
// the pump translates the spawner's engine-facing vocabulary into the
// allow/deny the REST reply route accepts. Pushing the raw "approve" would make
// an allowed tool render as denied on a client that only knows allow/deny.
func TestChatService_ApprovalResolved_NormalizesDecisionToClientVocabulary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		decision spawner.ApprovalDecision
		want     string
	}{
		{"approve", spawner.ApprovalApprove, "allow"},
		{"approve_for_session", spawner.ApprovalApproveForSession, "allow"},
		{"deny", spawner.ApprovalDeny, "deny"},
		{"abort", spawner.ApprovalAbort, "deny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, hub, factory := newChatTestService(t)

			sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			eng := factory.last()
			ch := subscribeChatSession(t, hub, sid)

			eng.emit(spawner.EngineEvent{
				Type:       spawner.EventApprovalResolved,
				SessionID:  sid,
				ApprovalID: "appr-vocab",
				Decision:   tc.decision,
			})

			ev := waitForSessionEvent(t, ch, ws.EventConsoleChatApprovalResolved, 2*time.Second)
			if got := ev.Data["decision"]; got != tc.want {
				t.Errorf("approval_resolved decision for %s = %v, want %q", tc.decision, got, tc.want)
			}
		})
	}
}

// TestChatService_TokenUsage_PushesAgentContextUpdated asserts EventTokenUsage
// pushes agent.context_updated on the session channel. It is the only path
// covering codex (whose sink deliberately pushes nothing); a claude chat also
// gets this event from the socket fan-out in handler_context.go.
func TestChatService_TokenUsage_PushesAgentContextUpdated(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	eng.emit(spawner.EngineEvent{Type: spawner.EventTokenUsage, SessionID: sid, ContextLeftPct: 77})

	ev := waitForSessionEvent(t, ch, ws.EventAgentContextUpdated, 2*time.Second)
	if ev.SessionID != sid {
		t.Errorf("context_updated SessionID = %q, want %q", ev.SessionID, sid)
	}
	if got := ev.Data["context_left"]; got != float64(77) {
		t.Errorf("context_updated context_left = %v, want 77", got)
	}
}

// TestChatService_ApprovalResolved_RemovesFromSnapshotAndCarriesDecisionReason
// asserts EventApprovalResolved pushes console_chat.approval_resolved with
// decision+reason AND clears the pending approval from Snapshot() — a client
// polling GET .../{sid} after a reload must not see an already-resolved
// approval as still pending.
func TestChatService_ApprovalResolved_RemovesFromSnapshotAndCarriesDecisionReason(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	eng.emit(spawner.EngineEvent{
		Type:      spawner.EventApprovalRequest,
		SessionID: sid,
		Approval:  &spawner.ApprovalRequest{ID: "appr-resolve-1", Kind: "commandExecution", Command: "ls"},
	})
	_ = waitForSessionEvent(t, ch, ws.EventConsoleChatApprovalRequest, 2*time.Second)

	snap, ok := svc.Snapshot(sid)
	if !ok || len(snap.PendingApprovals) != 1 {
		t.Fatalf("Snapshot before resolve = ok=%v pending=%+v, want one pending approval", ok, snap.PendingApprovals)
	}

	eng.emit(spawner.EngineEvent{
		Type:       spawner.EventApprovalResolved,
		SessionID:  sid,
		ApprovalID: "appr-resolve-1",
		Decision:   spawner.ApprovalDeny,
		Text:       "nrflo: approval timed out",
	})

	ev := waitForSessionEvent(t, ch, ws.EventConsoleChatApprovalResolved, 2*time.Second)
	if ev.Data["approval_id"] != "appr-resolve-1" {
		t.Errorf("approval_resolved approval_id = %v, want appr-resolve-1", ev.Data["approval_id"])
	}
	if ev.Data["decision"] != string(spawner.ApprovalDeny) {
		t.Errorf("approval_resolved decision = %v, want %q", ev.Data["decision"], spawner.ApprovalDeny)
	}
	if ev.Data["reason"] != "nrflo: approval timed out" {
		t.Errorf("approval_resolved reason = %v, want %q", ev.Data["reason"], "nrflo: approval timed out")
	}

	snap, ok = svc.Snapshot(sid)
	if !ok {
		t.Fatal("Snapshot after resolve: session no longer live")
	}
	if len(snap.PendingApprovals) != 0 {
		t.Errorf("Snapshot after resolve: pending approvals = %+v, want none (resolved approval must be removed)", snap.PendingApprovals)
	}
}

// TestChatService_ReplyApproval_DoesNotPushDirectly_OnlyTheEngineEventDoes
// asserts ChatService.ReplyApproval itself never resolves/pushes: it only
// forwards to the engine, and the fake engine's own EventApprovalResolved
// (mirroring the real engines' contract) is what a subscriber actually
// observes. If ReplyApproval also pushed, the UI would double-render one
// resolution.
func TestChatService_ReplyApproval_DoesNotPushDirectly_OnlyTheEngineEventDoes(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)

	eng.emit(spawner.EngineEvent{
		Type:      spawner.EventApprovalRequest,
		SessionID: sid,
		Approval:  &spawner.ApprovalRequest{ID: "appr-single-writer", Kind: "commandExecution", Command: "ls"},
	})
	_ = waitForSessionEvent(t, ch, ws.EventConsoleChatApprovalRequest, 2*time.Second)

	if err := svc.ReplyApproval(sid, "appr-single-writer", spawner.ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	// Exactly one approval_resolved push must arrive — from the engine's own
	// EventApprovalResolved (emitted by the fake engine's ReplyApproval, same
	// as the real engines do), not a second one from ChatService itself.
	_ = waitForSessionEvent(t, ch, ws.EventConsoleChatApprovalResolved, 2*time.Second)
	select {
	case raw := <-ch:
		t.Fatalf("unexpected extra WS push after the single approval_resolved: %s", string(raw))
	case <-time.After(150 * time.Millisecond):
		// expected: no second push.
	}
}
