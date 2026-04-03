package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// buildCreateProjectReq builds a POST /api/v1/projects request with the given JSON body.
func buildCreateProjectReq(t *testing.T, body string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
}

// TestHandleCreateProject_WithoutSafetyHook verifies backward compatibility: omitting
// claude_safety_hook creates the project successfully with null for that field.
func TestHandleCreateProject_WithoutSafetyHook(t *testing.T) {
	s := newProjectsServer(t)

	body, _ := json.Marshal(map[string]interface{}{
		"id":   "create-no-hook",
		"name": "No Hook Project",
	})
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)
	if _, ok := resp["claude_safety_hook"]; !ok {
		t.Fatal("response missing claude_safety_hook field")
	}
	if resp["claude_safety_hook"] != nil {
		t.Errorf("claude_safety_hook = %v, want nil when not provided", resp["claude_safety_hook"])
	}
}

// TestHandleCreateProject_WithValidSafetyHook verifies that providing a valid
// claude_safety_hook JSON persists the config and the response includes it.
func TestHandleCreateProject_WithValidSafetyHook(t *testing.T) {
	s := newProjectsServer(t)

	body, _ := json.Marshal(map[string]interface{}{
		"id":                 "create-with-hook",
		"name":               "Hook Project",
		"claude_safety_hook": validSafetyHookJSON,
	})
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)
	hook, ok := resp["claude_safety_hook"]
	if !ok {
		t.Fatal("response missing claude_safety_hook field")
	}
	if hook == nil {
		t.Fatal("claude_safety_hook is nil, want non-nil")
	}
	hookStr, ok := hook.(string)
	if !ok {
		t.Fatalf("claude_safety_hook type = %T, want string", hook)
	}
	if hookStr != validSafetyHookJSON {
		t.Errorf("claude_safety_hook = %q, want %q", hookStr, validSafetyHookJSON)
	}
}

// TestHandleCreateProject_InvalidSafetyHookJSON verifies that an invalid JSON value
// for claude_safety_hook returns 400.
func TestHandleCreateProject_InvalidSafetyHookJSON(t *testing.T) {
	s := newProjectsServer(t)

	cases := []struct {
		name    string
		hookVal string
	}{
		{"plain text", "not-json"},
		{"partial JSON", `{"enabled":true`},
		{"number string", "42"},
		{"array JSON", "[1,2,3]"},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectID := "create-invalid-hook-" + string(rune('a'+i))
			body, _ := json.Marshal(map[string]interface{}{
				"id":                 projectID,
				"claude_safety_hook": tc.hookVal,
			})
			rr := httptest.NewRecorder()
			s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))

			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, "invalid claude_safety_hook")
		})
	}
}

// TestHandleCreateProject_MissingID verifies that omitting the id field returns 400.
func TestHandleCreateProject_MissingID(t *testing.T) {
	s := newProjectsServer(t)

	body := `{"name":"No ID Project"}`
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, body))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleCreateProject_SafetyHookPersistedToConfig verifies that after creating a
// project with a safety hook, a subsequent GET returns the same stored hook value.
func TestHandleCreateProject_SafetyHookPersistedToConfig(t *testing.T) {
	s := newProjectsServer(t)

	body, _ := json.Marshal(map[string]interface{}{
		"id":                 "create-persist-hook",
		"claude_safety_hook": validSafetyHookJSON,
	})
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	// Verify via GET that the hook is persisted in the config table.
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "create-persist-hook"))
	if rrGet.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body: %s", rrGet.Code, rrGet.Body.String())
	}

	resp := decodeProjectResp(t, rrGet)
	hookStr, ok := resp["claude_safety_hook"].(string)
	if !ok {
		t.Fatalf("GET claude_safety_hook type = %T, want string", resp["claude_safety_hook"])
	}
	if hookStr != validSafetyHookJSON {
		t.Errorf("GET claude_safety_hook = %q, want %q", hookStr, validSafetyHookJSON)
	}
}

