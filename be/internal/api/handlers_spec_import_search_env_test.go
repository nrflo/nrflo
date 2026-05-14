package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/spec_import"
)

// seedJiraEnvVars inserts the three required Jira env vars for projectID into the DB.
func seedJiraEnvVars(t *testing.T, s *Server, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	vars := [][2]string{
		{"JIRA_BASE_URL", "https://jira.example.com"},
		{"JIRA_EMAIL", "user@example.com"},
		{"JIRA_API_TOKEN", "secret-token"},
	}
	for _, kv := range vars {
		if _, err := s.pool.Exec(
			`INSERT INTO project_env_vars (project_id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			projectID, kv[0], kv[1], now, now,
		); err != nil {
			t.Fatalf("seed env var %s: %v", kv[0], err)
		}
	}
}

// TestHandleJiraSearch_MissingProjectID_400 verifies the guard added by the implementor.
func TestHandleJiraSearch_MissingProjectID_400(t *testing.T) {
	s, _ := newSpecImportServer(t)
	stub := &stubJiraAdapter{
		stubSpecImportAdapter: stubSpecImportAdapter{src: spec_import.SourceJira},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/jira/search?q=test", nil)
	// No injectProject — context has no project, getProjectID returns "".
	rr := httptest.NewRecorder()
	s.handleJiraSearch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "project ID required (X-Project header)" {
		t.Errorf("error = %q, want 'project ID required (X-Project header)'", resp["error"])
	}
}

// TestHandleGitHubSearch_MissingProjectID_400 mirrors the Jira case for the GitHub search endpoint.
func TestHandleGitHubSearch_MissingProjectID_400(t *testing.T) {
	s, _ := newSpecImportServer(t)
	stub := &stubSpecImportAdapter{src: spec_import.SourceGitHubIssue}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/github/search?q=test", nil)
	// No injectProject — context has no project, getProjectID returns "".
	rr := httptest.NewRecorder()
	s.handleGitHubSearch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "project ID required (X-Project header)" {
		t.Errorf("error = %q, want 'project ID required (X-Project header)'", resp["error"])
	}
}

// TestHandleJiraSearch_EnvPropagated_200 verifies that project_env_vars are loaded and forwarded
// to the adapter's Search call, and that the handler returns 200 when search succeeds.
func TestHandleJiraSearch_EnvPropagated_200(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	seedJiraEnvVars(t, s, projectID)

	stub := &stubJiraAdapter{
		stubSpecImportAdapter: stubSpecImportAdapter{
			src: spec_import.SourceJira,
			jiraResults: []spec_import.JiraIssueSummary{
				{Key: "PROJ-42", Summary: "Some issue", Status: "In Progress"},
			},
		},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/jira/search?q=some+issue", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleJiraSearch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Assert all three Jira env vars were forwarded to the adapter.
	wantEnv := map[string]string{
		"JIRA_BASE_URL":  "https://jira.example.com",
		"JIRA_EMAIL":     "user@example.com",
		"JIRA_API_TOKEN": "secret-token",
	}
	for k, want := range wantEnv {
		if got := stub.capturedEnv[k]; got != want {
			t.Errorf("capturedEnv[%q] = %q, want %q", k, got, want)
		}
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	results, _ := resp["results"].([]interface{})
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

// TestHandleJiraSearch_NoEnvVars_412 verifies that when a project has no env vars seeded,
// the adapter receives an empty env map and the 412 missing_env response is returned.
func TestHandleJiraSearch_NoEnvVars_412(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	// Intentionally do not seed any project_env_vars.

	stub := &stubJiraAdapter{
		stubSpecImportAdapter: stubSpecImportAdapter{
			src: spec_import.SourceJira,
			jiraErr: spec_import.MissingEnvError{
				Source:  spec_import.SourceJira,
				Missing: []string{"JIRA_BASE_URL", "JIRA_EMAIL", "JIRA_API_TOKEN"},
			},
		},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/jira/search?q=anything", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleJiraSearch(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; body: %s", rr.Code, rr.Body.String())
	}

	// Adapter must have received an empty env (no vars were seeded).
	if len(stub.capturedEnv) != 0 {
		t.Errorf("capturedEnv = %v, want empty map (no project env vars seeded)", stub.capturedEnv)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "missing_env" {
		t.Errorf("error = %v, want missing_env", resp["error"])
	}
	missing, _ := resp["missing"].([]interface{})
	wantMissing := []string{"JIRA_BASE_URL", "JIRA_EMAIL", "JIRA_API_TOKEN"}
	if len(missing) != len(wantMissing) {
		t.Errorf("len(missing) = %d, want %d; got %v", len(missing), len(wantMissing), missing)
	} else {
		for i, want := range wantMissing {
			if got, _ := missing[i].(string); got != want {
				t.Errorf("missing[%d] = %q, want %q", i, got, want)
			}
		}
	}
}
