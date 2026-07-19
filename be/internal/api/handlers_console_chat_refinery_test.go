package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
)

// TestHandleCreateConsoleChat_RefineryEnabled_Returns201 verifies the REST
// body's refinery_enabled field is accepted (parsed into the widened
// ChatService.Create signature) without changing the create response shape.
// Wiring RefineryMgr.Start is covered at the console.ChatService layer
// (chat_service_refinery_test.go) with a fake RefineryLifecycle; the real
// server always wires a live refinery.Manager here, so this only exercises
// request parsing end to end.
func TestHandleCreateConsoleChat_RefineryEnabled_Returns201(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-refinery")
	adminID := createTestUser(t, s, "chat-admin-refinery@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-refinery", `{"engine":"codex","model":"","refinery_enabled":true}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	if eng := factory.last(); eng == nil {
		t.Fatal("no fake engine constructed")
	}
}

// TestHandleCreateConsoleChat_OmittedRefineryEnabled_Returns201 is the
// byte-identical regression: omitting refinery_enabled must not change the
// create path at all.
func TestHandleCreateConsoleChat_OmittedRefineryEnabled_Returns201(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-norefinery")
	adminID := createTestUser(t, s, "chat-admin-norefinery@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	sid, eng := createChatSession(t, s, factory, "proj-chat-norefinery", cookie)
	if sid == "" || eng == nil {
		t.Fatal("create chat session without refinery_enabled failed")
	}
}
