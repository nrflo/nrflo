package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
)

func switchModelReq(sid, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/chats/"+sid+"/switch-model", strings.NewReader(body))
	r.SetPathValue("sid", sid)
	return r
}

func handsSiblingReq(sid string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/chats/"+sid+"/hands-sibling", nil)
	r.SetPathValue("sid", sid)
	return r
}

// createT0DeciderChatSession mirrors createChatSession but starts a
// t0-decider profile chat under the claude engine (matching the profile's
// DefaultEngine) so the sibling-flow gate (t0-decider-only) is satisfied.
func createT0DeciderChatSession(t *testing.T, s *Server, factory *fakeEngineFactory, project string, cookie *http.Cookie) string {
	t.Helper()
	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq(project, `{"engine":"claude","model":"","profile":"t0-decider"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create t0-decider chat status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal create chat response: %v", err)
	}
	if body["session_id"] == "" {
		t.Fatalf("create chat response missing session_id: %+v", body)
	}
	return body["session_id"]
}

func TestHandleSwitchConsoleChatModel_HappyPath_Returns201WithSiblingID(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-switch-model")
	adminID := createTestUser(t, s, "chat-switch1@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid := createT0DeciderChatSession(t, s, factory, "proj-chat-switch-model", cookie)
	originEngine := factory.last()

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleSwitchConsoleChatModel)))
	req := switchModelReq(sid, `{"engine":"claude","model":"sonnet-5"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["sibling_session_id"] == "" || body["sibling_session_id"] == sid {
		t.Errorf("sibling_session_id = %q, want a new distinct session id", body["sibling_session_id"])
	}
	if originEngine.isStopped() {
		t.Error("origin engine was stopped by switch-model, want it left live")
	}
}

func TestHandleSwitchConsoleChatModel_NonT0Decider_Returns400(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-switch-nont0")
	adminID := createTestUser(t, s, "chat-switch2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-switch-nont0", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleSwitchConsoleChatModel)))
	req := switchModelReq(sid, `{}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleSwitchConsoleChatModel_UnknownSession_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-switch3@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleSwitchConsoleChatModel)))
	req := switchModelReq("no-such-session", `{}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleOpenHandsSibling_NonT0Decider_Returns400 exercises the
// well-defined (non-buggy) part of the hands-sibling flow: the profile gate
// rejects a non-t0-decider origin before ever reaching the broken
// engine-resolution path (see console.chat_service_sibling_test.go's
// documented OpenHandsSibling production bug).
func TestHandleOpenHandsSibling_NonT0Decider_Returns400(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-hands-nont0")
	adminID := createTestUser(t, s, "chat-hands1@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-hands-nont0", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleOpenHandsSibling)))
	req := handsSiblingReq(sid)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleOpenHandsSibling_UnknownSession_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-hands2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleOpenHandsSibling)))
	req := handsSiblingReq("no-such-session")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}
