package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
)

// waitForChatWSEvent reads client's send channel until an event of wantType
// arrives (ignoring others), or fails the test after timeout.
func waitForChatWSEvent(t *testing.T, ch <-chan []byte, wantType string, timeout time.Duration) ws.Event {
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

// TestConsoleChat_FullLoop_E2E exercises acceptance case 1: create -> message
// -> live delta (WS only, never agent_messages) -> assistant text persisted +
// messages.updated -> GET history -> approval request/resolve -> close kills
// the bearer (401 on /console/tools) and stops the engine.
func TestConsoleChat_FullLoop_E2E(t *testing.T) {
	s, factory := newChatTestServer(t)
	go s.wsHub.Run()
	t.Cleanup(s.wsHub.Stop)

	seedConsoleProject(t, s, "proj-chat-e2e")
	adminID := createTestUser(t, s, "chat-e2e-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	sid, eng := createChatSession(t, s, factory, "proj-chat-e2e", cookie)

	row, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil || row == nil {
		t.Fatalf("load session: row=%v err=%v", row, err)
	}
	token := row.SpawnToken.String

	client, ch := ws.NewTestClient(s.wsHub, "e2e-client")
	s.wsHub.Register(client)
	s.wsHub.SubscribeSession(client, sid)

	// POST /messages
	msgChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatMessage)))
	msgReq := chatMessageReq(sid, `{"text":"hello"}`)
	msgReq.AddCookie(cookie)
	msgRR := httptest.NewRecorder()
	msgChain.ServeHTTP(msgRR, msgReq)
	if msgRR.Code != http.StatusAccepted {
		t.Fatalf("POST messages status = %d, want 202; body=%s", msgRR.Code, msgRR.Body.String())
	}

	// Live delta: WS only, never agent_messages.
	eng.emit(spawner.EngineEvent{Type: spawner.EventTextDelta, SessionID: sid, Text: "partial "})
	deltaEv := waitForChatWSEvent(t, ch, ws.EventConsoleChatDelta, 2*time.Second)
	if deltaEv.Data["text"] != "partial " {
		t.Errorf("delta text = %v, want %q", deltaEv.Data["text"], "partial ")
	}
	if count, err := repo.NewAgentMessageRepo(s.pool, s.clock).CountBySession(sid); err != nil || count != 0 {
		t.Fatalf("agent_messages after delta: count=%d err=%v, want 0", count, err)
	}

	// Sink-level assistant text: persisted + messages.updated pushed.
	eng.simulateAssistantText(sid, "proj-chat-e2e", "hello there")
	waitForChatWSEvent(t, ch, ws.EventMessagesUpdated, 2*time.Second)
	if count, err := repo.NewAgentMessageRepo(s.pool, s.clock).CountBySession(sid); err != nil || count != 1 {
		t.Fatalf("agent_messages after assistant text: count=%d err=%v, want 1", count, err)
	}

	// GET /messages returns history.
	histChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChatMessages)))
	histReq := chatMessagesGetReq(sid)
	histReq.AddCookie(cookie)
	histRR := httptest.NewRecorder()
	histChain.ServeHTTP(histRR, histReq)
	if histRR.Code != http.StatusOK {
		t.Fatalf("GET messages status = %d, want 200; body=%s", histRR.Code, histRR.Body.String())
	}
	var hist struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(histRR.Body.Bytes(), &hist); err != nil {
		t.Fatalf("unmarshal history: %v", err)
	}
	if hist.Total != 1 {
		t.Fatalf("history total = %d, want 1", hist.Total)
	}

	// Approval request pushed, then resolved via allow.
	eng.emit(spawner.EngineEvent{
		Type: spawner.EventApprovalRequest, SessionID: sid,
		Approval: &spawner.ApprovalRequest{ID: "appr-e2e", Kind: "commandExecution", Command: "ls"},
	})
	reqEv := waitForChatWSEvent(t, ch, ws.EventConsoleChatApprovalRequest, 2*time.Second)
	if reqEv.Data["approval_id"] != "appr-e2e" {
		t.Errorf("approval_request approval_id = %v, want appr-e2e", reqEv.Data["approval_id"])
	}

	apprChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatApproval)))
	apprReq := chatApprovalReq(sid, "appr-e2e", `{"decision":"allow"}`)
	apprReq.AddCookie(cookie)
	apprRR := httptest.NewRecorder()
	apprChain.ServeHTTP(apprRR, apprReq)
	if apprRR.Code != http.StatusNoContent {
		t.Fatalf("POST approval status = %d, want 204; body=%s", apprRR.Code, apprRR.Body.String())
	}
	// The resolution comes back in the same vocabulary the request went out in:
	// the route accepts allow/deny, so the WS push says "allow" — not the
	// engine-facing spawner value ("approve"), which no client understands.
	resolvedEv := waitForChatWSEvent(t, ch, ws.EventConsoleChatApprovalResolved, 2*time.Second)
	if resolvedEv.Data["decision"] != "allow" {
		t.Errorf("approval_resolved decision = %v, want allow", resolvedEv.Data["decision"])
	}

	// Bearer works against /console/tools while the session is alive.
	toolsChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	beforeRR := httptest.NewRecorder()
	toolsChain.ServeHTTP(beforeRR, catalogueReq(token))
	if beforeRR.Code != http.StatusOK {
		t.Fatalf("GET /console/tools before close = %d, want 200; body=%s", beforeRR.Code, beforeRR.Body.String())
	}

	// Close: engine.Stop called, token dies.
	closeChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleChat)))
	closeReq := chatCloseReq(sid)
	closeReq.AddCookie(cookie)
	closeRR := httptest.NewRecorder()
	closeChain.ServeHTTP(closeRR, closeReq)
	if closeRR.Code != http.StatusNoContent {
		t.Fatalf("POST close status = %d, want 204; body=%s", closeRR.Code, closeRR.Body.String())
	}
	if !eng.isStopped() {
		t.Error("engine.Stop was not called by close")
	}

	afterRR := httptest.NewRecorder()
	toolsChain.ServeHTTP(afterRR, catalogueReq(token))
	if afterRR.Code != http.StatusUnauthorized {
		t.Fatalf("GET /console/tools after close = %d, want 401 (token must die on close); body=%s", afterRR.Code, afterRR.Body.String())
	}

	closedRow, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil {
		t.Fatalf("GetConsoleChat after close: %v", err)
	}
	if closedRow.Status != model.AgentSessionInteractiveCompleted {
		t.Errorf("Status after close = %q, want interactive_completed", closedRow.Status)
	}
}
