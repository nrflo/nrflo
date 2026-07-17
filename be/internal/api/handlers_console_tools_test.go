package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

// nonGoalTools are session-bound/lifecycle builtins that must never be
// reachable through the console profile (acceptance criterion 1).
var nonGoalTools = []string{
	"agent_finished", "agent_fail", "agent_continue", "agent_callback", "agent_context_update",
	"findings_add", "findings_add_bulk", "findings_append", "findings_append_bulk",
	"findings_get", "findings_delete", "emit_findings",
	"run_subworkflow", "get_subworkflow", "consult", "read_document", "artifact_add",
}

func seedConsoleSession(t *testing.T, s *Server, projectID string) (sessionID, token string) {
	t.Helper()
	sessionID, token, err := service.NewConsoleService(s.pool, s.clock).CreateSession(projectID, "")
	if err != nil {
		t.Fatalf("CreateSession(%q): %v", projectID, err)
	}
	return sessionID, token
}

func catalogueReq(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/console/tools", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func callToolReq(name, token, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/tools/"+name+"/call", strings.NewReader(body))
	r.SetPathValue("name", name)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

type catalogueResponse struct {
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	} `json:"tools"`
}

func TestHandleListConsoleTools_ValidBearer_Returns200WithFullProfile(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-catalogue")
	_, token := seedConsoleSession(t, s, "proj-ct-catalogue")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, catalogueReq(token))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body catalogueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	byName := make(map[string]json.RawMessage, len(body.Tools))
	for _, tool := range body.Tools {
		byName[tool.Name] = tool.InputSchema
	}

	wantPresent := append(append([]string{}, wantReusedBuiltinsForTest...), wantConsoleOnlyForTest...)
	for _, name := range wantPresent {
		schema, ok := byName[name]
		if !ok {
			t.Errorf("catalogue missing expected tool %q", name)
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(schema, &parsed); err != nil {
			t.Errorf("%s: input_schema does not unmarshal: %v", name, err)
			continue
		}
		if parsed["type"] != "object" {
			t.Errorf("%s: input_schema.type = %v, want object", name, parsed["type"])
		}
	}
	for _, name := range nonGoalTools {
		if _, ok := byName[name]; ok {
			t.Errorf("catalogue unexpectedly contains non-goal tool %q", name)
		}
	}
}

// wantReusedBuiltinsForTest / wantConsoleOnlyForTest mirror console.reusedBuiltins()
// + the console-only handlers registered in console.BuildRegistry, kept as an
// independent literal here (api package can't import console's unexported list).
var wantReusedBuiltinsForTest = []string{
	"project_findings_add", "project_findings_add_bulk", "project_findings_append",
	"project_findings_append_bulk", "project_findings_get", "project_findings_delete",
	"workflow_continue", "workflow_fail", "ticket_create", "ticket_update", "ticket_add_dependency",
	"web_search", "web_fetch",
}

var wantConsoleOnlyForTest = []string{
	"workflow_run", "workflow_stop", "workflow_retry_failed", "workflow_get", "workflow_list",
	"project_list", "project_status", "ticket_list", "ticket_get", "ticket_current", "artifact_list", "artifact_get",
}

func TestHandleListConsoleTools_NoAuth_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, catalogueReq(""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleListConsoleTools_CookieAdmin_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	adminID := createTestUser(t, s, "console-admin1@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	req := catalogueReq("")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (cookie principal never populates agent session); body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleListConsoleTools_WorkflowAgentToken_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-wfagent")
	seedWorkflowAgentForConsoleTest(t, s, "proj-ct-wfagent", "wf-agent-ct-1")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, catalogueReq("tok-wf-agent-ct-1"))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (kind=workflow_agent must not pass); body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleListConsoleTools_ClosedSession_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-closed")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-closed")
	if err := service.NewConsoleService(s.pool, s.clock).CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, catalogueReq(token))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for closed session token; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleListConsoleTools_ClosedSession_NilSessionMgr_Returns401 is the
// acceptance case that motivates doing the console check in-handler rather
// than in a new middleware: with sessionMgr==nil, requireAuth's cookie
// fallback passes the request through unauthenticated (no agentSessionKey
// set), so handleListConsoleTools must 401 on its own via getAgentSession==nil.
func TestHandleListConsoleTools_ClosedSession_NilSessionMgr_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-closed-nomgr")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-closed-nomgr")
	if err := service.NewConsoleService(s.pool, s.clock).CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	nilMgrServer := &Server{pool: s.pool, clock: s.clock, config: s.config, dataPath: s.dataPath}
	handler := nilMgrServer.requireAuth(http.HandlerFunc(nilMgrServer.handleListConsoleTools))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, catalogueReq(token))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with nil sessionMgr; body=%s", rr.Code, rr.Body.String())
	}
}
