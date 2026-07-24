package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
)

func yoloReq(method, sid string) *http.Request {
	r := httptest.NewRequest(method, "/api/v1/console/chats/"+sid+"/yolo", nil)
	r.SetPathValue("sid", sid)
	return r
}

// TestHandleSetConsoleChatYolo_PostOnDeleteOff drives the real handler chain:
// POST turns yolo on (engine + persisted column + audit row), DELETE turns
// it off, mirroring the session-approvals revoke handler test.
func TestHandleSetConsoleChatYolo_PostOnDeleteOff(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-yolo")
	adminID := createTestUser(t, s, "chat-yolo-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-yolo", cookie)

	yoloChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleSetConsoleChatYolo)))

	postReq := yoloReq(http.MethodPost, sid)
	postReq.AddCookie(cookie)
	postRR := httptest.NewRecorder()
	yoloChain.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusNoContent {
		t.Fatalf("POST yolo status = %d, want 204; body=%s", postRR.Code, postRR.Body.String())
	}
	if !eng.Yolo() {
		t.Error("engine Yolo() = false after POST .../yolo, want true")
	}

	deleteReq := yoloReq(http.MethodDelete, sid)
	deleteReq.AddCookie(cookie)
	deleteRR := httptest.NewRecorder()
	yoloChain.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("DELETE yolo status = %d, want 204; body=%s", deleteRR.Code, deleteRR.Body.String())
	}
	if eng.Yolo() {
		t.Error("engine Yolo() = true after DELETE .../yolo, want false")
	}

	auditChain := s.sessionMgr.LoadAndSave(s.requireAdmin(http.HandlerFunc(s.handleListAuditLog)))
	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-log?resource_type=agent_session&resource_id="+sid+"&action=console_chat.yolo", nil)
	auditReq.AddCookie(cookie)
	auditRR := httptest.NewRecorder()
	auditChain.ServeHTTP(auditRR, auditReq)
	if auditRR.Code != http.StatusOK {
		t.Fatalf("audit-log status = %d, want 200; body=%s", auditRR.Code, auditRR.Body.String())
	}
	var auditBody struct {
		Items []model.AuditEntry `json:"items"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(auditRR.Body.Bytes(), &auditBody); err != nil {
		t.Fatalf("unmarshal audit-log: %v", err)
	}
	if auditBody.Total != 2 {
		t.Errorf("audit-log total = %d, want 2 (one per POST/DELETE)", auditBody.Total)
	}
}

func TestHandleSetConsoleChatYolo_UnknownSid_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-yolo-404@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleSetConsoleChatYolo)))
	req := yoloReq(http.MethodPost, "no-such-chat")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleListConsoleChats_YoloField_ResolvesNullToGlobal verifies the
// list item's yolo field resolves the persisted (NULL) column to the
// default-ON global setting, mirroring resolveYolo's fallback.
func TestHandleListConsoleChats_YoloField_ResolvesNullToGlobal(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-list-yolo")
	adminID := createTestUser(t, s, "chat-list-yolo-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-list-yolo", cookie)

	listChain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleChats)))
	listReq := listChatsReq("proj-chat-list-yolo")
	listReq.AddCookie(cookie)
	listRR := httptest.NewRecorder()
	listChain.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", listRR.Code, listRR.Body.String())
	}
	var body struct {
		Sessions []consoleChatListItem `json:"sessions"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	var item *consoleChatListItem
	for i := range body.Sessions {
		if body.Sessions[i].SessionID == sid {
			item = &body.Sessions[i]
		}
	}
	if item == nil {
		t.Fatalf("session %q not found in list response: %+v", sid, body.Sessions)
	}
	if !item.Yolo {
		t.Error("list item Yolo = false for a fresh chat, want true (default-ON console_yolo)")
	}
}

// TestHandleGetConsoleChat_YoloField_PrefersLiveSnapshot verifies the detail
// response prefers the live engine's Yolo state (via Snapshot) over the
// persisted column once a live session has toggled it.
func TestHandleGetConsoleChat_YoloField_PrefersLiveSnapshot(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-detail-yolo")
	adminID := createTestUser(t, s, "chat-detail-yolo-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-detail-yolo", cookie)

	yoloChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleSetConsoleChatYolo)))
	offReq := yoloReq(http.MethodDelete, sid)
	offReq.AddCookie(cookie)
	offRR := httptest.NewRecorder()
	yoloChain.ServeHTTP(offRR, offReq)
	if offRR.Code != http.StatusNoContent {
		t.Fatalf("DELETE yolo status = %d, want 204; body=%s", offRR.Code, offRR.Body.String())
	}

	detailChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	detailReq := getChatReq(sid)
	detailReq.AddCookie(cookie)
	detailRR := httptest.NewRecorder()
	detailChain.ServeHTTP(detailRR, detailReq)
	if detailRR.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200; body=%s", detailRR.Code, detailRR.Body.String())
	}
	var body struct {
		Yolo bool `json:"yolo"`
		Live bool `json:"live"`
	}
	if err := json.Unmarshal(detailRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if !body.Live {
		t.Fatal("detail live = false, want true (session still open)")
	}
	if body.Yolo {
		t.Error("detail yolo = true after DELETE .../yolo, want false (live snapshot)")
	}
}
