package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/ws"
)

// insertChainForTest inserts a minimal workflow_chain row so that validateChainIDs passes.
func insertChainForTest(t *testing.T, s *Server, projectID, chainID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(
		`INSERT INTO workflow_chains (id, project_id, name, description, created_at, updated_at)
		 VALUES (?, ?, ?, '', ?, ?)`,
		chainID, projectID, "test-chain-"+chainID, now, now,
	)
	if err != nil {
		t.Fatalf("insertChainForTest(%q, %q): %v", projectID, chainID, err)
	}
}

// createScheduledTaskWithChains creates a task with workflow_chain_ids via handler.
func createScheduledTaskWithChains(t *testing.T, s *Server, projectID, taskID string, workflows, chainIDs []string) *model.ScheduledTask {
	t.Helper()
	wfsJSON, _ := json.Marshal(workflows)
	chainsJSON, _ := json.Marshal(chainIDs)
	body := `{"id":"` + taskID + `","name":"Task ` + taskID + `","cron_expression":"* * * * *","workflows":` +
		string(wfsJSON) + `,"workflow_chain_ids":` + string(chainsJSON) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduled-tasks", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleCreateScheduledTask(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createScheduledTaskWithChains(%q): status = %d, body: %s", taskID, rr.Code, rr.Body.String())
	}
	return decodeScheduledTask(t, rr)
}

// -- Create with workflow_chain_ids --

// TestHandleCreateScheduledTask_EmptyBothFields_Returns400 verifies that providing
// empty workflows AND empty workflow_chain_ids returns 400 with workflows_required.
func TestHandleCreateScheduledTask_EmptyBothFields_Returns400(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-empty-both-h", "wf-empty-both-h", "project")

	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-empty-both-h",
		`{"name":"X","cron_expression":"* * * * *","workflows":[],"workflow_chain_ids":[]}`,
		nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "workflows_required")
}

// TestHandleCreateScheduledTask_WithWorkflowChainIDs verifies that a task created
// with workflow_chain_ids includes them in the response.
func TestHandleCreateScheduledTask_WithWorkflowChainIDs(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-wchain", "wf-wchain", "project")
	insertChainForTest(t, s, "proj-wchain", "chain-resp")

	task := createScheduledTaskWithChains(t, s, "proj-wchain", "task-wchain",
		[]string{"wf-wchain"}, []string{"chain-resp"})

	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "chain-resp" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-resp]", task.WorkflowChainIDs)
	}
	if len(task.Workflows) != 1 || task.Workflows[0] != "wf-wchain" {
		t.Errorf("Workflows = %v, want [wf-wchain]", task.Workflows)
	}
}

// TestHandleCreateScheduledTask_ChainIDsOnly_Returns201 verifies that creating a
// task with only chain IDs (no workflows) returns 201.
func TestHandleCreateScheduledTask_ChainIDsOnly_Returns201(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-chain-only-h", "wf-chain-only-h", "project")
	insertChainForTest(t, s, "proj-chain-only-h", "chain-only-1")

	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-chain-only-h",
		`{"id":"task-chain-only-h","name":"Chain Only","cron_expression":"* * * * *","workflows":[],"workflow_chain_ids":["chain-only-1"]}`,
		nil)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	task := decodeScheduledTask(t, rr)
	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "chain-only-1" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-only-1]", task.WorkflowChainIDs)
	}
	if len(task.Workflows) != 0 {
		t.Errorf("Workflows = %v, want empty", task.Workflows)
	}
}

// TestHandleCreateScheduledTask_InvalidChainID_Returns400 verifies that a
// non-existent chain ID returns 400.
func TestHandleCreateScheduledTask_InvalidChainID_Returns400(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-inv-chain", "", "")

	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-inv-chain",
		`{"name":"X","cron_expression":"* * * * *","workflows":[],"workflow_chain_ids":["no-such-chain"]}`,
		nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid_chain")
}

// TestHandleGetScheduledTask_IncludesWorkflowChainIDs verifies that the GET
// response includes workflow_chain_ids for a task that has them.
func TestHandleGetScheduledTask_IncludesWorkflowChainIDs(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-get-chain", "wf-get-chain", "project")
	insertChainForTest(t, s, "proj-get-chain", "chain-in-get")

	createScheduledTaskWithChains(t, s, "proj-get-chain", "task-get-chain",
		[]string{"wf-get-chain"}, []string{"chain-in-get"})

	rr := doSchedRequest(t, s, s.handleGetScheduledTask, http.MethodGet, "/api/v1/scheduled-tasks/task-get-chain",
		"proj-get-chain", "", map[string]string{"id": "task-get-chain"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	task := decodeScheduledTask(t, rr)
	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "chain-in-get" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-in-get]", task.WorkflowChainIDs)
	}
}

