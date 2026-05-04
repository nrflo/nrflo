package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/scheduler"
	"be/internal/ws"
)

// newScheduledTaskServer creates a minimal Server for scheduled task handler tests.
func newScheduledTaskServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sched_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	sched := scheduler.New(pool, nil, hub, clock.Real(), nil, nil)
	t.Cleanup(func() {
		sched.Stop()
		hub.Stop()
		pool.Close()
	})
	return &Server{pool: pool, clock: clock.Real(), wsHub: hub, scheduler: sched}
}

// seedSchedProject inserts a project and optionally a project-scoped workflow.
func seedSchedProject(t *testing.T, s *Server, projectID, workflowID, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seedSchedProject(%q): %v", projectID, err)
	}
	if workflowID != "" {
		if _, err := s.pool.Exec(
			`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
			 VALUES (?, ?, '', ?, '[]', 1, ?, ?)`,
			workflowID, projectID, scopeType, now, now,
		); err != nil {
			t.Fatalf("seedWorkflow(%q): %v", workflowID, err)
		}
	}
}

// decodeScheduledTask decodes a ScheduledTask from the response recorder.
func decodeScheduledTask(t *testing.T, rr *httptest.ResponseRecorder) *model.ScheduledTask {
	t.Helper()
	var task model.ScheduledTask
	if err := json.NewDecoder(rr.Body).Decode(&task); err != nil {
		t.Fatalf("decode ScheduledTask: %v", err)
	}
	return &task
}

// createScheduledTask creates a task via handler and asserts 201.
func createScheduledTask(t *testing.T, s *Server, projectID, taskID, workflowID string) *model.ScheduledTask {
	t.Helper()
	body := `{"id":"` + taskID + `","name":"Task ` + taskID + `","cron_expression":"* * * * *","workflows":["` + workflowID + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduled-tasks", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleCreateScheduledTask(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createScheduledTask(%q): status = %d, body: %s", taskID, rr.Code, rr.Body.String())
	}
	return decodeScheduledTask(t, rr)
}

