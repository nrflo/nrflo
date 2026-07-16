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

func revokeSessionApprovalReq(sid, tool string) *http.Request {
	r := httptest.NewRequest(http.MethodDelete,
		"/api/v1/console/chats/"+sid+"/session-approvals/"+tool, nil)
	r.SetPathValue("sid", sid)
	r.SetPathValue("tool", tool)
	return r
}

// TestConsoleChatSessionApprovals_DetailAndRevoke: the detail snapshot lists
// the engine's session-approved tools; DELETE .../session-approvals/{tool}
// revokes on the engine and pushes the updated list on the session channel.
func TestConsoleChatSessionApprovals_DetailAndRevoke(t *testing.T) {
	s, factory := newChatTestServer(t)
	startTestHub(t, s)
	seedConsoleProject(t, s, "proj-chat-sess-appr")
	adminID := createTestUser(t, s, "chat-sess-appr-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-sess-appr", cookie)
	eng.setSessionAllowed("bash", "edit_file")

	detailChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	req := getChatReq(sid)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	detailChain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("detail status = %d; body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		SessionApprovals []string `json:"session_approvals"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.SessionApprovals) != 2 || body.SessionApprovals[0] != "bash" || body.SessionApprovals[1] != "edit_file" {
		t.Fatalf("session_approvals = %v, want [bash edit_file]", body.SessionApprovals)
	}

	client, ch := ws.NewTestClient(s.wsHub, "sess-appr-sub")
	s.wsHub.Register(client)
	s.wsHub.SubscribeSession(client, sid)

	revokeChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleRevokeConsoleChatSessionApproval)))
	revokeReq := revokeSessionApprovalReq(sid, "bash")
	revokeReq.AddCookie(cookie)
	revokeRR := httptest.NewRecorder()
	revokeChain.ServeHTTP(revokeRR, revokeReq)
	if revokeRR.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204; body=%s", revokeRR.Code, revokeRR.Body.String())
	}
	if got := eng.SessionApprovals(); len(got) != 1 || got[0] != "edit_file" {
		t.Fatalf("engine allowlist after revoke = %v, want [edit_file]", got)
	}
	waitForWSEventType(t, ch, ws.EventConsoleChatSessionApprovals, 2*time.Second)
}

// TestConsoleChatSessionApprovals_PumpPushesOnAllowForSession: a resolved
// approve_for_session decision makes pumpChatEvents push the full updated
// allowlist, so a live page learns of the new entry without a refetch.
func TestConsoleChatSessionApprovals_PumpPushesOnAllowForSession(t *testing.T) {
	s, factory := newChatTestServer(t)
	startTestHub(t, s)
	seedConsoleProject(t, s, "proj-chat-sess-appr-pump")
	adminID := createTestUser(t, s, "chat-sess-appr-pump@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-sess-appr-pump", cookie)

	client, ch := ws.NewTestClient(s.wsHub, "sess-appr-pump-sub")
	s.wsHub.Register(client)
	s.wsHub.SubscribeSession(client, sid)

	eng.setSessionAllowed("bash")
	eng.emit(spawner.EngineEvent{
		Type:       spawner.EventApprovalResolved,
		SessionID:  sid,
		ApprovalID: "appr-pump-1",
		Decision:   spawner.ApprovalApproveForSession,
	})
	waitForWSEventType(t, ch, ws.EventConsoleChatSessionApprovals, 2*time.Second)
}

func TestRevokeConsoleChatSessionApproval_UnknownSid_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-sess-appr-404@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleRevokeConsoleChatSessionApproval)))
	req := revokeSessionApprovalReq("no-such-chat", "bash")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}
