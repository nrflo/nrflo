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
	tok, plain, err := svc.Create(projectID, name, "")
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