// TestHandleUpdateScheduledTask_SetWorkflowChainIDs verifies that PATCH can
// add workflow_chain_ids to an existing task.
func TestHandleUpdateScheduledTask_SetWorkflowChainIDs(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-upd-chain-h", "wf-upd-chain-h", "project")
	insertChainForTest(t, s, "proj-upd-chain-h", "chain-upd-1")
	createScheduledTask(t, s, "proj-upd-chain-h", "task-upd-chain-h", "wf-upd-chain-h")

	rr := doSchedRequest(t, s, s.handleUpdateScheduledTask, http.MethodPatch, "/api/v1/scheduled-tasks/task-upd-chain-h",
		"proj-upd-chain-h",
		`{"workflow_chain_ids":["chain-upd-1"]}`,
		map[string]string{"id": "task-upd-chain-h"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	task := decodeScheduledTask(t, rr)
	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "chain-upd-1" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-upd-1]", task.WorkflowChainIDs)
	}
}

// TestHandleListScheduleRuns_IncludesChainRuns verifies that the list-runs
// response includes the chain_runs field from the DB.
func TestHandleListScheduleRuns_IncludesChainRuns(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-runs-chain-h", "wf-runs-chain-h", "project")
	createScheduledTask(t, s, "proj-runs-chain-h", "task-runs-chain-h", "wf-runs-chain-h")

	// Insert a schedule_run with chain_runs directly.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	chainJSON := `[{"chain_id":"c-h1","chain_run_id":"cr-h1"},{"chain_id":"c-h2","error":"oops"}]`
	if _, err := s.pool.Exec(
		`INSERT INTO schedule_runs (id, scheduled_task_id, project_id, triggered_at, status, workflows, chain_runs, error)
		 VALUES ('run-chain-h', 'task-runs-chain-h', 'proj-runs-chain-h', ?, 'triggered', '[]', ?, '')`,
		now, chainJSON,
	); err != nil {
		t.Fatalf("insert schedule_run: %v", err)
	}

	rr := doSchedRequest(t, s, s.handleListScheduleRuns, http.MethodGet,
		"/api/v1/scheduled-tasks/task-runs-chain-h/runs",
		"proj-runs-chain-h", "", map[string]string{"id": "task-runs-chain-h"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var runs []model.ScheduleRun
	if err := json.NewDecoder(rr.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs count = %d, want 1", len(runs))
	}
	if len(runs[0].ChainRuns) != 2 {
		t.Fatalf("ChainRuns len = %d, want 2", len(runs[0].ChainRuns))
	}
	if runs[0].ChainRuns[0].ChainID != "c-h1" || runs[0].ChainRuns[0].ChainRunID != "cr-h1" {
		t.Errorf("ChainRuns[0] = %+v, want {c-h1 cr-h1}", runs[0].ChainRuns[0])
	}
	if runs[0].ChainRuns[1].Error != "oops" {
		t.Errorf("ChainRuns[1].Error = %q, want 'oops'", runs[0].ChainRuns[1].Error)
	}
}

// TestHandleScheduledTask_WSEvent_OnCreateWithChainIDs verifies that the WS event
// broadcast on task creation includes the workflow_chain_ids in the task data.
func TestHandleScheduledTask_WSEvent_OnCreateWithChainIDs(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-ws-chain-h", "wf-ws-chain-h", "project")
	insertChainForTest(t, s, "proj-ws-chain-h", "chain-ws-h")

	client, ch := ws.NewTestClient(s.wsHub, "ws-chain-h-client")
	s.wsHub.Subscribe(client, "proj-ws-chain-h", "")

	createScheduledTaskWithChains(t, s, "proj-ws-chain-h", "task-ws-chain-h",
		[]string{"wf-ws-chain-h"}, []string{"chain-ws-h"})

	gotEvent := drainForEvent(ch, ws.EventScheduleCreated, 500*time.Millisecond)
	if !gotEvent {
		t.Error("did not receive schedule.created WS event after creating task with chain IDs")
	}
}

// TestHandleUpdateScheduledTask_ClearWorkflows_KeepChains_Returns200 verifies that
// clearing workflows while keeping chain IDs is valid (at-least-one-of constraint).
func TestHandleUpdateScheduledTask_ClearWorkflows_KeepChains_Returns200(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-clr-wf-h", "wf-clr-h", "project")
	insertChainForTest(t, s, "proj-clr-wf-h", "chain-clr-h")

	// Create with both.
	createScheduledTaskWithChains(t, s, "proj-clr-wf-h", "task-clr-wf-h",
		[]string{"wf-clr-h"}, []string{"chain-clr-h"})

	// Clear workflows, keep chain IDs unchanged (not sent in PATCH → nil → no update).
	rr := doSchedRequest(t, s, s.handleUpdateScheduledTask, http.MethodPatch,
		"/api/v1/scheduled-tasks/task-clr-wf-h",
		"proj-clr-wf-h", `{"workflows":[]}`, map[string]string{"id": "task-clr-wf-h"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	task := decodeScheduledTask(t, rr)
	if len(task.Workflows) != 0 {
		t.Errorf("Workflows = %v, want empty after clear", task.Workflows)
	}
	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "chain-clr-h" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-clr-h] (unchanged)", task.WorkflowChainIDs)
	}
}
