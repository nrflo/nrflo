package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
)

// TestHandleCreateConsoleChat_UnknownProfile_Returns400 verifies the create
// body's profile field is validated through ChatService.Create, mapping
// console.ErrUnknownProfile to a 400 rather than a 500.
func TestHandleCreateConsoleChat_UnknownProfile_Returns400(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-badprofile")
	adminID := createTestUser(t, s, "chat-admin-badprofile@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-badprofile", `{"engine":"codex","profile":"no-such-profile"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleCreateConsoleChat_T0DeciderProfile_ThreadsNativeToolsNone verifies
// a create body naming the t0-decider profile reaches buildChatEngineSpec's
// native-tool restriction on the started engine (NativeToolsCSV="none").
func TestHandleCreateConsoleChat_T0DeciderProfile_ThreadsNativeToolsNone(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-t0profile")
	adminID := createTestUser(t, s, "chat-admin-t0profile@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-t0profile", `{"engine":"claude","model":"","profile":"t0-decider"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}

	eng := factory.last()
	if eng == nil {
		t.Fatal("no fake engine constructed")
	}
	if eng.startSpec.NativeToolsCSV != "none" {
		t.Errorf("startSpec.NativeToolsCSV = %q, want none", eng.startSpec.NativeToolsCSV)
	}
	if eng.startSpec.ContextBudgetTokens != 50000 {
		t.Errorf("startSpec.ContextBudgetTokens = %d, want 50000", eng.startSpec.ContextBudgetTokens)
	}
}
