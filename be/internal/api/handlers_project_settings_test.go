package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/artifact"
	"be/internal/clock"
	"be/internal/db"
)

func newProjectSettingsServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ps_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "proj-settings-test"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'Settings Test', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return &Server{pool: pool, clock: clock.Real()}, projectID
}

func doCleanupRequest(t *testing.T, s *Server, handler func(http.ResponseWriter, *http.Request),
	method, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/projects/"+projectID+"/settings/cleanup", strings.NewReader(body))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func decodeCleanupResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode cleanup response: %v", err)
	}
	return m
}

func decodeArtifactStorageResponse(t *testing.T, rr *httptest.ResponseRecorder) artifact.Config {
	t.Helper()
	var cfg artifact.Config
	if err := json.NewDecoder(rr.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode artifact storage response: %v", err)
	}
	return cfg
}

func TestHandleGetProjectCleanup_Defaults(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCleanupRequest(t, s, s.handleGetProjectCleanup, http.MethodGet, projectID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeCleanupResponse(t, rr)
	if m["enabled"] != false {
		t.Errorf("enabled = %v, want false", m["enabled"])
	}
	if m["retention_limit"] != float64(0) {
		t.Errorf("retention_limit = %v, want 0", m["retention_limit"])
	}
}

func TestHandleGetProjectCleanup_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//settings/cleanup", nil)
	rr := httptest.NewRecorder()
	s.handleGetProjectCleanup(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when project id is missing", rr.Code)
	}
}

func TestHandlePutProjectCleanup_EnableOnly(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCleanupRequest(t, s, s.handlePutProjectCleanup, http.MethodPut, projectID, `{"enabled":true}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeCleanupResponse(t, rr)
	if m["enabled"] != true {
		t.Errorf("enabled = %v, want true", m["enabled"])
	}
	if m["retention_limit"] != float64(0) {
		t.Errorf("retention_limit = %v, want 0 (default, no limit configured)", m["retention_limit"])
	}
}

func TestHandlePutProjectCleanup_RoundTrip(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCleanupRequest(t, s, s.handlePutProjectCleanup, http.MethodPut, projectID,
		`{"enabled":true,"retention_limit":500}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	rr2 := doCleanupRequest(t, s, s.handleGetProjectCleanup, http.MethodGet, projectID, "")
	if rr2.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rr2.Code)
	}
	m := decodeCleanupResponse(t, rr2)
	if m["enabled"] != true {
		t.Errorf("enabled = %v, want true", m["enabled"])
	}
	if m["retention_limit"] != float64(500) {
		t.Errorf("retention_limit = %v, want 500", m["retention_limit"])
	}
}

func TestHandlePutProjectCleanup_InvalidRetentionLimit(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCleanupRequest(t, s, s.handlePutProjectCleanup, http.MethodPut, projectID,
		`{"enabled":true,"retention_limit":5}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for retention_limit=5 (< 10)", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "10") {
		t.Errorf("error body should mention minimum 10; got: %s", rr.Body.String())
	}
}

func TestHandlePutProjectCleanup_BadBody(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCleanupRequest(t, s, s.handlePutProjectCleanup, http.MethodPut, projectID, `{not json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed JSON", rr.Code)
	}
}

func TestHandlePutProjectCleanup_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects//settings/cleanup",
		strings.NewReader(`{"enabled":true}`))
	rr := httptest.NewRecorder()
	s.handlePutProjectCleanup(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when project id is missing", rr.Code)
	}
}
