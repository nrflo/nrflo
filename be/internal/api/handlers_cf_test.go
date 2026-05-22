package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── handleContinueWorkflow (ticket-scoped) ────────────────────────────────────

// TestHandleContinueWorkflow_Guards exercises the continue (ticket-scoped) guard ladder.
func TestHandleContinueWorkflow_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project", "", false, true, `{"workflow":"feature"}`, http.StatusBadRequest, "X-Project"},
		{"missing_ticket_id", "", true, false, `{"workflow":"feature"}`, http.StatusBadRequest, "ticket ID"},
		{"nil_orchestrator", "", true, true, `{"workflow":"feature"}`, http.StatusServiceUnavailable, "orchestrator not available"},
		{"invalid_json", "real", true, true, "{bad json", http.StatusBadRequest, "invalid request body"},
		{"missing_workflow", "real", true, true, `{"instructions":"do it"}`, http.StatusBadRequest, "workflow name is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/tickets/TKT-1/workflow/continue", "TKT-1",
				func(s *Server) http.HandlerFunc { return s.handleContinueWorkflow })
		})
	}
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

// TestHandleFailWorkflow_Guards exercises the fail (ticket-scoped) guard ladder.
func TestHandleFailWorkflow_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project", "", false, true, `{"workflow":"feature","reason":"bad"}`, http.StatusBadRequest, "X-Project"},
		{"nil_orchestrator", "", true, true, `{"workflow":"feature","reason":"bad"}`, http.StatusServiceUnavailable, "orchestrator not available"},
		{"missing_workflow", "real", true, true, `{"reason":"bad"}`, http.StatusBadRequest, "workflow name is required"},
		{"missing_reason", "real", true, true, `{"workflow":"feature"}`, http.StatusBadRequest, "reason is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/tickets/TKT-1/workflow/fail", "TKT-1",
				func(s *Server) http.HandlerFunc { return s.handleFailWorkflow })
		})
	}
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

// TestHandleContinueWorkflowProject_Guards exercises the continue (project-scoped) guard ladder.
func TestHandleContinueWorkflowProject_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project_id", "", false, false, `{"instance_id":"inst-1"}`, http.StatusBadRequest, "project ID required"},
		{"nil_orchestrator", "", false, true, `{"instance_id":"inst-1"}`, http.StatusServiceUnavailable, "orchestrator not available"},
		{"invalid_json", "real", false, true, "{bad json", http.StatusBadRequest, "invalid request body"},
		{"missing_instance_id", "real", false, true, `{"instructions":"go"}`, http.StatusBadRequest, "instance_id is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/projects/proj-1/workflow/continue", "proj-1",
				func(s *Server) http.HandlerFunc { return s.handleContinueWorkflowProject })
		})
	}
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

// TestHandleFailWorkflowProject_Guards exercises the fail (project-scoped) guard ladder.
func TestHandleFailWorkflowProject_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project_id", "", false, false, `{"instance_id":"inst-1","reason":"bad"}`, http.StatusBadRequest, "project ID required"},
		{"missing_instance_id", "real", false, true, `{"reason":"bad"}`, http.StatusBadRequest, "instance_id is required"},
		{"missing_reason", "real", false, true, `{"instance_id":"inst-1"}`, http.StatusBadRequest, "reason is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/projects/proj-1/workflow/fail", "proj-1",
				func(s *Server) http.HandlerFunc { return s.handleFailWorkflowProject })
		})
	}
}
