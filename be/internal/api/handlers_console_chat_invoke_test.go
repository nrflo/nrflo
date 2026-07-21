package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/repo"
)

func invokeReq(sid, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/chats/"+sid+"/invoke", strings.NewReader(body))
	r.SetPathValue("sid", sid)
	return r
}

func chatToolsReq(sid string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/console/chats/"+sid+"/tools", nil)
	r.SetPathValue("sid", sid)
	return r
}

func bearerReq(r *http.Request, token string) *http.Request {
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

type invokeResponse struct {
	OK         bool   `json:"ok"`
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
	Informed   bool   `json:"informed"`
}

func TestHandleConsoleChatInvoke_AdminCookie_Returns200(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-invoke-admin")
	adminID := createTestUser(t, s, "chat-invoke-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-invoke-admin", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatInvoke)))
	req := invokeReq(sid, `{"tool":"project_list","arguments":{}}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body invokeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK {
		t.Errorf("ok = false, want true; result=%s", body.Result)
	}
}

func TestHandleConsoleChatInvoke_OwnBearer_Returns200(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-invoke-bearer")
	adminID := createTestUser(t, s, "chat-invoke-bearer-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-invoke-bearer", cookie)
	row, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil || row == nil {
		t.Fatalf("load session: row=%v err=%v", row, err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatInvoke)))
	req := bearerReq(invokeReq(sid, `{"tool":"project_list","arguments":{}}`), row.SpawnToken.String)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConsoleChatInvoke_AnotherChatsBearer_Returns403(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-invoke-cross")
	adminID := createTestUser(t, s, "chat-invoke-cross-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid1, _ := createChatSession(t, s, factory, "proj-chat-invoke-cross", cookie)
	sid2, _ := createChatSession(t, s, factory, "proj-chat-invoke-cross", cookie)
	row2, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid2)
	if err != nil || row2 == nil {
		t.Fatalf("load session 2: row=%v err=%v", row2, err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatInvoke)))
	req := bearerReq(invokeReq(sid1, `{"tool":"project_list","arguments":{}}`), row2.SpawnToken.String)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConsoleChatInvoke_EmptyTool_Returns400(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-invoke-empty")
	adminID := createTestUser(t, s, "chat-invoke-empty-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-invoke-empty", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatInvoke)))
	req := invokeReq(sid, `{"tool":"","arguments":{}}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConsoleChatInvoke_UnknownTool_Returns404(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-invoke-unknown")
	adminID := createTestUser(t, s, "chat-invoke-unknown-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-invoke-unknown", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatInvoke)))
	req := invokeReq(sid, `{"tool":"no_such_tool","arguments":{}}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConsoleChatInvoke_InformModelTrue_ReturnsInformedTrue(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-invoke-inform")
	adminID := createTestUser(t, s, "chat-invoke-inform-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-invoke-inform", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatInvoke)))
	req := invokeReq(sid, `{"tool":"project_list","arguments":{},"inform_model":true}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body invokeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.Informed {
		t.Error("informed = false, want true")
	}
}

func TestHandleConsoleChatTools_ReturnsProfileCatalogueWithSchema(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-tools")
	adminID := createTestUser(t, s, "chat-tools-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-tools", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatTools)))
	req := chatToolsReq(sid)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body catalogueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Tools) == 0 {
		t.Fatal("Tools is empty, want the console catalogue")
	}
	for _, tool := range body.Tools {
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("tool %q input_schema does not unmarshal: %v", tool.Name, err)
		}
		if schema["type"] != "object" {
			t.Errorf("tool %q input_schema.type = %v, want %q", tool.Name, schema["type"], "object")
			break
		}
	}
}

func TestHandleConsoleChatInvoke_WritesAuditRowRegardlessOfOutcome(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-invoke-audit")
	adminID := createTestUser(t, s, "chat-invoke-audit-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-invoke-audit", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleConsoleChatInvoke)))

	// One success, one failure (unknown tool) — both must be audited.
	req1 := invokeReq(sid, `{"tool":"project_list","arguments":{}}`)
	req1.AddCookie(cookie)
	rr1 := httptest.NewRecorder()
	chain.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("success call status = %d, want 200; body=%s", rr1.Code, rr1.Body.String())
	}

	req2 := invokeReq(sid, `{"tool":"no_such_tool","arguments":{}}`)
	req2.AddCookie(cookie)
	rr2 := httptest.NewRecorder()
	chain.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("unknown-tool call status = %d, want 404; body=%s", rr2.Code, rr2.Body.String())
	}

	entries := auditRowsFor(t, s, sid)
	var invokeEntries int
	for _, e := range entries {
		if e.Action == "console.tool.call" {
			invokeEntries++
		}
	}
	if invokeEntries != 2 {
		t.Fatalf("len(console.tool.call audit rows) = %d, want 2 (one per invoke call, success and failure)", invokeEntries)
	}
}
