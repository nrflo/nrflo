package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/ws"
)

// chatWSAuthorizerFor runs one authenticated GET /api/v1/ws through the real
// auth chain and returns the ws.SessionAuthorizer the handler would install on
// that connection.
func chatWSAuthorizerFor(t *testing.T, s *Server, apply func(*http.Request)) ws.SessionAuthorizer {
	t.Helper()
	var auth ws.SessionAuthorizer
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		auth = s.consoleChatSessionAuthorizer(r)
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	apply(req)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/ws = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if auth == nil {
		t.Fatal("no session authorizer built for the connection")
	}
	return auth
}

// TestConsoleChatWSAuthorizer_MatchesRESTPredicate asserts the WS session
// channel is gated by the same admin/service-principal/own-bearer predicate as
// the REST routes: a client that gets 403 from GET /console/chats/{sid}/messages
// must not be able to read the same chat's live deltas by subscribing to its
// session channel instead.
func TestConsoleChatWSAuthorizer_MatchesRESTPredicate(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-wsauth")
	adminID := createTestUser(t, s, "chat-wsauth-admin@test.com", model.UserRoleAdmin, false)
	adminCookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-wsauth", adminCookie)

	viewerID := createTestUser(t, s, "chat-wsauth-viewer@test.com", model.UserRoleViewer, false)
	viewerCookie := injectSession(t, s, viewerID)

	adminAuth := chatWSAuthorizerFor(t, s, func(r *http.Request) { r.AddCookie(adminCookie) })
	if !adminAuth(sid) {
		t.Error("admin must be allowed to subscribe to a console-chat session channel")
	}
	if adminAuth("no-such-session") {
		t.Error("an unknown session id must be denied even for an admin")
	}

	viewerAuth := chatWSAuthorizerFor(t, s, func(r *http.Request) { r.AddCookie(viewerCookie) })
	if viewerAuth(sid) {
		t.Error("a non-admin user is 403'd by the REST routes and must be denied the session channel too")
	}
}

// TestConsoleChatWSAuthorizer_MismatchedServiceToken_Denied asserts a service
// token scoped to another project cannot subscribe to this project's chat.
func TestConsoleChatWSAuthorizer_MismatchedServiceToken_Denied(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-wsauth-a")
	adminID := createTestUser(t, s, "chat-wsauth-svc@test.com", model.UserRoleAdmin, false)
	adminCookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-wsauth-a", adminCookie)

	_, plain := seedServiceToken(t, s, "proj-chat-wsauth-b", "ci")
	auth := chatWSAuthorizerFor(t, s, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+plain)
	})
	if auth(sid) {
		t.Error("a service token for another project must be denied the session channel")
	}
}
