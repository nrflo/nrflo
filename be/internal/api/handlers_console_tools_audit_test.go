package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

func auditRowsFor(t *testing.T, s *Server, sessionID string) []*model.AuditEntry {
	t.Helper()
	entries, _, err := repo.NewAuditRepo(s.pool, s.clock).List(model.AuditFilter{
		ResourceType: "agent_session", ResourceID: sessionID,
	}, 1, 50)
	if err != nil {
		t.Fatalf("AuditRepo.List: %v", err)
	}
	return entries
}

func TestHandleCallConsoleTool_SuccessfulCall_WritesOkAuditRow(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-audit-ok")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-audit-ok")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, callToolReq("project_list", token, `{"arguments":{}}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	entries := auditRowsFor(t, s, sessionID)
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Action != "console.tool.call" {
		t.Errorf("action = %q, want console.tool.call", e.Action)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(e.Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["tool"] != "project_list" {
		t.Errorf("metadata.tool = %v, want project_list", meta["tool"])
	}
	if meta["outcome"] != "ok" {
		t.Errorf("metadata.outcome = %v, want ok", meta["outcome"])
	}
	if meta["args_digest"] == "" || meta["args_digest"] == nil {
		t.Errorf("metadata.args_digest = %v, want non-empty", meta["args_digest"])
	}
	if _, ok := meta["duration_ms"]; !ok {
		t.Errorf("metadata missing duration_ms: %v", meta)
	}
	if meta["project"] != "proj-ct-audit-ok" {
		t.Errorf("metadata.project = %v, want proj-ct-audit-ok", meta["project"])
	}
}

// TestHandleCallConsoleTool_GlobalSession_AuditsTargetProject pins the audit
// row to the project the call actually acted on. A global-scope console session
// may retarget another project via X-Project (consoleToolProject), and the
// trail must name that project — not the session's own `__global__`.
func TestHandleCallConsoleTool_GlobalSession_AuditsTargetProject(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, service.GlobalProjectID)
	seedConsoleProject(t, s, "proj-ct-audit-target")
	sessionID, token := seedConsoleSession(t, s, service.GlobalProjectID)

	req := callToolReq("project_list", token, `{"arguments":{}}`)
	req.Header.Set("X-Project", "proj-ct-audit-target")

	// projectMiddleware is what lifts X-Project into the context getProjectID
	// reads, so it must be in the chain here exactly as Start() assembles it.
	chain := s.projectMiddleware(s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool))))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	entries := auditRowsFor(t, s, sessionID)
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(entries[0].Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["project"] != "proj-ct-audit-target" {
		t.Errorf("metadata.project = %v, want proj-ct-audit-target (the acted-on project, not %s)",
			meta["project"], service.GlobalProjectID)
	}
}

func TestHandleCallConsoleTool_ToolErrorCall_WritesToolErrorAuditRow(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-audit-toolerr")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-audit-toolerr")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, callToolReq("workflow_get", token, `{"arguments":{"instance_id":"wfi-nope"}}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	entries := auditRowsFor(t, s, sessionID)
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(entries[0].Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["outcome"] != "tool_error" {
		t.Errorf("metadata.outcome = %v, want tool_error", meta["outcome"])
	}
}

func TestHandleCallConsoleTool_UnlistedTool_WritesNotFoundAuditRow(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-audit-404")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-audit-404")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, callToolReq("nope", token, `{"arguments":{}}`))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}

	entries := auditRowsFor(t, s, sessionID)
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(entries[0].Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["outcome"] != "not_found" {
		t.Errorf("metadata.outcome = %v, want not_found", meta["outcome"])
	}
}

// TestHandleCallConsoleTool_AuditTrail_QueryableViaAuditLogEndpoint exercises
// the full acceptance path end-to-end: a console tool call, then the same
// row retrieved through the admin-facing GET /api/v1/audit-log endpoint
// filtered by resource_type=agent_session&resource_id=<session id>.
func TestHandleCallConsoleTool_AuditTrail_QueryableViaAuditLogEndpoint(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-audit-endpoint")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-audit-endpoint")

	callChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	callChain.ServeHTTP(rr, callToolReq("project_list", token, `{"arguments":{}}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("tool call status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	adminID := createTestUser(t, s, "console-audit-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	auditChain := s.sessionMgr.LoadAndSave(s.requireAdmin(http.HandlerFunc(s.handleListAuditLog)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-log?resource_type=agent_session&resource_id="+sessionID, nil)
	req.AddCookie(cookie)
	rr2 := httptest.NewRecorder()
	auditChain.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("audit-log status = %d, want 200; body=%s", rr2.Code, rr2.Body.String())
	}

	var body struct {
		Items []model.AuditEntry `json:"items"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("total=%d len(items)=%d, want 1/1", body.Total, len(body.Items))
	}
	if body.Items[0].ResourceID != sessionID || body.Items[0].ResourceType != "agent_session" {
		t.Errorf("item = %+v, want resource_type=agent_session resource_id=%s", body.Items[0], sessionID)
	}
}
