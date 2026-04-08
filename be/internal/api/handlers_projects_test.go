package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func newProjectsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "projects_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// seedTestProject inserts a minimal project row for handler tests.
func seedTestProject(t *testing.T, s *Server, projectID string) {
	t.Helper()
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES (?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		projectID, projectID,
	)
	if err != nil {
		t.Fatalf("seed project %q: %v", projectID, err)
	}
}

func buildGetProjectReq(t *testing.T, projectID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID, nil)
	req.SetPathValue("id", projectID)
	return req
}

func buildPatchProjectReq(t *testing.T, projectID, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+projectID, strings.NewReader(body))
	req.SetPathValue("id", projectID)
	return req
}

func decodeProjectResp(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode project response: %v", err)
	}
	return resp
}

// validSafetyHookJSON is a minimal valid SafetyHookConfig JSON string.
const validSafetyHookJSON = `{"enabled":true,"allow_git":true,"rm_rf_allowed_paths":["node_modules","build"],"dangerous_patterns":[]}`

// patchBodyWithHook returns a PATCH body JSON where claude_safety_hook is a JSON-string value.
func patchBodyWithHook(hookJSON string) string {
	b, _ := json.Marshal(map[string]string{"claude_safety_hook": hookJSON})
	return string(b)
}

// TestHandleGetProject_SafetyHookNullWhenNotSet verifies GET returns null for claude_safety_hook
// when no config has been stored.
func TestHandleGetProject_SafetyHookNullWhenNotSet(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-a")

	rr := httptest.NewRecorder()
	s.handleGetProject(rr, buildGetProjectReq(t, "proj-a"))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)
	if _, ok := resp["claude_safety_hook"]; !ok {
		t.Fatal("response missing claude_safety_hook field")
	}
	if resp["claude_safety_hook"] != nil {
		t.Errorf("claude_safety_hook = %v, want nil", resp["claude_safety_hook"])
	}
}

// TestHandlePatchProject_ValidSafetyHook verifies PATCH with valid JSON saves the config
// and the response includes the stored value.
func TestHandlePatchProject_ValidSafetyHook(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-b")

	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "proj-b", patchBodyWithHook(validSafetyHookJSON)))

	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)
	hook, ok := resp["claude_safety_hook"]
	if !ok {
		t.Fatal("response missing claude_safety_hook field")
	}
	if hook == nil {
		t.Fatal("claude_safety_hook is nil, want non-nil")
	}
	// The value returned is a string (the stored JSON).
	hookStr, ok := hook.(string)
	if !ok {
		t.Fatalf("claude_safety_hook type = %T, want string", hook)
	}
	if hookStr != validSafetyHookJSON {
		t.Errorf("claude_safety_hook = %q, want %q", hookStr, validSafetyHookJSON)
	}
}

