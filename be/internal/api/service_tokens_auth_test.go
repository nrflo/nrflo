package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/clock"
	"be/internal/service"
)

func seedServiceToken(t *testing.T, s *Server, projectID, name string) (tokenID, plaintext string) {
	t.Helper()
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, datetime('now'), datetime('now'))`, projectID, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	svc := service.NewServiceTokenService(s.pool, clock.Real())
	tok, plain, err := svc.Create(projectID, name, "", "project")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return tok.ID, plain
}

func TestRequireAuth_ServiceToken_Accepted(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-svc-auth", "ci-pipeline")

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("expected next handler to be called for valid service token")
	}
}

func TestRequireAuth_ServiceToken_ProjectMismatch_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-svc-mismatch", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Project", "some-other-project")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestServiceTokenService_GlobalScope(t *testing.T) {
	s := newServerWithAuth(t)
	svc := service.NewServiceTokenService(s.pool, clock.Real())

	tok, _, err := svc.Create("", "global-ci", "", "global")
	if err != nil {
		t.Fatalf("create global token: %v", err)
	}
	if tok.Scope != "global" || tok.ProjectID != "" {
		t.Errorf("global token = scope %q project %q, want global/empty", tok.Scope, tok.ProjectID)
	}
	if _, _, err := svc.Create("", "bad", "", "project"); err == nil {
		t.Error("project-scope token without project_id should error")
	}
	if _, _, err := svc.Create("", "bad", "", "weird"); err == nil {
		t.Error("invalid scope should error")
	}
}

func TestRequireAuth_GlobalServiceToken_AnyProjectAccepted(t *testing.T) {
	s := newServerWithAuth(t)
	svc := service.NewServiceTokenService(s.pool, clock.Real())
	_, plain, err := svc.Create("", "global-ci", "", "global")
	if err != nil {
		t.Fatalf("create global token: %v", err)
	}

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Project", "any-project-at-all") // a project token would 403 here
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("global token status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("expected next handler called for global service token with arbitrary X-Project")
	}
}

func TestRequireAuth_ServiceToken_Unknown_FallsThroughToCookie(t *testing.T) {
	s := newServerWithAuth(t)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer nrf_obviouslynotarealtokenstringxxxxxxxx")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (no token, no cookie); body=%s", rr.Code, rr.Body.String())
	}
}

func TestRequireProjectAdmin_ServiceToken_MatchingPathID_Passes(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-svc-pa", "ci")

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(sentinelHandler(&called)))
	req := httptest.NewRequest(http.MethodGet, "/projects/proj-svc-pa/env-vars/FOO", nil)
	req.SetPathValue("id", "proj-svc-pa")
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("expected next handler to be called")
	}
}

func TestRequireProjectAdmin_ServiceToken_NonMatchingPathID_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-svc-pa-deny", "ci")
	// also seed the "other" project so the FK check on env-vars routes (not under test here) wouldn't break
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at)
		VALUES ('other-proj', 'OtherProj', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed other project: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodPut, "/projects/other-proj/env-vars/FOO", nil)
	req.SetPathValue("id", "other-proj")
	req.Header.Set("Authorization", "Bearer "+plain)
	// X-Project absent (auth middleware allows that); requireProjectAdmin must
	// still deny because the path's project doesn't match the token's project.
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

// Header/query-scoped routes (e.g. python-scripts writes) carry the project in
// the request, not the {id} path param. requireProjectAdmin must resolve scope
// via getProjectID so a matching service token passes.
func TestRequireProjectAdmin_ServiceToken_QueryScope_Passes(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-svc-hdr", "ci")

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(sentinelHandler(&called)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project=proj-svc-hdr", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("expected next handler to be called for matching project scope")
	}
}

// A token for project B must not act on project A even when the auth-validated
// X-Project header says B: the project resolved by getProjectID (here the
// ?project= query) is what's authorized, closing the header/query divergence.
func TestRequireProjectAdmin_ServiceToken_QueryProjectMismatch_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-svc-hdr-b", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project=proj-svc-hdr-a", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestServiceTokenService_LookupByPlaintext_RoundTrip(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-svc-lookup", "ci")

	svc := service.NewServiceTokenService(s.pool, clock.Real())
	tok, err := svc.LookupByPlaintext(plain)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if tok == nil {
		t.Fatal("expected token, got nil")
	}
	if tok.ProjectID != "proj-svc-lookup" {
		t.Fatalf("project = %q, want %q", tok.ProjectID, "proj-svc-lookup")
	}

	// unknown plaintext -> nil, no error
	miss, err := svc.LookupByPlaintext("nrf_definitelynotrealxxxxxxxxxxxxxxxx")
	if err != nil {
		t.Fatalf("lookup miss should not error: %v", err)
	}
	if miss != nil {
		t.Fatal("expected nil for unknown plaintext")
	}
}

// A global service token must satisfy requireProjectAdmin, just like it
// satisfies requireAuth with any X-Project. Without this, a global token
// cannot manage any project-scoped resources (env-vars, python-scripts, etc),
// defeating the purpose of a global token.
func TestRequireProjectAdmin_GlobalServiceToken_AnyProjectAccepted(t *testing.T) {
	s := newServerWithAuth(t)
	svc := service.NewServiceTokenService(s.pool, clock.Real())
	_, plain, err := svc.Create("", "global-ci", "", "global")
	if err != nil {
		t.Fatalf("create global token: %v", err)
	}

	// Seed a project so the path can target it
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, datetime('now'), datetime('now'))`, "proj-global-admin", "ProjGlobalAdmin"); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(sentinelHandler(&called)))
	req := httptest.NewRequest(http.MethodPut, "/projects/proj-global-admin/env-vars/FOO", nil)
	req.SetPathValue("id", "proj-global-admin")
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("global token status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("expected next handler to be called for global service token")
	}
}
