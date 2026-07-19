package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/spawner"
	"be/internal/ws"
)

func getChatReq(sid string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/console/chats/"+sid, nil)
	r.SetPathValue("sid", sid)
	return r
}

// waitForWSEventType blocks until an event of wantType arrives on ch,
// ignoring others, or fails the test after timeout.
func waitForWSEventType(t *testing.T, ch <-chan []byte, wantType string, timeout time.Duration) {
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
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for WS event type %q", wantType)
		}
	}
}

// TestHandleGetConsoleChat_KindGuard_ConsoleAndWorkflowAgentSidsReturn404
// asserts the {sid} detail route reuses the kind='console_chat' guard: a
// kind='console' or kind='workflow_agent' session id must 404, never be
// misread as a chat.
func TestHandleGetConsoleChat_KindGuard_ConsoleAndWorkflowAgentSidsReturn404(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-detail-kind")
	adminID := createTestUser(t, s, "chat-detail-kind-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	seedWorkflowAgentForConsoleTest(t, s, "proj-chat-detail-kind", "wf-agent-detail")

	consoleChain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	consoleReq := createConsoleReq("proj-chat-detail-kind")
	consoleReq.AddCookie(cookie)
	consoleRR := httptest.NewRecorder()
	consoleChain.ServeHTTP(consoleRR, consoleReq)
	if consoleRR.Code != http.StatusCreated {
		t.Fatalf("create console session status = %d, want 201; body=%s", consoleRR.Code, consoleRR.Body.String())
	}
	var consoleBody map[string]string
	if err := json.Unmarshal(consoleRR.Body.Bytes(), &consoleBody); err != nil {
		t.Fatalf("unmarshal console session response: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	for _, sid := range []string{consoleBody["session_id"], "wf-agent-detail"} {
		req := getChatReq(sid)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		chain.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("sid=%q status = %d, want 404; body=%s", sid, rr.Code, rr.Body.String())
		}
	}
}

func TestHandleGetConsoleChat_UnknownSid_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-detail-unknown-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	req := getChatReq("no-such-chat-session")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetConsoleChat_NonAuthorizedUser_Returns403(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-detail-403")
	adminID := createTestUser(t, s, "chat-detail-403-admin@test.com", model.UserRoleAdmin, false)
	adminCookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-detail-403", adminCookie)

	viewerID := createTestUser(t, s, "chat-detail-403-viewer@test.com", model.UserRoleViewer, false)
	viewerCookie := injectSession(t, s, viewerID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	req := getChatReq(sid)
	req.AddCookie(viewerCookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleGetConsoleChat_Detail_ReturnsRunningTurnAndPendingApproval drives
// a real engine event through to the ChatService snapshot and asserts the
// detail route surfaces turn=running plus the pending approval — this is what
// lets a reloaded page restore an in-flight approval card and the turn
// spinner instead of losing them.
func TestHandleGetConsoleChat_Detail_ReturnsRunningTurnAndPendingApproval(t *testing.T) {
	s, factory := newChatTestServer(t)
	startTestHub(t, s)
	seedConsoleProject(t, s, "proj-chat-detail-live")
	adminID := createTestUser(t, s, "chat-detail-live-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-detail-live", cookie)

	client, ch := ws.NewTestClient(s.wsHub, "detail-live-sub")
	s.wsHub.Register(client)
	s.wsHub.SubscribeSession(client, sid)

	// The turn state machine is flipped by ChatService.SendMessage (via
	// chatSession.beginTurn), not by the engine's own EventTurnStarted push —
	// so a real POST /messages is what puts Snapshot().Turn into "running".
	msgChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatMessage)))
	msgReq := chatMessageReq(sid, `{"text":"do the thing"}`)
	msgReq.AddCookie(cookie)
	msgRR := httptest.NewRecorder()
	msgChain.ServeHTTP(msgRR, msgReq)
	if msgRR.Code != http.StatusAccepted {
		t.Fatalf("POST messages status = %d, want 202; body=%s", msgRR.Code, msgRR.Body.String())
	}

	eng.emit(spawner.EngineEvent{
		Type:      spawner.EventApprovalRequest,
		SessionID: sid,
		Approval:  &spawner.ApprovalRequest{ID: "appr-detail-1", Kind: "commandExecution", Command: "rm -rf /tmp/x", Cwd: "/work", Reason: "destructive"},
	})
	waitForWSEventType(t, ch, ws.EventConsoleChatApprovalRequest, 2*time.Second)
	eng.emit(spawner.EngineEvent{Type: spawner.EventTextDelta, SessionID: sid, ItemID: "answer-1", Text: "recoverable partial"})
	waitForWSEventType(t, ch, ws.EventConsoleChatDelta, 2*time.Second)
	eng.emit(spawner.EngineEvent{Type: spawner.EventThinking, SessionID: sid, ItemID: "think-1", Text: "considering"})
	waitForWSEventType(t, ch, ws.EventConsoleChatThinking, 2*time.Second)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	req := getChatReq(sid)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var body struct {
		Live             bool                      `json:"live"`
		Turn             string                    `json:"turn"`
		PendingApprovals []consoleChatApprovalItem `json:"pending_approvals"`
		LiveItems        []map[string]string       `json:"live_items"`
		Thinking         map[string]string         `json:"thinking"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.Live {
		t.Fatal("Live = false, want true")
	}
	if body.Turn != "running" {
		t.Errorf("Turn = %q, want running", body.Turn)
	}
	if len(body.PendingApprovals) != 1 || body.PendingApprovals[0].ApprovalID != "appr-detail-1" {
		t.Fatalf("PendingApprovals = %+v, want one entry for appr-detail-1", body.PendingApprovals)
	}
	if body.PendingApprovals[0].Command != "rm -rf /tmp/x" {
		t.Errorf("PendingApprovals[0].Command = %q, want %q", body.PendingApprovals[0].Command, "rm -rf /tmp/x")
	}
	if len(body.LiveItems) != 1 || body.LiveItems[0]["text"] != "recoverable partial" {
		t.Fatalf("LiveItems = %+v", body.LiveItems)
	}
	if body.Thinking["text"] != "considering" {
		t.Fatalf("Thinking = %+v", body.Thinking)
	}
}

// TestHandleGetConsoleChat_CostEstimate_LiveAndAfterEngineExit is the full
// flow for the ticket's console surface: create a real chat session (which
// registers RegisterSessionCost against the request's model, exactly as
// ChatService.Create does in production), feed usage through the same
// spawner.AddSessionCostUsage entry point a real engine's usage hook calls,
// then assert the detail route surfaces the live in-memory cost — and, after
// the engine exits (FinalizeSessionCost flushes to the DB row), that the same
// value is still readable via the raw-query fallback (lastFlushedCostEstimate).
func TestHandleGetConsoleChat_CostEstimate_LiveAndAfterEngineExit(t *testing.T) {
	s, factory := newChatTestServer(t)
	startTestHub(t, s)
	seedConsoleProject(t, s, "proj-chat-cost")
	adminID := createTestUser(t, s, "chat-cost-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-cost", `{"engine":"codex","model":"gpt-5.6-sol"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create chat status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var createBody map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("unmarshal create chat response: %v", err)
	}
	sid := createBody["session_id"]
	eng := factory.last()

	// gpt-5.6-sol: price_in=5, price_out=30 per MTok (migration 000183 seed).
	spawner.AddSessionCostUsage(sid, 1_000_000, 200_000, 0, 0)
	wantCost := 1_000_000.0/1e6*5 + 200_000.0/1e6*30

	getChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	liveReq := getChatReq(sid)
	liveReq.AddCookie(cookie)
	liveRR := httptest.NewRecorder()
	getChain.ServeHTTP(liveRR, liveReq)
	if liveRR.Code != http.StatusOK {
		t.Fatalf("get chat (live) status = %d, want 200; body=%s", liveRR.Code, liveRR.Body.String())
	}
	var liveBody struct {
		CostEstimate float64 `json:"cost_estimate"`
	}
	if err := json.Unmarshal(liveRR.Body.Bytes(), &liveBody); err != nil {
		t.Fatalf("unmarshal live body: %v", err)
	}
	if diff := liveBody.CostEstimate - wantCost; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("live cost_estimate = %v, want %v", liveBody.CostEstimate, wantCost)
	}

	// Tear the engine down: engineExited calls FinalizeSessionCost, forcing an
	// immediate DB flush of the last snapshot before dropping the in-memory entry.
	waitForChatEngineExit(t, s, sid, eng)

	afterReq := getChatReq(sid)
	afterReq.AddCookie(cookie)
	afterRR := httptest.NewRecorder()
	getChain.ServeHTTP(afterRR, afterReq)
	if afterRR.Code != http.StatusOK {
		t.Fatalf("get chat (after exit) status = %d, want 200; body=%s", afterRR.Code, afterRR.Body.String())
	}
	var afterBody struct {
		CostEstimate float64 `json:"cost_estimate"`
	}
	if err := json.Unmarshal(afterRR.Body.Bytes(), &afterBody); err != nil {
		t.Fatalf("unmarshal after-exit body: %v", err)
	}
	if diff := afterBody.CostEstimate - wantCost; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("after-exit cost_estimate (raw DB fallback) = %v, want %v", afterBody.CostEstimate, wantCost)
	}
}