// TestHandlePatchProject_InvalidSafetyHookJSON verifies PATCH with non-JSON value returns 400.
func TestHandlePatchProject_InvalidSafetyHookJSON(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-c")

	cases := []struct {
		name string
		body string
	}{
		{"plain text", `{"claude_safety_hook":"not-json"}`},
		{"partial JSON", `{"claude_safety_hook":"{\"enabled\":true"}`},
		{"number", `{"claude_safety_hook":"42"}`},
		{"array JSON", `{"claude_safety_hook":"[1,2,3]"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			s.handleUpdateProject(rr, buildPatchProjectReq(t, "proj-c", tc.body))

			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, "invalid claude_safety_hook")
		})
	}
}

// TestHandlePatchProject_EmptyStringClearsSafetyHook verifies PATCH with empty string
// clears the config so GET returns null.
func TestHandlePatchProject_EmptyStringClearsSafetyHook(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-d")

	// First set a value.
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "proj-d",
		patchBodyWithHook(validSafetyHookJSON)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// Clear with empty string.
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "proj-d", `{"claude_safety_hook":""}`))
	if rr2.Code != http.StatusOK {
		t.Fatalf("clear PATCH status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}

	// GET should return null.
	rr3 := httptest.NewRecorder()
	s.handleGetProject(rr3, buildGetProjectReq(t, "proj-d"))
	if rr3.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr3.Code)
	}
	resp := decodeProjectResp(t, rr3)
	if resp["claude_safety_hook"] != nil {
		t.Errorf("after clear: claude_safety_hook = %v, want nil", resp["claude_safety_hook"])
	}
}

// TestHandlePatchProject_OmittedFieldPreservesSafetyHook verifies PATCH without
// claude_safety_hook field does not modify an existing config value.
func TestHandlePatchProject_OmittedFieldPreservesSafetyHook(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-e")

	// Set safety hook.
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "proj-e",
		patchBodyWithHook(validSafetyHookJSON)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// PATCH only the name — no claude_safety_hook field.
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "proj-e", `{"name":"updated-name"}`))
	if rr2.Code != http.StatusOK {
		t.Fatalf("name PATCH status = %d, want 200", rr2.Code)
	}

	// Safety hook should still be present.
	rr3 := httptest.NewRecorder()
	s.handleGetProject(rr3, buildGetProjectReq(t, "proj-e"))
	if rr3.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr3.Code)
	}
	resp := decodeProjectResp(t, rr3)
	if resp["claude_safety_hook"] == nil {
		t.Error("claude_safety_hook was unexpectedly cleared by unrelated PATCH")
	}
}

// TestHandleGetProject_SafetyHookRoundTrip verifies full round-trip: PATCH sets value,
// GET returns it, PATCH clears it, GET returns null.
func TestHandleGetProject_SafetyHookRoundTrip(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-rt")

	// Set
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "proj-rt",
		patchBodyWithHook(validSafetyHookJSON)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("set PATCH status = %d, want 200", rr1.Code)
	}

	// GET — must contain the hook
	rr2 := httptest.NewRecorder()
	s.handleGetProject(rr2, buildGetProjectReq(t, "proj-rt"))
	resp2 := decodeProjectResp(t, rr2)
	if resp2["claude_safety_hook"] == nil {
		t.Fatal("GET after set: claude_safety_hook is nil")
	}

	// Clear
	rr3 := httptest.NewRecorder()
	s.handleUpdateProject(rr3, buildPatchProjectReq(t, "proj-rt", `{"claude_safety_hook":""}`))
	if rr3.Code != http.StatusOK {
		t.Fatalf("clear PATCH status = %d, want 200", rr3.Code)
	}

	// GET — must return null
	rr4 := httptest.NewRecorder()
	s.handleGetProject(rr4, buildGetProjectReq(t, "proj-rt"))
	resp4 := decodeProjectResp(t, rr4)
	if resp4["claude_safety_hook"] != nil {
		t.Errorf("GET after clear: claude_safety_hook = %v, want nil", resp4["claude_safety_hook"])
	}
}

// TestHandleListProjects_IncludesSafetyHook verifies GET /projects returns claude_safety_hook
// for each project: null when unset, string when set.
func TestHandleListProjects_IncludesSafetyHook(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-list-1")
	seedTestProject(t, s, "proj-list-2")

	// Set hook on proj-list-1 only.
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "proj-list-1",
		patchBodyWithHook(validSafetyHookJSON)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	// List
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	listRR := httptest.NewRecorder()
	s.handleListProjects(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listRR.Code, listRR.Body.String())
	}

	var listResp struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Projects) < 2 {
		t.Fatalf("len(projects) = %d, want >= 2", len(listResp.Projects))
	}

	byID := make(map[string]map[string]interface{})
	for _, p := range listResp.Projects {
		id, _ := p["id"].(string)
		byID[id] = p
	}

	p1, ok := byID["proj-list-1"]
	if !ok {
		t.Fatal("proj-list-1 missing from list")
	}
	if p1["claude_safety_hook"] == nil {
		t.Error("proj-list-1: claude_safety_hook should be non-nil")
	}

	p2, ok := byID["proj-list-2"]
	if !ok {
		t.Fatal("proj-list-2 missing from list")
	}
	if p2["claude_safety_hook"] != nil {
		t.Errorf("proj-list-2: claude_safety_hook = %v, want nil", p2["claude_safety_hook"])
	}
}

// TestHandlePatchProject_DisabledSafetyHookAccepted verifies that a JSON object with
// enabled:false is accepted (valid JSON — the API does not reject disabled configs).
func TestHandlePatchProject_DisabledSafetyHookAccepted(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "proj-dis")

	const disabledHook = `{"enabled":false,"allow_git":true,"rm_rf_allowed_paths":[],"dangerous_patterns":[]}`
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "proj-dis", patchBodyWithHook(disabledHook)))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// GET should return the stored string.
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "proj-dis"))
	resp := decodeProjectResp(t, rrGet)
	if resp["claude_safety_hook"] == nil {
		t.Error("claude_safety_hook is nil, want stored disabled config")
	}
}
