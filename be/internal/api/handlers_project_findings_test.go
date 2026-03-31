package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/types"
)

func newProjectFindingsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "project_findings_handler_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// projectFindingsReq builds a GET request for GET /api/v1/projects/{id}/findings.
func projectFindingsReq(t *testing.T, projectID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/findings", nil)
	req.SetPathValue("id", projectID)
	return req
}

// decodeMapResponse decodes a map[string]interface{} from the response recorder.
func decodeMapResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode map response: %v", err)
	}
	return resp
}

// seedProject inserts a minimal project row to satisfy FK constraints.
func seedProjectForFindings(t *testing.T, s *Server, projectID string) {
	t.Helper()
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		projectID,
	)
	if err != nil {
		t.Fatalf("seed project %q: %v", projectID, err)
	}
}

// addProjectFinding seeds a single key-value finding using the service.
func addProjectFinding(t *testing.T, s *Server, projectID, key, value string) {
	t.Helper()
	seedProjectForFindings(t, s, projectID)
	svc := service.NewProjectFindingsService(s.pool, s.clock)
	if err := svc.Add(projectID, &types.ProjectFindingsAddRequest{Key: key, Value: value}); err != nil {
		t.Fatalf("add project finding %q=%q: %v", key, value, err)
	}
}

// TestHandleGetProjectFindings_EmptyReturnsEmptyMap verifies fresh project returns {}.
func TestHandleGetProjectFindings_EmptyReturnsEmptyMap(t *testing.T) {
	s := newProjectFindingsServer(t)

	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, projectFindingsReq(t, "proj-empty"))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeMapResponse(t, rr)
	if len(resp) != 0 {
		t.Errorf("len(findings) = %d, want 0; got: %v", len(resp), resp)
	}
}

// TestHandleGetProjectFindings_MissingProjectID verifies empty path value returns 400.
func TestHandleGetProjectFindings_MissingProjectID(t *testing.T) {
	s := newProjectFindingsServer(t)

	// Build request without setting the path value (simulates empty id).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//findings", nil)
	// PathValue("id") returns "" when not set.
	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID")
}

// TestHandleGetProjectFindings_SingleFinding verifies a single string finding is returned.
func TestHandleGetProjectFindings_SingleFinding(t *testing.T) {
	s := newProjectFindingsServer(t)
	addProjectFinding(t, s, "proj-single", "status", "done")

	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, projectFindingsReq(t, "proj-single"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeMapResponse(t, rr)
	if len(resp) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(resp))
	}
	if v, ok := resp["status"]; !ok {
		t.Error("response missing 'status' key")
	} else if v != "done" {
		t.Errorf("findings['status'] = %v, want 'done'", v)
	}
}

// TestHandleGetProjectFindings_MultipleFindings verifies multiple findings are returned.
func TestHandleGetProjectFindings_MultipleFindings(t *testing.T) {
	s := newProjectFindingsServer(t)
	addProjectFinding(t, s, "proj-multi", "key1", "val1")
	addProjectFinding(t, s, "proj-multi", "key2", "val2")
	addProjectFinding(t, s, "proj-multi", "key3", "val3")

	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, projectFindingsReq(t, "proj-multi"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeMapResponse(t, rr)
	if len(resp) != 3 {
		t.Fatalf("len(findings) = %d, want 3; got: %v", len(resp), resp)
	}
	for _, key := range []string{"key1", "key2", "key3"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("response missing key %q", key)
		}
	}
}

// TestHandleGetProjectFindings_JSONValueParsed verifies JSON object values are returned as parsed objects.
func TestHandleGetProjectFindings_JSONValueParsed(t *testing.T) {
	s := newProjectFindingsServer(t)
	addProjectFinding(t, s, "proj-json", "config", `{"enabled":true,"count":42}`)

	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, projectFindingsReq(t, "proj-json"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeMapResponse(t, rr)
	raw, ok := resp["config"]
	if !ok {
		t.Fatal("response missing 'config' key")
	}

	// The value should be a parsed JSON object, not a raw string.
	obj, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("findings['config'] type = %T, want map[string]interface{}", raw)
	}
	if obj["enabled"] != true {
		t.Errorf("config.enabled = %v, want true", obj["enabled"])
	}
	if obj["count"] != float64(42) {
		t.Errorf("config.count = %v, want 42", obj["count"])
	}
}

// TestHandleGetProjectFindings_JSONArrayValue verifies JSON array values are returned as parsed arrays.
func TestHandleGetProjectFindings_JSONArrayValue(t *testing.T) {
	s := newProjectFindingsServer(t)
	addProjectFinding(t, s, "proj-arr", "items", `["a","b","c"]`)

	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, projectFindingsReq(t, "proj-arr"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeMapResponse(t, rr)
	raw, ok := resp["items"]
	if !ok {
		t.Fatal("response missing 'items' key")
	}
	arr, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("findings['items'] type = %T, want []interface{}", raw)
	}
	if len(arr) != 3 {
		t.Errorf("len(items) = %d, want 3", len(arr))
	}
}

// TestHandleGetProjectFindings_DifferentProjects verifies findings are scoped to the requested project.
func TestHandleGetProjectFindings_DifferentProjects(t *testing.T) {
	s := newProjectFindingsServer(t)
	addProjectFinding(t, s, "proj-a", "owner", "alice")
	addProjectFinding(t, s, "proj-b", "owner", "bob")

	// Request proj-a: should only see alice.
	rrA := httptest.NewRecorder()
	s.handleGetProjectFindings(rrA, projectFindingsReq(t, "proj-a"))
	if rrA.Code != http.StatusOK {
		t.Fatalf("proj-a status = %d, want 200", rrA.Code)
	}
	respA := decodeMapResponse(t, rrA)
	if len(respA) != 1 {
		t.Fatalf("proj-a: len(findings) = %d, want 1", len(respA))
	}
	if respA["owner"] != "alice" {
		t.Errorf("proj-a: owner = %v, want 'alice'", respA["owner"])
	}

	// Request proj-b: should only see bob.
	rrB := httptest.NewRecorder()
	s.handleGetProjectFindings(rrB, projectFindingsReq(t, "proj-b"))
	if rrB.Code != http.StatusOK {
		t.Fatalf("proj-b status = %d, want 200", rrB.Code)
	}
	respB := decodeMapResponse(t, rrB)
	if respB["owner"] != "bob" {
		t.Errorf("proj-b: owner = %v, want 'bob'", respB["owner"])
	}
}

// TestHandleGetProjectFindings_UpsertOverwrites verifies that re-adding the same key returns updated value.
func TestHandleGetProjectFindings_UpsertOverwrites(t *testing.T) {
	s := newProjectFindingsServer(t)
	addProjectFinding(t, s, "proj-upsert", "phase", "init")
	addProjectFinding(t, s, "proj-upsert", "phase", "done")

	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, projectFindingsReq(t, "proj-upsert"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	resp := decodeMapResponse(t, rr)
	if len(resp) != 1 {
		t.Errorf("len(findings) = %d, want 1 (upsert should not duplicate)", len(resp))
	}
	if resp["phase"] != "done" {
		t.Errorf("phase = %v, want 'done'", resp["phase"])
	}
}

// TestHandleGetProjectFindings_ContentTypeJSON verifies Content-Type is application/json.
func TestHandleGetProjectFindings_ContentTypeJSON(t *testing.T) {
	s := newProjectFindingsServer(t)

	rr := httptest.NewRecorder()
	s.handleGetProjectFindings(rr, projectFindingsReq(t, "proj-ct"))

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}