// doSchedRequest is a helper to send a request to a handler.
func doSchedRequest(t *testing.T, s *Server, handler func(http.ResponseWriter, *http.Request),
	method, path, projectID, body string, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if projectID != "" {
		ctx := context.WithValue(req.Context(), projectKey, projectID)
		req = req.WithContext(ctx)
	}
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

// -- List --

func TestHandleListScheduledTasks_MissingProject(t *testing.T) {
	s := newScheduledTaskServer(t)
	rr := doSchedRequest(t, s, s.handleListScheduledTasks, http.MethodGet, "/api/v1/scheduled-tasks", "", "", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleListScheduledTasks_Empty(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-ls", "", "")
	rr := doSchedRequest(t, s, s.handleListScheduledTasks, http.MethodGet, "/api/v1/scheduled-tasks", "proj-ls", "", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var list []model.ScheduledTask
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

func TestHandleListScheduledTasks_ReturnsTasks(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-ls2", "wf-ls2", "project")
	createScheduledTask(t, s, "proj-ls2", "t1", "wf-ls2")

	rr := doSchedRequest(t, s, s.handleListScheduledTasks, http.MethodGet, "/api/v1/scheduled-tasks", "proj-ls2", "", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var list []model.ScheduledTask
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len = %d, want 1", len(list))
	}
}

// -- Create --

func TestHandleCreateScheduledTask_MissingProject(t *testing.T) {
	s := newScheduledTaskServer(t)
	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"", `{"name":"X","cron_expression":"* * * * *","workflows":["wf"]}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateScheduledTask_Valid(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-cr", "wf-cr", "project")

	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-cr", `{"id":"task-cr","name":"Created","cron_expression":"*/5 * * * *","workflows":["wf-cr"]}`, nil)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	task := decodeScheduledTask(t, rr)
	if task.ID != "task-cr" {
		t.Errorf("ID = %q, want 'task-cr'", task.ID)
	}
	if task.CronExpression != "*/5 * * * *" {
		t.Errorf("CronExpression = %q, want '*/5 * * * *'", task.CronExpression)
	}
}

func TestHandleCreateScheduledTask_BadJSON(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-bj", "", "")
	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-bj", "not-json", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateScheduledTask_InvalidCron(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-ic", "wf-ic", "project")
	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-ic", `{"name":"X","cron_expression":"not-cron","workflows":["wf-ic"]}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid cron")
}

func TestHandleCreateScheduledTask_TicketScopedWorkflow(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-ts", "wf-ticket", "ticket")
	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-ts", `{"name":"X","cron_expression":"* * * * *","workflows":["wf-ticket"]}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "not_project_scope")
}

func TestHandleCreateScheduledTask_DuplicateID_Returns409(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-dup", "wf-dup", "project")
	createScheduledTask(t, s, "proj-dup", "dup-task", "wf-dup")

	rr := doSchedRequest(t, s, s.handleCreateScheduledTask, http.MethodPost, "/api/v1/scheduled-tasks",
		"proj-dup", `{"id":"dup-task","name":"D2","cron_expression":"* * * * *","workflows":["wf-dup"]}`, nil)
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

// -- Get --

func TestHandleGetScheduledTask_MissingProject(t *testing.T) {
	s := newScheduledTaskServer(t)
	rr := doSchedRequest(t, s, s.handleGetScheduledTask, http.MethodGet, "/api/v1/scheduled-tasks/x",
		"", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetScheduledTask_NotFound(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-getnf", "", "")
	rr := doSchedRequest(t, s, s.handleGetScheduledTask, http.MethodGet, "/api/v1/scheduled-tasks/no-task",
		"proj-getnf", "", map[string]string{"id": "no-task"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleGetScheduledTask_Found(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-getf", "wf-getf", "project")
	createScheduledTask(t, s, "proj-getf", "task-getf", "wf-getf")

	rr := doSchedRequest(t, s, s.handleGetScheduledTask, http.MethodGet, "/api/v1/scheduled-tasks/task-getf",
		"proj-getf", "", map[string]string{"id": "task-getf"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	task := decodeScheduledTask(t, rr)
	if task.ID != "task-getf" {
		t.Errorf("ID = %q, want 'task-getf'", task.ID)
	}
}

// -- Update --

func TestHandleUpdateScheduledTask_MissingProject(t *testing.T) {
	s := newScheduledTaskServer(t)
	rr := doSchedRequest(t, s, s.handleUpdateScheduledTask, http.MethodPatch, "/api/v1/scheduled-tasks/x",
		"", `{"name":"Y"}`, map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleUpdateScheduledTask_NotFound(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-upd-nf", "", "")
	rr := doSchedRequest(t, s, s.handleUpdateScheduledTask, http.MethodPatch, "/api/v1/scheduled-tasks/no-task",
		"proj-upd-nf", `{"name":"Y"}`, map[string]string{"id": "no-task"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleUpdateScheduledTask_InvalidCron(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-upd-cron", "wf-upd-cron", "project")
	createScheduledTask(t, s, "proj-upd-cron", "task-upd-cron", "wf-upd-cron")

	rr := doSchedRequest(t, s, s.handleUpdateScheduledTask, http.MethodPatch, "/api/v1/scheduled-tasks/task-upd-cron",
		"proj-upd-cron", `{"cron_expression":"bad"}`, map[string]string{"id": "task-upd-cron"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleUpdateScheduledTask_Valid(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-upd-ok", "wf-upd-ok", "project")
	createScheduledTask(t, s, "proj-upd-ok", "task-upd-ok", "wf-upd-ok")

	rr := doSchedRequest(t, s, s.handleUpdateScheduledTask, http.MethodPatch, "/api/v1/scheduled-tasks/task-upd-ok",
		"proj-upd-ok", `{"name":"Renamed"}`, map[string]string{"id": "task-upd-ok"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	task := decodeScheduledTask(t, rr)
	if task.Name != "Renamed" {
		t.Errorf("Name = %q, want 'Renamed'", task.Name)
	}
}

// -- Delete --

func TestHandleDeleteScheduledTask_MissingProject(t *testing.T) {
	s := newScheduledTaskServer(t)
	rr := doSchedRequest(t, s, s.handleDeleteScheduledTask, http.MethodDelete, "/api/v1/scheduled-tasks/x",
		"", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleDeleteScheduledTask_NotFound(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-del-nf", "", "")
	rr := doSchedRequest(t, s, s.handleDeleteScheduledTask, http.MethodDelete, "/api/v1/scheduled-tasks/no-task",
		"proj-del-nf", "", map[string]string{"id": "no-task"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDeleteScheduledTask_Valid(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-del-ok", "wf-del-ok", "project")
	createScheduledTask(t, s, "proj-del-ok", "task-del-ok", "wf-del-ok")

	rr := doSchedRequest(t, s, s.handleDeleteScheduledTask, http.MethodDelete, "/api/v1/scheduled-tasks/task-del-ok",
		"proj-del-ok", "", map[string]string{"id": "task-del-ok"})
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body: %s", rr.Code, rr.Body.String())
	}
}

// -- ListRuns --

func TestHandleListScheduleRuns_MissingProject(t *testing.T) {
	s := newScheduledTaskServer(t)
	rr := doSchedRequest(t, s, s.handleListScheduleRuns, http.MethodGet, "/api/v1/scheduled-tasks/x/runs",
		"", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleListScheduleRuns_Empty(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-runs-h", "wf-runs-h", "project")
	createScheduledTask(t, s, "proj-runs-h", "task-runs-h", "wf-runs-h")

	rr := doSchedRequest(t, s, s.handleListScheduleRuns, http.MethodGet, "/api/v1/scheduled-tasks/task-runs-h/runs",
		"proj-runs-h", "", map[string]string{"id": "task-runs-h"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var runs []model.ScheduleRun
	if err := json.NewDecoder(rr.Body).Decode(&runs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("len = %d, want 0", len(runs))
	}
}

// -- RunNow --

func TestHandleRunScheduledTaskNow_MissingProject(t *testing.T) {
	s := newScheduledTaskServer(t)
	s.scheduler = nil
	rr := doSchedRequest(t, s, s.handleRunScheduledTaskNow, http.MethodPost, "/api/v1/scheduled-tasks/x/run-now",
		"", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleRunScheduledTaskNow_NilScheduler(t *testing.T) {
	s := newScheduledTaskServer(t)
	s.scheduler = nil
	rr := doSchedRequest(t, s, s.handleRunScheduledTaskNow, http.MethodPost, "/api/v1/scheduled-tasks/x/run-now",
		"proj-rn", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	assertErrorContains(t, rr, "scheduler not available")
}

// Verify WS events are broadcast on create/update/delete.
func TestHandleScheduledTask_WSEventsOnMutations(t *testing.T) {
	s := newScheduledTaskServer(t)
	seedSchedProject(t, s, "proj-ws", "wf-ws", "project")

	client, ch := ws.NewTestClient(s.wsHub, "ws-client")
	s.wsHub.Subscribe(client, "proj-ws", "")

	// Create.
	createScheduledTask(t, s, "proj-ws", "task-ws", "wf-ws")
	gotEvent := drainForEvent(ch, ws.EventScheduleCreated, 500*time.Millisecond)
	if !gotEvent {
		t.Error("did not receive schedule.created WS event")
	}

	// Update.
	doSchedRequest(t, s, s.handleUpdateScheduledTask, http.MethodPatch, "/api/v1/scheduled-tasks/task-ws",
		"proj-ws", `{"name":"Updated"}`, map[string]string{"id": "task-ws"})
	gotEvent = drainForEvent(ch, ws.EventScheduleUpdated, 500*time.Millisecond)
	if !gotEvent {
		t.Error("did not receive schedule.updated WS event")
	}

	// Delete.
	doSchedRequest(t, s, s.handleDeleteScheduledTask, http.MethodDelete, "/api/v1/scheduled-tasks/task-ws",
		"proj-ws", "", map[string]string{"id": "task-ws"})
	gotEvent = drainForEvent(ch, ws.EventScheduleDeleted, 500*time.Millisecond)
	if !gotEvent {
		t.Error("did not receive schedule.deleted WS event")
	}
}

// drainForEvent drains the channel looking for an event of the given type within timeout.
func drainForEvent(ch <-chan []byte, eventType string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-ch:
			var evt map[string]interface{}
			if err := json.Unmarshal(msg, &evt); err == nil {
				if evt["type"] == eventType {
					return true
				}
			}
		case <-deadline:
			return false
		}
	}
}
