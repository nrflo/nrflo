package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// newWorkflowMIServer creates a minimal Server for workflow multi-instance handler tests.
func newWorkflowMIServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wf_mi_handler_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// seedTicketTwoInstances seeds a project, workflow definition, a ticket, and two workflow
// instances for the same ticket. Returns (inst1ID, inst2ID).
func seedTicketTwoInstances(t *testing.T, s *Server, projectID, ticketID string) (string, string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	earlier := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339Nano)

	if _, err := s.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_at, updated_at, created_by)
		 VALUES (?, ?, 'Test Ticket', 'open', 2, 'task', ?, ?, 'test')`,
		ticketID, projectID, now, now,
	); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, phases, groups, created_at, updated_at)
		 VALUES ('wf-mi', ?, '', 'ticket', '[{"agent":"agent1","layer":0}]', '[]', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed workflow def: %v", err)
	}

	inst1 := "mi-inst-1"
	inst2 := "mi-inst-2"
	for _, row := range []struct {
		id        string
		createdAt string
	}{
		{inst1, earlier},
		{inst2, now},
	} {
		if _, err := s.pool.Exec(
			`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
			 VALUES (?, ?, ?, 'wf-mi', 'ticket', 'active', '{}', 0, ?, ?)`,
			row.id, projectID, ticketID, row.createdAt, now,
		); err != nil {
			t.Fatalf("seed workflow instance %q: %v", row.id, err)
		}
	}
	return inst1, inst2
}

// TestHandleGetWorkflow_AllWorkflowsKeyedByInstanceID verifies that the response
// all_workflows map is keyed by instance_id (not workflow name) when multiple
// ticket workflow instances exist.
func TestHandleGetWorkflow_AllWorkflowsKeyedByInstanceID(t *testing.T) {
	s := newWorkflowMIServer(t)
	inst1, inst2 := seedTicketTwoInstances(t, s, "proj-miwf", "MIWF-1")

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/tickets/MIWF-1/workflow", "proj-miwf"), nil)
	req.SetPathValue("id", "MIWF-1")
	rr := httptest.NewRecorder()
	s.handleGetWorkflow(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// has_workflow must be true
	if hw, _ := body["has_workflow"].(bool); !hw {
		t.Error("expected has_workflow=true")
	}

	// all_workflows must be present and map-typed
	allWFs, ok := body["all_workflows"].(map[string]interface{})
	if !ok {
		t.Fatalf("all_workflows missing or wrong type: %T", body["all_workflows"])
	}

	// Keys must be instance IDs, not the workflow name
	if _, exists := allWFs[inst1]; !exists {
		t.Errorf("all_workflows missing key %q (instance 1 ID)", inst1)
	}
	if _, exists := allWFs[inst2]; !exists {
		t.Errorf("all_workflows missing key %q (instance 2 ID)", inst2)
	}
	if len(allWFs) != 2 {
		t.Errorf("all_workflows has %d entries, want 2", len(allWFs))
	}
	// Workflow name must NOT be a key
	if _, exists := allWFs["wf-mi"]; exists {
		t.Error("all_workflows should not use workflow name as key")
	}

	// Each state must carry instance_id and workflow fields
	for instID, stateRaw := range allWFs {
		st, ok := stateRaw.(map[string]interface{})
		if !ok {
			t.Errorf("state for %q is not a map", instID)
			continue
		}
		if st["instance_id"] != instID {
			t.Errorf("state[%q].instance_id = %v, want %q", instID, st["instance_id"], instID)
		}
		if st["workflow"] != "wf-mi" {
			t.Errorf("state[%q].workflow = %v, want %q", instID, st["workflow"], "wf-mi")
		}
	}

	// Deduplicated workflow name list must contain exactly the one workflow name
	wfs, _ := body["workflows"].([]interface{})
	if len(wfs) != 1 {
		t.Errorf("workflows list has %d entries, want 1 (deduped)", len(wfs))
	}
	if len(wfs) == 1 && wfs[0] != "wf-mi" {
		t.Errorf("workflows[0] = %v, want %q", wfs[0], "wf-mi")
	}
}

// TestHandleGetWorkflow_InstanceIDQueryParam_SelectsState verifies that ?instance_id=
// causes the top-level state field to reflect that specific workflow instance.
func TestHandleGetWorkflow_InstanceIDQueryParam_SelectsState(t *testing.T) {
	s := newWorkflowMIServer(t)
	inst1, _ := seedTicketTwoInstances(t, s, "proj-miwf2", "MIWF-2")

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/tickets/MIWF-2/workflow", "proj-miwf2")+"&instance_id="+inst1, nil)
	req.SetPathValue("id", "MIWF-2")
	rr := httptest.NewRecorder()
	s.handleGetWorkflow(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// The selected state must reflect inst1
	st, ok := body["state"].(map[string]interface{})
	if !ok {
		t.Fatalf("state is not a map: %T", body["state"])
	}
	if st["instance_id"] != inst1 {
		t.Errorf("state.instance_id = %v, want %q", st["instance_id"], inst1)
	}
}

// TestHandleGetAgentSessions_FilterByInstanceID verifies that the ?instance_id= query
// parameter limits the returned sessions to those belonging to that workflow instance.
func TestHandleGetAgentSessions_FilterByInstanceID(t *testing.T) {
	s := newWorkflowMIServer(t)
	inst1, inst2 := seedTicketTwoInstances(t, s, "proj-mias", "MIAS-1")

	// Seed one session per instance
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, row := range []struct {
		sessID string
		instID string
	}{
		{"sess-inst1", inst1},
		{"sess-inst2", inst2},
	} {
		if _, err := s.pool.Exec(`
			INSERT INTO agent_sessions
				(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
				 model_id, status, result, result_reason, pid, findings,
				 context_left, ancestor_session_id, spawn_command, prompt_context,
				 restart_count, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', NULL, 'completed', 'pass', NULL, NULL, NULL,
				NULL, NULL, NULL, NULL, 0, NULL, NULL, ?, ?)`,
			row.sessID, "proj-mias", "MIAS-1", row.instID, now, now,
		); err != nil {
			t.Fatalf("seed session %q: %v", row.sessID, err)
		}
	}

	// Without filter — must return all sessions (both instances)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/tickets/MIAS-1/agents", "proj-mias"), nil)
	req.SetPathValue("id", "MIAS-1")
	rr := httptest.NewRecorder()
	s.handleGetAgentSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("without filter: status = %d, want 200", rr.Code)
	}
	var allBody map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&allBody) //nolint:errcheck
	allSessions, _ := allBody["sessions"].([]interface{})
	if len(allSessions) != 2 {
		t.Errorf("without filter: %d sessions, want 2", len(allSessions))
	}

	// With instance_id filter — must return only the session for inst1
	req2 := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/tickets/MIAS-1/agents", "proj-mias")+"&instance_id="+inst1, nil)
	req2.SetPathValue("id", "MIAS-1")
	rr2 := httptest.NewRecorder()
	s.handleGetAgentSessions(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("with filter: status = %d, want 200", rr2.Code)
	}
	var filtBody map[string]interface{}
	json.NewDecoder(rr2.Body).Decode(&filtBody) //nolint:errcheck
	filtSessions, _ := filtBody["sessions"].([]interface{})
	if len(filtSessions) != 1 {
		t.Errorf("with instance_id filter: %d sessions, want 1", len(filtSessions))
	}
	if len(filtSessions) == 1 {
		sess, ok := filtSessions[0].(map[string]interface{})
		if !ok {
			t.Fatalf("filtered session is not a map")
		}
		if sess["workflow_instance_id"] != inst1 {
			t.Errorf("filtered session workflow_instance_id = %v, want %q", sess["workflow_instance_id"], inst1)
		}
	}
}