// TestHandleCreateProject_SafetyHookVisibleInList verifies that after creating a project
// with a safety hook, GET /projects includes the hook for that project.
func TestHandleCreateProject_SafetyHookVisibleInList(t *testing.T) {
	s := newProjectsServer(t)

	body, _ := json.Marshal(map[string]interface{}{
		"id":                 "create-list-hook",
		"claude_safety_hook": validSafetyHookJSON,
	})
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

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

	for _, p := range listResp.Projects {
		if p["id"] == "create-list-hook" {
			hookStr, ok := p["claude_safety_hook"].(string)
			if !ok {
				t.Fatalf("claude_safety_hook type = %T, want string", p["claude_safety_hook"])
			}
			if hookStr != validSafetyHookJSON {
				t.Errorf("list claude_safety_hook = %q, want %q", hookStr, validSafetyHookJSON)
			}
			return
		}
	}
	t.Fatal("create-list-hook missing from list response")
}

// TestHandleCreateProject_EmptyStringSafetyHookOmitted verifies that an empty string
// value for claude_safety_hook does not persist any config (treated as omitted).
func TestHandleCreateProject_EmptyStringSafetyHookOmitted(t *testing.T) {
	s := newProjectsServer(t)

	body, _ := json.Marshal(map[string]interface{}{
		"id":                 "create-empty-hook",
		"claude_safety_hook": "",
	})
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	// GET should return null (empty string skips persistence).
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "create-empty-hook"))
	if rrGet.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rrGet.Code)
	}
	resp := decodeProjectResp(t, rrGet)
	if resp["claude_safety_hook"] != nil {
		t.Errorf("claude_safety_hook = %v, want nil for empty-string input", resp["claude_safety_hook"])
	}
}

// TestHandleCreateProject_InvalidRequestBody verifies that a malformed JSON body returns 400.
func TestHandleCreateProject_InvalidRequestBody(t *testing.T) {
	s := newProjectsServer(t)

	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, "not-json{{{"))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleCreateProject_NameDefaultsToID verifies that when name is omitted the project
// name is set to the ID value.
func TestHandleCreateProject_NameDefaultsToID(t *testing.T) {
	s := newProjectsServer(t)

	body, _ := json.Marshal(map[string]interface{}{"id": "create-name-default"})
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	resp := decodeProjectResp(t, rr)
	if name, _ := resp["name"].(string); name != "create-name-default" {
		t.Errorf("name = %q, want %q (should default to id)", name, "create-name-default")
	}
}

// TestHandleCreateProject_WithDangerousPatterns verifies that a safety hook with populated
// dangerous_patterns is persisted and returned correctly (core ticket requirement).
func TestHandleCreateProject_WithDangerousPatterns(t *testing.T) {
	s := newProjectsServer(t)

	const hookWithPatterns = `{"enabled":true,"allow_git":true,"rm_rf_allowed_paths":[],"dangerous_patterns":["DROP TABLE","rm -rf /",":(){:|:&};:","--hard","chmod -R 777 /"]}`
	body, _ := json.Marshal(map[string]interface{}{
		"id":                 "create-with-patterns",
		"claude_safety_hook": hookWithPatterns,
	})
	rr := httptest.NewRecorder()
	s.handleCreateProject(rr, buildCreateProjectReq(t, string(body)))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)
	hookStr, ok := resp["claude_safety_hook"].(string)
	if !ok {
		t.Fatalf("claude_safety_hook type = %T, want string", resp["claude_safety_hook"])
	}
	if hookStr != hookWithPatterns {
		t.Errorf("claude_safety_hook = %q, want %q", hookStr, hookWithPatterns)
	}

	// Verify persistence via a separate GET.
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "create-with-patterns"))
	if rrGet.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rrGet.Code)
	}
	getRespBody := decodeProjectResp(t, rrGet)
	if getRespBody["claude_safety_hook"] != hookWithPatterns {
		t.Errorf("GET claude_safety_hook = %v, want %q", getRespBody["claude_safety_hook"], hookWithPatterns)
	}
}
