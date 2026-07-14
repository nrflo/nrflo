package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
)

// newChatTestServer builds a Server (real auth stack) with its
// consoleChatEngineFunc seam pointed at a fakeEngineFactory — no
// codex/claude binary is ever spawned.
func newChatTestServer(t *testing.T) (*Server, *fakeEngineFactory) {
	t.Helper()
	s := newServerWithAuth(t)
	factory := &fakeEngineFactory{}
	s.consoleChatEngineFunc = factory.factory
	return s, factory
}

func createChatReq(project, body string) *http.Request {
	url := "/api/v1/console/chats"
	if project != "" {
		url += "?project=" + project
	}
	return httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
}

func chatMessageReq(sid, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/chats/"+sid+"/messages", strings.NewReader(body))
	r.SetPathValue("sid", sid)
	return r
}

func chatApprovalReq(sid, aid, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/chats/"+sid+"/approvals/"+aid, strings.NewReader(body))
	r.SetPathValue("sid", sid)
	r.SetPathValue("aid", aid)
	return r
}

func chatCloseReq(sid string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/chats/"+sid+"/close", nil)
	r.SetPathValue("sid", sid)
	return r
}

func chatMessagesGetReq(sid string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/console/chats/"+sid+"/messages", nil)
	r.SetPathValue("sid", sid)
	return r
}

// createChatSession drives the real handler chain to mint a chat session and
// returns its id plus the fake engine behind it.
func createChatSession(t *testing.T, s *Server, factory *fakeEngineFactory, project string, cookie *http.Cookie) (string, *fakeConsoleEngine) {
	t.Helper()
	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq(project, `{"engine":"codex","model":""}`)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create chat status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal create chat response: %v", err)
	}
	if body["session_id"] == "" {
		t.Fatalf("create chat response missing session_id: %+v", body)
	}
	return body["session_id"], factory.last()
}

func TestHandleCreateConsoleChat_MissingProject_Returns400(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-admin1@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("", `{"engine":"codex"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleChat_MissingEngine_Returns400(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-noengine")
	adminID := createTestUser(t, s, "chat-admin2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-noengine", `{}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleChat_UnknownProject_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-admin3@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("no-such-project", `{"engine":"codex"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleChat_NonAdminHuman_Returns403(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-nonadmin")
	userID := createTestUser(t, s, "chat-user1@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, userID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-nonadmin", `{"engine":"codex"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleChat_AdminCookie_Returns201AndStartsEngine(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-create")
	adminID := createTestUser(t, s, "chat-admin4@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	sid, eng := createChatSession(t, s, factory, "proj-chat-create", cookie)
	if eng == nil {
		t.Fatal("no fake engine was constructed")
	}
	if eng.startSpec.SessionID != sid {
		t.Errorf("engine started with SessionID = %q, want %q", eng.startSpec.SessionID, sid)
	}
}

func TestHandleConsoleChatMessage_EmptyText_Returns400(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-msg-empty")
	adminID := createTestUser(t, s, "chat-admin5@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-msg-empty", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatMessage)))
	req := chatMessageReq(sid, `{"text":"  "}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConsoleChatMessage_UnknownSession_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-admin6@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatMessage)))
	req := chatMessageReq("no-such-session", `{"text":"hi"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConsoleChatMessage_SecondWhileTurnActive_Returns409(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-409")
	adminID := createTestUser(t, s, "chat-admin7@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-409", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatMessage)))

	req1 := chatMessageReq(sid, `{"text":"first"}`)
	req1.AddCookie(cookie)
	rr1 := httptest.NewRecorder()
	chain.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first message status = %d, want 202; body=%s", rr1.Code, rr1.Body.String())
	}

	req2 := chatMessageReq(sid, `{"text":"second"}`)
	req2.AddCookie(cookie)
	rr2 := httptest.NewRecorder()
	chain.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("second message status = %d, want 409; body=%s", rr2.Code, rr2.Body.String())
	}
}
