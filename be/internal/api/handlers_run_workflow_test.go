package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── handleRunWorkflow (ticket-scoped) ─────────────────────────────────────────

// TestHandleRunWorkflow_Guards exercises the run-workflow (ticket-scoped) guard ladder.
func TestHandleRunWorkflow_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project", "", false, true, `{"workflow":"feature"}`, http.StatusBadRequest, "X-Project"},
		{"missing_ticket_id", "", true, false, `{"workflow":"feature"}`, http.StatusBadRequest, "ticket ID"},
		{"orchestrator_nil", "", true, true, `{"workflow":"feature"}`, http.StatusServiceUnavailable, "orchestrator not available"},
		{"invalid_body", "real", true, true, "{bad json", http.StatusBadRequest, "invalid request body"},
		{"missing_workflow", "real", true, true, `{"interactive":false}`, http.StatusBadRequest, "workflow name is required"},
		{"mutual_exclusivity", "real", true, true, `{"workflow":"feature","interactive":true,"plan_mode":true}`, http.StatusBadRequest, "mutually exclusive"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/tickets/TKT-1/workflow/run", "TKT-1",
				func(s *Server) http.HandlerFunc { return s.handleRunWorkflow })
		})
	}
}

// TestHandleRunWorkflow_InteractiveOnly_Passes verifies that interactive=true without plan_mode
// passes the mutual exclusivity check (proceeds to orchestrator.Start which fails with no project).
func TestHandleRunWorkflow_InteractiveOnly_Passes(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"feature","interactive":true}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj-x"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	// The handler passes mutual exclusivity check; Start() fails with "project not found"
	// which is 500 (not 400 for mutual exclusivity)
	if rr.Code == http.StatusBadRequest {
		var body map[string]string
		json.NewDecoder(rr.Body).Decode(&body)
		if strings.Contains(body["error"], "mutually exclusive") {
			t.Error("interactive=true alone should not trigger mutual exclusivity error")
		}
	}
}

// TestHandleRunWorkflow_PlanModeOnly_Passes verifies that plan_mode=true without interactive
// passes the mutual exclusivity check.
func TestHandleRunWorkflow_PlanModeOnly_Passes(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"feature","plan_mode":true}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj-y"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	// Passes mutual exclusivity; Start() fails because project doesn't exist
	if rr.Code == http.StatusBadRequest {
		var body map[string]string
		json.NewDecoder(rr.Body).Decode(&body)
		if strings.Contains(body["error"], "mutually exclusive") {
			t.Error("plan_mode=true alone should not trigger mutual exclusivity error")
		}
	}
}

// ── handleRunProjectWorkflow (project-scoped) ─────────────────────────────────

// TestHandleRunProjectWorkflow_Guards exercises the run-workflow (project-scoped) guard ladder.
func TestHandleRunProjectWorkflow_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project_id", "", false, false, `{"workflow":"feature"}`, http.StatusBadRequest, "project ID required"},
		{"orchestrator_nil", "", false, true, `{"workflow":"feature"}`, http.StatusServiceUnavailable, ""},
		{"invalid_body", "real", false, true, "{not json", http.StatusBadRequest, "invalid request body"},
		{"missing_workflow", "real", false, true, `{"plan_mode":false}`, http.StatusBadRequest, "workflow name is required"},
		{"mutual_exclusivity", "real", false, true, `{"workflow":"feature","interactive":true,"plan_mode":true}`, http.StatusBadRequest, "mutually exclusive"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/projects/proj-1/workflow/run", "proj-1",
				func(s *Server) http.HandlerFunc { return s.handleRunProjectWorkflow })
		})
	}
}

// TestHandleRunProjectWorkflow_ValidModeFlagsPassCheck verifies that valid interactive/plan_mode
// combinations (both false, and interactive-only) pass the mutual exclusivity check.
func TestHandleRunProjectWorkflow_ValidModeFlagsPassCheck(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"both_false", `{"workflow":"feature","interactive":false,"plan_mode":false}`},
		{"interactive_only", `{"workflow":"test","interactive":true,"plan_mode":false}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := newTakeControlServer(t)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/run",
				strings.NewReader(tc.body))
			req.SetPathValue("id", "proj-1")
			rr := httptest.NewRecorder()
			s.handleRunProjectWorkflow(rr, req)

			// Should not return 400 for "mutually exclusive"
			if rr.Code == http.StatusBadRequest {
				var respBody map[string]string
				json.NewDecoder(rr.Body).Decode(&respBody)
				if strings.Contains(respBody["error"], "mutually exclusive") {
					t.Errorf("%s should not trigger mutual exclusivity error", tc.name)
				}
			}
		})
	}
}
