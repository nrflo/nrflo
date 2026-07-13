package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// validPlanManifestJSON is a minimal, valid v1 plan manifest: single layer,
// single node, which also satisfies the final-layer-exactly-one-node rule.
const validPlanManifestJSON = `{"version":1,"goal":"do the thing","layers":[{"layer":0,"policy":"any","nodes":[{"id":"n1","template":"tmpl1","instructions":"do it"}]}]}`

// invalidPlanManifestJSON is missing "goal" and has an unknown top-level field,
// so it fails both DisallowUnknownFields (parse) and required-goal (semantic).
const invalidPlanManifestJSON = `{"version":1,"unknown_field":true,"layers":[{"layer":0,"policy":"any","nodes":[{"id":"n1","template":"tmpl1","instructions":"do it"}]}]}`

// newPlanServer creates a minimal Server (no wsHub) for plan handler tests
// that don't need to assert broadcasts.
func newPlanServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "plan_handler_test.db")
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

// seedPlanInstance seeds a project, a workflow with one fanout_template agent
// definition (id "tmpl1"), and a workflow instance under that project.
func seedPlanInstance(t *testing.T, s *Server, projectID, instanceID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := s.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed project %q: %v", projectID, err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, created_at, updated_at)
		 VALUES ('wf-test', ?, '', 'project', '[]', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed workflow for project %q: %v", projectID, err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, node_role, consultant, model, execution_mode, created_at, updated_at)
		 VALUES ('tmpl1', ?, 'wf-test', 'fanout_template', 0, 'sonnet', 'cli_interactive', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed agent definition for project %q: %v", projectID, err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES (?, ?, '', 'wf-test', 'project', 'active', 0, ?, ?)`,
		instanceID, projectID, now, now,
	); err != nil {
		t.Fatalf("seed workflow instance %q: %v", instanceID, err)
	}
}

// planReviseReq builds a POST request for .../plan/revise with the given body.
func planReviseReq(t *testing.T, iid string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workflow-instances/"+iid+"/plan/revise", bytes.NewReader(body))
	req.SetPathValue("iid", iid)
	return req
}

// planApproveReq builds a POST request for .../plan/approve with the given body.
func planApproveReq(t *testing.T, iid string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workflow-instances/"+iid+"/plan/approve", bytes.NewReader(body))
	req.SetPathValue("iid", iid)
	return req
}

// planGetReq builds a GET request for .../plan.
func planGetReq(t *testing.T, iid string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances/"+iid+"/plan", nil)
	req.SetPathValue("iid", iid)
	return req
}

// decodePlanRevision decodes a model.PlanRevision from a response recorder.
func decodePlanRevision(t *testing.T, rr *httptest.ResponseRecorder) *model.PlanRevision {
	t.Helper()
	var rev model.PlanRevision
	if err := json.NewDecoder(rr.Body).Decode(&rev); err != nil {
		t.Fatalf("decode PlanRevision: %v; body: %s", err, rr.Body.String())
	}
	return &rev
}
