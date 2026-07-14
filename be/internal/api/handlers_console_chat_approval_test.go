package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
)

func TestHandleConsoleChatApproval_InvalidDecision_Returns400(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-baddecision")
	adminID := createTestUser(t, s, "chat-admin8@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-baddecision", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatApproval)))
	req := chatApprovalReq(sid, "appr-1", `{"decision":"maybe"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConsoleChatApproval_Allow_MapsToApproveAndPersists(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-approve")
	adminID := createTestUser(t, s, "chat-admin9@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-approve", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatApproval)))
	req := chatApprovalReq(sid, "appr-1", `{"decision":"allow"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}

	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.approvals) != 1 || eng.approvals[0].id != "appr-1" || eng.approvals[0].decision != spawner.ApprovalApprove {
		t.Errorf("engine approvals = %+v, want one call for appr-1/approve", eng.approvals)
	}
}

func TestHandleConsoleChatApproval_Deny_MapsToApprovalDeny(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-deny")
	adminID := createTestUser(t, s, "chat-admin-deny@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-deny", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatApproval)))
	req := chatApprovalReq(sid, "appr-deny-1", `{"decision":"deny"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}

	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.approvals) != 1 || eng.approvals[0].id != "appr-deny-1" || eng.approvals[0].decision != spawner.ApprovalDeny {
		t.Errorf("engine approvals = %+v, want one call for appr-deny-1/deny", eng.approvals)
	}
}

func TestHandleCloseConsoleChat_StopsEngineAndKillsToken(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-close")
	adminID := createTestUser(t, s, "chat-admin10@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-close", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleChat)))
	req := chatCloseReq(sid)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}
	if !eng.isStopped() {
		t.Error("engine.Stop was not called by close")
	}
}

func TestHandleCloseConsoleChat_UnknownSession_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-admin11@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleChat)))
	req := chatCloseReq("no-such-session")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCloseConsoleChat_UnrelatedUser_Returns403(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-403")
	adminID := createTestUser(t, s, "chat-admin12@test.com", model.UserRoleAdmin, false)
	adminCookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-403", adminCookie)

	viewerID := createTestUser(t, s, "chat-viewer1@test.com", model.UserRoleViewer, false)
	viewerCookie := injectSession(t, s, viewerID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleChat)))
	req := chatCloseReq(sid)
	req.AddCookie(viewerCookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCloseConsoleChat_OwnBearer_Returns204(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-ownbearer")
	adminID := createTestUser(t, s, "chat-admin13@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-ownbearer", cookie)

	row, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil || row == nil {
		t.Fatalf("load session: row=%v err=%v", row, err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleChat)))
	req := chatCloseReq(sid)
	req.Header.Set("Authorization", "Bearer "+row.SpawnToken.String)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (own bearer may close itself); body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetConsoleChatMessages_ReturnsHistory(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-history")
	adminID := createTestUser(t, s, "chat-admin14@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-history", cookie)

	eng.simulateAssistantText(sid, "proj-chat-history", "hello there")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChatMessages)))
	req := chatMessagesGetReq(sid)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var body struct {
		SessionID string        `json:"session_id"`
		Messages  []interface{} `json:"messages"`
		Total     int           `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Total != 1 || len(body.Messages) != 1 {
		t.Errorf("history = %+v, want 1 message", body)
	}
}
