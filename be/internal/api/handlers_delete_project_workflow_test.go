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
	"be/internal/ws"
)

// newDeleteProjWFServer creates a minimal Server for delete project workflow instance handler tests.
func newDeleteProjWFServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "del_proj_wf_handler_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// seedProjWFInstance seeds a project, a workflow, and a workflow instance with given status/scope.
func seedProjWFInstance(t *testing.T, s *Server, projectID, instanceID, status, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := s.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed project %q: %v", projectID, err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, phases, groups, created_at, updated_at)
		 VALUES ('wf-test', ?, '', ?, '[{"agent":"test","layer":0}]', '[]', ?, ?)`,
		projectID, scopeType, now, now,
	); err != nil {
		t.Fatalf("seed workflow for project %q: %v", projectID, err)
	}

	if _, err := s.pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		 VALUES (?, ?, '', 'wf-test', ?, ?, '{}', 0, ?, ?)`,
		instanceID, projectID, scopeType, status, now, now,
	); err != nil {
		t.Fatalf("seed workflow instance %q: %v", instanceID, err)
	}
}

// deleteProjWFReq builds a DELETE request for DELETE /api/v1/projects/{id}/workflow/{instance_id}.
func deleteProjWFReq(t *testing.T, projectID, instanceID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/projects/"+projectID+"/workflow/"+instanceID, nil)
	req.SetPathValue("id", projectID)
	req.SetPathValue("instance_id", instanceID)
	return req
}

// TestHandleDeleteProjectWorkflowInstance_StatusCases tests 200 for deletable statuses and 409 for active.
func TestHandleDeleteProjectWorkflowInstance_StatusCases(t *testing.T) {
	cases := []struct {
		status     string
		wantStatus int
	}{
		{"completed", http.StatusOK},
		{"project_completed", http.StatusOK},
		{"failed", http.StatusOK},
		{"active", http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			s := newDeleteProjWFServer(t)
			projID := "proj-" + tc.status
			instID := "inst-" + tc.status
			seedProjWFInstance(t, s, projID, instID, tc.status, "project")

			rr := httptest.NewRecorder()
			s.handleDeleteProjectWorkflowInstance(rr, deleteProjWFReq(t, projID, instID))

			if rr.Code != tc.wantStatus {
				t.Errorf("status(%q) = %d, want %d; body: %s", tc.status, rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}

// TestHandleDeleteProjectWorkflowInstance_ActiveReturnsConflictError verifies error body for active instance.
func TestHandleDeleteProjectWorkflowInstance_ActiveReturnsConflictError(t *testing.T) {
	s := newDeleteProjWFServer(t)
	seedProjWFInstance(t, s, "proj-act", "inst-act", "active", "project")

	rr := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr, deleteProjWFReq(t, "proj-act", "inst-act"))

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
	assertErrorContains(t, rr, "active")
}

// TestHandleDeleteProjectWorkflowInstance_NotFound verifies 404 for a nonexistent instance.
func TestHandleDeleteProjectWorkflowInstance_NotFound(t *testing.T) {
	s := newDeleteProjWFServer(t)

	rr := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr, deleteProjWFReq(t, "proj-nf", "no-such-instance"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// TestHandleDeleteProjectWorkflowInstance_WrongProject verifies 404 when instance belongs to a different project.
func TestHandleDeleteProjectWorkflowInstance_WrongProject(t *testing.T) {
	s := newDeleteProjWFServer(t)
	seedProjWFInstance(t, s, "proj-real", "inst-wp", "completed", "project")

	rr := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr, deleteProjWFReq(t, "proj-other", "inst-wp"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// TestHandleDeleteProjectWorkflowInstance_TicketScoped verifies non-200 for a ticket-scoped instance.
func TestHandleDeleteProjectWorkflowInstance_TicketScoped(t *testing.T) {
	s := newDeleteProjWFServer(t)
	seedProjWFInstance(t, s, "proj-ts", "inst-ts", "completed", "ticket")

	rr := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr, deleteProjWFReq(t, "proj-ts", "inst-ts"))

	if rr.Code == http.StatusOK {
		t.Errorf("status = 200, want non-2xx for ticket-scoped instance")
	}
}

// TestHandleDeleteProjectWorkflowInstance_InstanceGoneAfterDelete verifies instance is removed from DB.
func TestHandleDeleteProjectWorkflowInstance_InstanceGoneAfterDelete(t *testing.T) {
	s := newDeleteProjWFServer(t)
	seedProjWFInstance(t, s, "proj-del", "inst-del", "completed", "project")

	rr1 := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr1, deleteProjWFReq(t, "proj-del", "inst-del"))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first delete status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}

	// Second DELETE must return 404 because the instance is gone.
	rr2 := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr2, deleteProjWFReq(t, "proj-del", "inst-del"))
	if rr2.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404 (instance gone)", rr2.Code)
	}
}

// TestHandleDeleteProjectWorkflowInstance_ResponseBody verifies the success response body.
func TestHandleDeleteProjectWorkflowInstance_ResponseBody(t *testing.T) {
	s := newDeleteProjWFServer(t)
	seedProjWFInstance(t, s, "proj-rb", "inst-rb", "failed", "project")

	rr := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr, deleteProjWFReq(t, "proj-rb", "inst-rb"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["message"] == "" {
		t.Errorf("response body missing 'message' field; got: %v", resp)
	}
}

// TestHandleDeleteProjectWorkflowInstance_WSEvent verifies WS event is broadcast on successful delete.
func TestHandleDeleteProjectWorkflowInstance_WSEvent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "del_proj_wf_ws_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	s := &Server{pool: pool, clock: clock.Real(), wsHub: hub}

	// Register and subscribe a test client to receive project-wide events.
	client, sendCh := ws.NewTestClient(hub, "ws-test-client")
	hub.Register(client)
	hub.Subscribe(client, "proj-ws", "")

	seedProjWFInstance(t, s, "proj-ws", "inst-ws", "completed", "project")

	rr := httptest.NewRecorder()
	s.handleDeleteProjectWorkflowInstance(rr, deleteProjWFReq(t, "proj-ws", "inst-ws"))

	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", rr.Code)
	}

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal WS event: %v", err)
		}
		if event.Type != ws.EventWorkflowInstanceDeleted {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventWorkflowInstanceDeleted)
		}
		if event.ProjectID != "proj-ws" {
			t.Errorf("event.ProjectID = %q, want %q", event.ProjectID, "proj-ws")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for workflow_instance.deleted WS event")
	}
}
