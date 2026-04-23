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
	"be/internal/ws"
)

// newStopEndlessLoopServer creates a minimal Server for stop-endless-loop handler tests.
func newStopEndlessLoopServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "stop_endless_loop_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// seedEndlessLoopInstance inserts a project + workflow + workflow_instance row.
// endlessLoop and stop get persisted on the instance. Status controls terminal vs active.
func seedEndlessLoopInstance(t *testing.T, s *Server, projectID, instanceID, scopeType, status string, endlessLoop, stop bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at)
			VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, created_at, updated_at)
			VALUES ('wf-test', ?, '', ?, '[]', ?, ?)`,
		projectID, scopeType, now, now,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO workflow_instances
			(id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count,
			endless_loop, stop_endless_loop_after_iteration, created_at, updated_at)
			VALUES (?, ?, '', 'wf-test', ?, ?, '{}', 0, ?, ?, ?, ?)`,
		instanceID, projectID, scopeType, status, endlessLoop, stop, now, now,
	); err != nil {
		t.Fatalf("seed wfi: %v", err)
	}
}

// stopEndlessLoopReq builds a POST request for the stop-endless-loop endpoint with the given body.
func stopEndlessLoopReq(t *testing.T, projectID, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+projectID+"/workflow/stop-endless-loop",
		strings.NewReader(body))
	req.SetPathValue("id", projectID)
	return req
}

// TestHandleStopEndlessLoop_MissingProjectID verifies 400 when project path param is empty.
func TestHandleStopEndlessLoop_MissingProjectID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects//workflow/stop-endless-loop",
		strings.NewReader(`{"instance_id":"x","stop":true}`))
	// No SetPathValue("id", ...)
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

// TestHandleStopEndlessLoop_InvalidBody verifies 400 on malformed JSON.
func TestHandleStopEndlessLoop_InvalidBody(t *testing.T) {
	s := newStopEndlessLoopServer(t)
	req := stopEndlessLoopReq(t, "proj-1", "{not json")
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

// TestHandleStopEndlessLoop_MissingInstanceID verifies 400 when instance_id is empty.
func TestHandleStopEndlessLoop_MissingInstanceID(t *testing.T) {
	s := newStopEndlessLoopServer(t)
	req := stopEndlessLoopReq(t, "proj-1", `{"stop":true}`)
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "instance_id")
}

// TestHandleStopEndlessLoop_NotFound verifies 404 for unknown instance id.
func TestHandleStopEndlessLoop_NotFound(t *testing.T) {
	s := newStopEndlessLoopServer(t)
	req := stopEndlessLoopReq(t, "proj-nf", `{"instance_id":"nonexistent","stop":true}`)
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

// TestHandleStopEndlessLoop_WrongProject verifies 400 when the instance belongs to a different project.
func TestHandleStopEndlessLoop_WrongProject(t *testing.T) {
	s := newStopEndlessLoopServer(t)
	seedEndlessLoopInstance(t, s, "proj-owner", "inst-wp", "project", "active", true, false)

	req := stopEndlessLoopReq(t, "proj-other", `{"instance_id":"inst-wp","stop":true}`)
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "does not belong")
}

// TestHandleStopEndlessLoop_TerminalStatus verifies 400 when the instance is not active.
func TestHandleStopEndlessLoop_TerminalStatus(t *testing.T) {
	cases := []string{"completed", "project_completed", "failed"}
	for _, status := range cases {
		t.Run(status, func(t *testing.T) {
			s := newStopEndlessLoopServer(t)
			seedEndlessLoopInstance(t, s, "proj-t", "inst-t-"+status, "project", status, true, false)

			req := stopEndlessLoopReq(t, "proj-t",
				`{"instance_id":"inst-t-`+status+`","stop":true}`)
			rr := httptest.NewRecorder()
			s.handleStopEndlessLoop(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status(%q) = %d, want 400; body: %s", status, rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, "not active")
		})
	}
}

// TestHandleStopEndlessLoop_NotInEndlessLoop verifies 400 when endless_loop=false.
func TestHandleStopEndlessLoop_NotInEndlessLoop(t *testing.T) {
	s := newStopEndlessLoopServer(t)
	seedEndlessLoopInstance(t, s, "proj-nel", "inst-nel", "project", "active", false, false)

	req := stopEndlessLoopReq(t, "proj-nel", `{"instance_id":"inst-nel","stop":true}`)
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "endless loop")
}

// TestHandleStopEndlessLoop_ActiveToggle_PersistsAndReturns200 verifies 200 + DB update + response body.
func TestHandleStopEndlessLoop_ActiveToggle_PersistsAndReturns200(t *testing.T) {
	s := newStopEndlessLoopServer(t)
	seedEndlessLoopInstance(t, s, "proj-ok", "inst-ok", "project", "active", true, false)

	// Toggle stop → true
	req := stopEndlessLoopReq(t, "proj-ok", `{"instance_id":"inst-ok","stop":true}`)
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["instance_id"] != "inst-ok" {
		t.Errorf("resp.instance_id = %v, want inst-ok", resp["instance_id"])
	}
	if resp["stop_endless_loop_after_iteration"] != true {
		t.Errorf("resp.stop = %v, want true", resp["stop_endless_loop_after_iteration"])
	}

	// Verify column updated in DB.
	var stopFlag bool
	if err := s.pool.QueryRow(
		`SELECT stop_endless_loop_after_iteration FROM workflow_instances WHERE id = 'inst-ok'`,
	).Scan(&stopFlag); err != nil {
		t.Fatalf("read back stop flag: %v", err)
	}
	if !stopFlag {
		t.Error("stop_endless_loop_after_iteration = false in DB, want true")
	}

	// Toggle back to false — should also succeed.
	req2 := stopEndlessLoopReq(t, "proj-ok", `{"instance_id":"inst-ok","stop":false}`)
	rr2 := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("toggle back status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	if err := s.pool.QueryRow(
		`SELECT stop_endless_loop_after_iteration FROM workflow_instances WHERE id = 'inst-ok'`,
	).Scan(&stopFlag); err != nil {
		t.Fatalf("read back stop flag: %v", err)
	}
	if stopFlag {
		t.Error("stop_endless_loop_after_iteration = true in DB, want false after toggle back")
	}
}

// TestHandleStopEndlessLoop_BroadcastsWSEvent verifies workflow.updated WS event on success.
func TestHandleStopEndlessLoop_BroadcastsWSEvent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "stop_endless_loop_ws_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	s := &Server{pool: pool, clock: clock.Real(), wsHub: hub}
	seedEndlessLoopInstance(t, s, "proj-ws", "inst-ws", "project", "active", true, false)

	client, sendCh := ws.NewTestClient(hub, "ws-stop-endless-loop")
	hub.Register(client)
	hub.Subscribe(client, "proj-ws", "")

	req := stopEndlessLoopReq(t, "proj-ws", `{"instance_id":"inst-ws","stop":true}`)
	rr := httptest.NewRecorder()
	s.handleStopEndlessLoop(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventWorkflowUpdated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventWorkflowUpdated)
		}
		if event.ProjectID != "proj-ws" {
			t.Errorf("event.ProjectID = %q, want %q", event.ProjectID, "proj-ws")
		}
		if event.Data["instance_id"] != "inst-ws" {
			t.Errorf("event.Data.instance_id = %v, want inst-ws", event.Data["instance_id"])
		}
		if event.Data["stop_endless_loop_after_iteration"] != true {
			t.Errorf("event.Data.stop = %v, want true", event.Data["stop_endless_loop_after_iteration"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for workflow.updated WS event")
	}
}
