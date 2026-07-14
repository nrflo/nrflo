package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

func TestRequireAuth_GlobalConsoleSession_XProjectMismatch_StillPasses(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, service.GlobalProjectID)
	seedConsoleProject(t, s, "proj-global-console-target")
	_, token := seedConsoleSession(t, s, service.GlobalProjectID)

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	r := reqWithBearer(token)
	r.Header.Set("X-Project", "proj-global-console-target")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK || !called {
		t.Fatalf("status=%d called=%v, want 200/true (global-scope console session exempt from X-Project match)", rr.Code, called)
	}
}

func TestRequireAuth_ProjectScopedConsoleSession_XProjectMismatch_403s(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-scoped-console")
	_, token := seedConsoleSession(t, s, "proj-scoped-console")

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	r := reqWithBearer(token)
	r.Header.Set("X-Project", "some-other-project")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, r)

	if rr.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d called=%v, want 403/false (project-scoped console session keeps strict X-Project match)", rr.Code, called)
	}
}

func TestConsoleToolProject_GlobalScopeSession_HonorsQueryOverride(t *testing.T) {
	sess := &model.AgentSession{ProjectID: service.GlobalProjectID}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/console/tools/project_status/call?project=proj-target", nil)

	if got := consoleToolProject(req, sess); got != "proj-target" {
		t.Errorf("consoleToolProject() = %q, want proj-target", got)
	}
}

func TestConsoleToolProject_GlobalScopeSession_XProjectHeaderOverride(t *testing.T) {
	sess := &model.AgentSession{ProjectID: service.GlobalProjectID}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/console/tools/project_status/call", nil)
	req.Header.Set("X-Project", "proj-header-target")

	// getProjectID reads X-Project from context, populated by projectMiddleware
	// ahead of routing in the real server chain — reproduce that here rather
	// than reading the header directly.
	s := &Server{}
	var got string
	s.projectMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = consoleToolProject(r, sess)
	})).ServeHTTP(httptest.NewRecorder(), req)

	if got != "proj-header-target" {
		t.Errorf("consoleToolProject() = %q, want proj-header-target", got)
	}
}

func TestConsoleToolProject_GlobalScopeSession_NoOverride_UsesSessionProject(t *testing.T) {
	sess := &model.AgentSession{ProjectID: service.GlobalProjectID}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/console/tools/project_status/call", nil)

	if got := consoleToolProject(req, sess); got != service.GlobalProjectID {
		t.Errorf("consoleToolProject() = %q, want %q", got, service.GlobalProjectID)
	}
}

func TestConsoleToolProject_ProjectScopedSession_IgnoresOverride(t *testing.T) {
	sess := &model.AgentSession{ProjectID: "proj-scoped-a"}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/console/tools/project_status/call?project=proj-scoped-b", nil)
	req.Header.Set("X-Project", "proj-scoped-b")

	if got := consoleToolProject(req, sess); got != "proj-scoped-a" {
		t.Errorf("consoleToolProject() = %q, want proj-scoped-a (override ignored)", got)
	}
}
