package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── handleContinueWorkflow (ticket-scoped) ────────────────────────────────────

func TestHandleContinueWorkflow_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/TKT-1/workflow/continue",
		strings.NewReader(`{"workflow":"feature"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleContinueWorkflow_MissingTicketID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets//workflow/continue", "proj"),
		strings.NewReader(`{"workflow":"feature"}`))
	// no path value "id" set → extractID returns ""
	rr := httptest.NewRecorder()
	s.handleContinueWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "ticket ID")
}

func TestHandleContinueWorkflow_NilOrchestrator(t *testing.T) {
	s := &Server{orchestrator: nil}
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/continue", "proj"),
		strings.NewReader(`{"workflow":"feature"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflow(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
}

func TestHandleContinueWorkflow_InvalidJSON(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/continue", "proj"),
		strings.NewReader("{bad json"))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

func TestHandleContinueWorkflow_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/continue", "proj"),
		strings.NewReader(`{"instructions":"do it"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

func TestHandleContinueWorkflow_NoWaitingInstance(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-NONE/workflow/continue", "proj-none"),
		strings.NewReader(`{"workflow":"feature"}`))
	req.SetPathValue("id", "TKT-NONE")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflow(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "no waiting workflow instance")
}

// ── handleFailWorkflow (ticket-scoped) ────────────────────────────────────────

func TestHandleFailWorkflow_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/TKT-1/workflow/fail",
		strings.NewReader(`{"workflow":"feature","reason":"bad"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleFailWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleFailWorkflow_NilOrchestrator(t *testing.T) {
	s := &Server{orchestrator: nil}
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/fail", "proj"),
		strings.NewReader(`{"workflow":"feature","reason":"bad"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleFailWorkflow(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
}

func TestHandleFailWorkflow_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/fail", "proj"),
		strings.NewReader(`{"reason":"bad"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleFailWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

func TestHandleFailWorkflow_MissingReason(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/fail", "proj"),
		strings.NewReader(`{"workflow":"feature"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleFailWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "reason is required")
}

func TestHandleFailWorkflow_NoActiveInstance(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-NONE/workflow/fail", "proj-none"),
		strings.NewReader(`{"workflow":"feature","reason":"giving up"}`))
	req.SetPathValue("id", "TKT-NONE")
	rr := httptest.NewRecorder()
	s.handleFailWorkflow(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "no active or waiting workflow instance")
}

// ── handleContinueWorkflowProject (project-scoped) ───────────────────────────

func TestHandleContinueWorkflowProject_MissingProjectID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/continue",
		strings.NewReader(`{"instance_id":"inst-1"}`))
	// no path value "id" set
	rr := httptest.NewRecorder()
	s.handleContinueWorkflowProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

func TestHandleContinueWorkflowProject_NilOrchestrator(t *testing.T) {
	s := &Server{orchestrator: nil}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/continue",
		strings.NewReader(`{"instance_id":"inst-1"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflowProject(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
}

func TestHandleContinueWorkflowProject_InvalidJSON(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/continue",
		strings.NewReader("{bad json"))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflowProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

func TestHandleContinueWorkflowProject_MissingInstanceID(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/continue",
		strings.NewReader(`{"instructions":"go"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflowProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "instance_id is required")
}

func TestHandleContinueWorkflowProject_InstanceNotFound(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/continue",
		strings.NewReader(`{"instance_id":"nonexistent-instance-xyz"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleContinueWorkflowProject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "workflow instance not found")
}

// ── handleFailWorkflowProject (project-scoped) ───────────────────────────────

func TestHandleFailWorkflowProject_MissingProjectID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/fail",
		strings.NewReader(`{"instance_id":"inst-1","reason":"bad"}`))
	// no path value "id" set
	rr := httptest.NewRecorder()
	s.handleFailWorkflowProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

func TestHandleFailWorkflowProject_MissingInstanceID(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/fail",
		strings.NewReader(`{"reason":"bad"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleFailWorkflowProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "instance_id is required")
}

func TestHandleFailWorkflowProject_MissingReason(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/fail",
		strings.NewReader(`{"instance_id":"inst-1"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleFailWorkflowProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "reason is required")
}
