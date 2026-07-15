package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

// TestConsoleToken_PassesBeforeClose_401sAfterClose is the ticket's core
// acceptance test: a console session's bearer token gates requireAuth exactly
// like a spawned agent's, and closing the session kills the token.
func TestConsoleToken_PassesBeforeClose_401sAfterClose(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-gate")

	consoleSvc := service.NewConsoleService(s.pool, s.clock)
	sessionID, token, err := consoleSvc.CreateSession("proj-console-gate", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	called := false
	authChain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	rr := httptest.NewRecorder()
	authChain.ServeHTTP(rr, reqWithBearer(token))
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("before close: status=%d called=%v, want 200/true", rr.Code, called)
	}

	if err := consoleSvc.CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	called = false
	rr2 := httptest.NewRecorder()
	authChain2 := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	authChain2.ServeHTTP(rr2, reqWithBearer(token))
	if rr2.Code != http.StatusUnauthorized || called {
		t.Fatalf("after close: status=%d called=%v, want 401/false", rr2.Code, called)
	}
}

func TestHandleCloseConsoleSession_OwnBearer_Returns204(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-close-own")
	consoleSvc := service.NewConsoleService(s.pool, s.clock)
	sessionID, token, err := consoleSvc.CreateSession("proj-console-close-own", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleSession)))
	req := closeConsoleReq(sessionID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCloseConsoleSession_Admin_Returns204(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-close-admin")
	adminID := createTestUser(t, s, "admin4@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	consoleSvc := service.NewConsoleService(s.pool, s.clock)
	sessionID, _, err := consoleSvc.CreateSession("proj-console-close-admin", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleSession)))
	req := closeConsoleReq(sessionID)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCloseConsoleSession_UnknownID_Returns404(t *testing.T) {
	s := newServerWithAuth(t)
	adminID := createTestUser(t, s, "admin5@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleSession)))
	req := closeConsoleReq("does-not-exist")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCloseConsoleSession_WorkflowAgentSessionID_Returns404(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-close-kindguard")
	seedWorkflowAgentForConsoleTest(t, s, "proj-console-close-kindguard", "wf-agent-close-1")
	adminID := createTestUser(t, s, "admin6@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleSession)))
	req := closeConsoleReq("wf-agent-close-1")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (kind guard must never terminate a workflow agent); body=%s", rr.Code, rr.Body.String())
	}

	var status string
	if err := s.pool.QueryRow(`SELECT status FROM agent_sessions WHERE id = 'wf-agent-close-1'`).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "user_interactive" {
		t.Errorf("workflow agent status = %q, want unchanged user_interactive", status)
	}
}

func TestHandleCloseConsoleSession_NonAdminForeignProject_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-close-foreign")
	consoleSvc := service.NewConsoleService(s.pool, s.clock)
	sessionID, _, err := consoleSvc.CreateSession("proj-console-close-foreign", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	userID := createTestUser(t, s, "user2@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, userID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleSession)))
	req := closeConsoleReq(sessionID)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}

	_, plain := seedServiceToken(t, s, "proj-console-close-other", "ci")
	req2 := closeConsoleReq(sessionID)
	req2.Header.Set("Authorization", "Bearer "+plain)
	rr2 := httptest.NewRecorder()
	chain.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusForbidden {
		t.Fatalf("foreign service token: status = %d, want 403; body=%s", rr2.Code, rr2.Body.String())
	}
}
