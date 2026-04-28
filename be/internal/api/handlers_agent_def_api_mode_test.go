package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// newAgentDefAPIModeServer creates a Server with given apiMode plus a seeded project
// and workflow for agent-definition handler tests. Returns (server, projectID, workflowID).
func newAgentDefAPIModeServer(t *testing.T, apiMode bool) (*Server, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agentdef_apimode_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj1", "Test", "/tmp", now, now,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"proj1", "wf1", "", "ticket", now, now,
	); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	s := &Server{pool: pool, clock: clock.Real(), apiMode: apiMode}
	return s, "proj1", "wf1"
}

// postAgentDefRequest makes a POST to handleCreateAgentDef with the given body.
// It sets ?project= and wid path value via SetPathValue.
func postAgentDefRequest(t *testing.T, s *Server, projectID, workflowID, body string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/workflows/" + workflowID + "/agents?project=" + projectID
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.SetPathValue("wid", workflowID)
	rr := httptest.NewRecorder()
	s.handleCreateAgentDef(rr, req)
	return rr
}

// patchAgentDefRequest makes a PATCH to handleUpdateAgentDef with the given body.
func patchAgentDefRequest(t *testing.T, s *Server, projectID, workflowID, agentID, body string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/workflows/" + workflowID + "/agents/" + agentID + "?project=" + projectID
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(body))
	req.SetPathValue("wid", workflowID)
	req.SetPathValue("id", agentID)
	rr := httptest.NewRecorder()
	s.handleUpdateAgentDef(rr, req)
	return rr
}

// TestHandleCreateAgentDef_APIModeDisabled verifies that POST with execution_mode=api
// returns 400 with error "api_mode_disabled" when the server is in cli mode.
func TestHandleCreateAgentDef_APIModeDisabled(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"api-agent","prompt":"do stuff","execution_mode":"api"}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "api_mode_disabled" {
		t.Errorf("error = %q, want %q", resp["error"], "api_mode_disabled")
	}
}

// TestHandleCreateAgentDef_APIModeEnabled_Succeeds verifies that POST with
// execution_mode=api returns 201 when the server is in api mode.
func TestHandleCreateAgentDef_APIModeEnabled_Succeeds(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, true)

	rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"api-agent-ok","prompt":"do stuff","execution_mode":"api"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleCreateAgentDef_CLIMode_UnaffectedByAPIMode verifies that creating
// a cli-mode agent succeeds even when the server was not started with --mode=api.
func TestHandleCreateAgentDef_CLIMode_UnaffectedByAPIMode(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"cli-agent","prompt":"do stuff","execution_mode":"cli"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleUpdateAgentDef_APIModeDisabled verifies that PATCH with execution_mode=api
// returns 400 with error "api_mode_disabled" when the server is in cli mode.
func TestHandleUpdateAgentDef_APIModeDisabled(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	// First create a CLI agent to update
	if rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"upd-to-api","prompt":"do stuff","execution_mode":"cli"}`); rr.Code != http.StatusCreated {
		t.Fatalf("setup: create agent status = %d, body=%s", rr.Code, rr.Body.String())
	}

	rr := patchAgentDefRequest(t, s, pid, wid, "upd-to-api",
		`{"execution_mode":"api"}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "api_mode_disabled" {
		t.Errorf("error = %q, want %q", resp["error"], "api_mode_disabled")
	}
}
