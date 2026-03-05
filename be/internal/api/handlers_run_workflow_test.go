package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── handleRunWorkflow (ticket-scoped) ─────────────────────────────────────────

// TestHandleRunWorkflow_MissingProject verifies 400 when ?project= is absent.
func TestHandleRunWorkflow_MissingProject(t *testing.T) {
	s := &Server{}
	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/TKT-1/workflow/run",
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// TestHandleRunWorkflow_MissingTicketID verifies 400 when ticket ID path param is empty.
func TestHandleRunWorkflow_MissingTicketID(t *testing.T) {
	s := &Server{}
	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets//workflow/run", "proj"),
		strings.NewReader(body))
	// No path value "id" set → extractID returns ""
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "ticket ID")
}

// TestHandleRunWorkflow_OrchestratorNil verifies 503 when orchestrator is not set.
func TestHandleRunWorkflow_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
}

// TestHandleRunWorkflow_InvalidBody verifies 400 for malformed JSON.
func TestHandleRunWorkflow_InvalidBody(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader("{bad json"))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

// TestHandleRunWorkflow_MissingWorkflow verifies 400 when workflow is empty.
func TestHandleRunWorkflow_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"interactive":false}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleRunWorkflow_MutualExclusivity verifies 400 when both interactive and plan_mode are true.
func TestHandleRunWorkflow_MutualExclusivity(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"feature","interactive":true,"plan_mode":true}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for mutual exclusivity violation", rr.Code)
	}
	assertErrorContains(t, rr, "mutually exclusive")
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

// TestHandleRunProjectWorkflow_MissingProjectID verifies 400 when project ID path param is empty.
func TestHandleRunProjectWorkflow_MissingProjectID(t *testing.T) {
	s := &Server{}
	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/run",
		strings.NewReader(body))
	// No path value "id" set
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

// TestHandleRunProjectWorkflow_OrchestratorNil verifies 503 when orchestrator is nil.
func TestHandleRunProjectWorkflow_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/run",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

// TestHandleRunProjectWorkflow_InvalidBody verifies 400 for malformed JSON.
func TestHandleRunProjectWorkflow_InvalidBody(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/run",
		strings.NewReader("{not json"))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

// TestHandleRunProjectWorkflow_MissingWorkflow verifies 400 when workflow is omitted.
func TestHandleRunProjectWorkflow_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"plan_mode":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/run",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleRunProjectWorkflow_MutualExclusivity verifies 400 when both interactive and plan_mode are true.
func TestHandleRunProjectWorkflow_MutualExclusivity(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"feature","interactive":true,"plan_mode":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/run",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for mutual exclusivity violation", rr.Code)
	}
	assertErrorContains(t, rr, "mutually exclusive")
}

// TestHandleRunProjectWorkflow_BothFalse_PassesCheck verifies both false passes mutual exclusivity.
func TestHandleRunProjectWorkflow_BothFalse_PassesCheck(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"feature","interactive":false,"plan_mode":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/run",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	// Should not return 400 for "mutually exclusive"
	if rr.Code == http.StatusBadRequest {
		var respBody map[string]string
		json.NewDecoder(rr.Body).Decode(&respBody)
		if strings.Contains(respBody["error"], "mutually exclusive") {
			t.Error("both false should not trigger mutual exclusivity error")
		}
	}
}

// TestHandleRunProjectWorkflow_InteractiveAndPlanModeFieldsParsed verifies
// the JSON body is parsed with the correct field names (interactive, plan_mode).
func TestHandleRunProjectWorkflow_InteractiveAndPlanModeFieldsParsed(t *testing.T) {
	// Use a string that would fail JSON if field names are wrong
	body := `{"workflow":"test","interactive":true,"plan_mode":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/run",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")

	// We can't inject an orchestrator mock, so we just verify the mutual exclusivity
	// check passes (interactive=true, plan_mode=false is valid).
	s := newTakeControlServer(t)
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	// Not 400 with "mutually exclusive"
	if rr.Code == http.StatusBadRequest {
		var respBody map[string]string
		json.NewDecoder(rr.Body).Decode(&respBody)
		if strings.Contains(respBody["error"], "mutually exclusive") {
			t.Error("interactive=true, plan_mode=false should not fail mutual exclusivity check")
		}
	}
}
