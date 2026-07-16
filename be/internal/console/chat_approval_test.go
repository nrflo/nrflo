package console

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
)

// waitForSessionEvent reads client's send channel until an event of wantType
// arrives (ignoring others), or fails the test after timeout.
func waitForSessionEvent(t *testing.T, ch <-chan []byte, wantType string, timeout time.Duration) ws.Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == wantType {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for WS event type %q", wantType)
		}
	}
}

func TestChatService_TextDelta_PushesWSOnly_NoAgentMessageRow(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	client, ch := ws.NewTestClient(hub, "delta-client")
	hub.Register(client)
	hub.SubscribeSession(client, sid)

	eng.emit(spawner.EngineEvent{Type: spawner.EventTextDelta, SessionID: sid, ItemID: "item-1", Text: "partial "})

	ev := waitForSessionEvent(t, ch, ws.EventConsoleChatDelta, 2*time.Second)
	if ev.SessionID != sid {
		t.Errorf("delta event SessionID = %q, want %q", ev.SessionID, sid)
	}
	if ev.Data["text"] != "partial " {
		t.Errorf("delta event text = %v, want %q", ev.Data["text"], "partial ")
	}

	count, err := repo.NewAgentMessageRepo(pool, svc.deps.Clock).CountBySession(sid)
	if err != nil {
		t.Fatalf("CountBySession: %v", err)
	}
	if count != 0 {
		t.Errorf("agent_messages after a text_delta = %d rows, want 0 (deltas are live-only)", count)
	}
}

func TestChatSink_AssistantText_PersistsAndBroadcastsMessagesUpdated(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	client, ch := ws.NewTestClient(hub, "text-client")
	hub.Register(client)
	hub.SubscribeSession(client, sid)

	eng.simulateAssistantText(sid, chatTestProjectID, "hello from the assistant")

	ev := waitForSessionEvent(t, ch, ws.EventMessagesUpdated, 2*time.Second)
	if ev.SessionID != sid {
		t.Errorf("messages.updated SessionID = %q, want %q", ev.SessionID, sid)
	}

	count, err := repo.NewAgentMessageRepo(pool, svc.deps.Clock).CountBySession(sid)
	if err != nil {
		t.Fatalf("CountBySession: %v", err)
	}
	if count != 1 {
		t.Fatalf("agent_messages rows = %d, want 1", count)
	}
}

func TestChatService_ApprovalRequest_ThenReply_ResolvesAndAudits(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	client, ch := ws.NewTestClient(hub, "approval-client")
	hub.Register(client)
	hub.SubscribeSession(client, sid)

	eng.emit(spawner.EngineEvent{
		Type:      spawner.EventApprovalRequest,
		SessionID: sid,
		Approval:  &spawner.ApprovalRequest{ID: "appr-1", Kind: "commandExecution", Command: "rm -rf /tmp/x"},
	})
	reqEv := waitForSessionEvent(t, ch, ws.EventConsoleChatApprovalRequest, 2*time.Second)
	if reqEv.Data["approval_id"] != "appr-1" {
		t.Errorf("approval_request data[approval_id] = %v, want appr-1", reqEv.Data["approval_id"])
	}

	if err := svc.ReplyApproval(sid, "appr-1", spawner.ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	// The engine takes the spawner vocabulary (asserted below), but the push is
	// normalized to what the client speaks: approve -> "allow".
	resolvedEv := waitForSessionEvent(t, ch, ws.EventConsoleChatApprovalResolved, 2*time.Second)
	if resolvedEv.Data["decision"] != "allow" {
		t.Errorf("approval_resolved decision = %v, want allow", resolvedEv.Data["decision"])
	}

	calls := eng.approvalCalls()
	if len(calls) != 1 || calls[0].id != "appr-1" || calls[0].decision != spawner.ApprovalApprove {
		t.Errorf("engine.ReplyApproval calls = %+v, want one call for appr-1/approve", calls)
	}

	entries, total, err := repo.NewAuditRepo(pool, svc.deps.Clock).List(model.AuditFilter{ResourceType: "agent_session", ResourceID: sid}, 1, 100)
	if err != nil {
		t.Fatalf("audit List: %v", err)
	}
	if total == 0 || len(entries) == 0 {
		t.Fatal("expected at least one audit entry for the approval flow")
	}
	var sawRequest, sawResolved bool
	for _, e := range entries {
		switch e.Action {
		case "console_chat.approval_request":
			sawRequest = true
		case "console_chat.approval_resolved":
			sawResolved = true
		}
	}
	if !sawRequest || !sawResolved {
		t.Errorf("audit actions = %+v, want both approval_request and approval_resolved", entries)
	}
}

func TestChatService_ReplyApproval_UnknownSession_ReturnsErrChatSessionNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	err := svc.ReplyApproval("no-such-session", "appr-1", spawner.ApprovalApprove)
	if err != ErrChatSessionNotFound {
		t.Errorf("ReplyApproval(unknown session) = %v, want ErrChatSessionNotFound", err)
	}
}
