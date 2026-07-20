package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

// toolArgsForDispatchTest supplies minimal valid arguments for every console
// tool except web_search/web_fetch, which are excluded from this live-dispatch
// table because they call a real network provider (tools_web) — no real
// network is allowed in tests (root CLAUDE.md rule 4). With no orchestrator
// wired (test servers never set s.orchestrator), workflow_run/stop/
// retry_failed short-circuit on their own missingService(...)
// check before touching the DB or network, so they stay in this table.
var toolArgsForDispatchTest = map[string]string{
	"project_findings_add":         `{"key":"k","value":"v"}`,
	"project_findings_add_bulk":    `{"key_values":{"k":"v"}}`,
	"project_findings_append":      `{"key":"k","value":"v"}`,
	"project_findings_append_bulk": `{"key_values":{"k":"v"}}`,
	"project_findings_get":         `{}`,
	"project_findings_delete":      `{"keys":["k"]}`,
	"workflow_continue":            `{"instance_id":"wfi-nope"}`,
	"workflow_fail":                `{"instance_id":"wfi-nope","reason":"r"}`,
	"ticket_create":                `{"title":"t"}`,
	"ticket_update":                `{"ticket_id":"t-nope","title":"t2"}`,
	"ticket_add_dependency":        `{"ticket_id":"a","depends_on_id":"b"}`,
	"workflow_run":                 `{"workflow":"w"}`,
	"workflow_stop":                `{"instance_id":"wfi-nope"}`,
	"workflow_retry_failed":        `{"workflow":"w","session_id":"s","instance_id":"wfi-nope"}`,
	"workflow_get":                 `{"instance_id":"wfi-nope"}`,
	"workflow_list":                `{}`,
	"project_list":                 `{}`,
	"project_status":               `{}`,
	"ticket_list":                  `{}`,
	"ticket_get":                   `{"ticket_id":"t-nope"}`,
	"ticket_current":               `{}`,
	"artifact_list":                `{"instance_id":"wfi-nope"}`,
	"artifact_get":                 `{"instance_id":"wfi-nope","name":"n"}`,
	"delegate":                     `{"tier":"extractor","brief":"b"}`,
	"get_delegation":               `{"delegation_id":"wfi-nope"}`,
	"dynamic_workflow":             `{"instructions":"do it"}`,
	"get_subworkflow":              `{"instance_id":"wfi-nope"}`,
	"revise_plan":                  `{"instance_id":"wfi-nope","revision":0}`,
	"approve_plan":                 `{"instance_id":"wfi-nope","revision":0}`,
	"consult":                      `{"consultant":"c","question":"q"}`,
}

func TestHandleCallConsoleTool_EveryListedTool_DispatchesNon404(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-dispatch")
	_, token := seedConsoleSession(t, s, "proj-ct-dispatch")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))

	// Sanity: the table above must cover the full catalogue minus the two
	// excluded network tools.
	wantTools := append(append([]string{}, wantReusedBuiltinsForTest...), wantConsoleOnlyForTest...)
	wantCovered := 0
	for _, name := range wantTools {
		if name == "web_search" || name == "web_fetch" {
			continue
		}
		wantCovered++
	}
	if len(toolArgsForDispatchTest) != wantCovered {
		t.Fatalf("toolArgsForDispatchTest has %d entries, want %d (catalogue minus web_search/web_fetch)", len(toolArgsForDispatchTest), wantCovered)
	}

	for name, args := range toolArgsForDispatchTest {
		t.Run(name, func(t *testing.T) {
			body := `{"arguments":` + args + `}`
			rr := httptest.NewRecorder()
			chain.ServeHTTP(rr, callToolReq(name, token, body))
			if rr.Code == http.StatusNotFound || rr.Code == http.StatusUnauthorized || rr.Code >= 500 {
				t.Errorf("status = %d, want a dispatch (not 404/401/5xx); body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleCallConsoleTool_UnlistedTool_Returns404(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-404")
	_, token := seedConsoleSession(t, s, "proj-ct-404")
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))

	for _, name := range []string{"agent_finished", "nope"} {
		t.Run(name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			chain.ServeHTTP(rr, callToolReq(name, token, `{"arguments":{}}`))
			if rr.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleCallConsoleTool_ClosedSession_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-call-closed")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-call-closed")
	if err := service.NewConsoleService(s.pool, s.clock).CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, callToolReq("project_list", token, `{"arguments":{}}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for closed session token; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCallConsoleTool_ClosedSession_NilSessionMgr_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-call-closed-nomgr")
	sessionID, token := seedConsoleSession(t, s, "proj-ct-call-closed-nomgr")
	if err := service.NewConsoleService(s.pool, s.clock).CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	nilMgrServer := &Server{pool: s.pool, clock: s.clock, config: s.config, dataPath: s.dataPath}
	handler := nilMgrServer.requireAuth(http.HandlerFunc(nilMgrServer.handleCallConsoleTool))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, callToolReq("project_list", token, `{"arguments":{}}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with nil sessionMgr; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCallConsoleTool_CookieAdmin_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	adminID := createTestUser(t, s, "console-admin2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	req := callToolReq("project_list", "", `{"arguments":{}}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCallConsoleTool_WorkflowAgentToken_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-call-wfagent")
	seedWorkflowAgentForConsoleTest(t, s, "proj-ct-call-wfagent", "wf-agent-ct-2")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, callToolReq("project_list", "tok-wf-agent-ct-2", `{"arguments":{}}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCallConsoleTool_MalformedBody_Returns400(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-malformed")
	_, token := seedConsoleSession(t, s, "proj-ct-malformed")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, callToolReq("project_list", token, `not-json`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCallConsoleTool_SuccessfulCall_ReturnsOutputShape(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-ct-shape")
	_, token := seedConsoleSession(t, s, "proj-ct-shape")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCallConsoleTool)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, callToolReq("project_list", token, `{"arguments":{}}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Output     string `json:"output"`
		IsError    bool   `json:"is_error"`
		DurationMs int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.IsError {
		t.Errorf("is_error = true, want false; output=%s", body.Output)
	}
}
