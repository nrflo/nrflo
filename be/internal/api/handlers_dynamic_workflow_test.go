package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/service"
)

// NOTE on scope: this package's test DB template (apiCopyTemplateDB,
// testmain_test.go) only runs migrations — it never calls
// service.EnsureGlobalDynamicWorkflow (that's only wired into
// cli/serve.go's real startup path). So the "dynamic" workflow definition
// never exists in these tests' DB, and no test here ever seeds a project
// row either. That means every case that reaches s.orchestrator.Start
// fails fast at the project lookup in orchestrator_start.go ("project not
// found: ...") — a synchronous error returned before the `go o.runLoop(...)`
// line, so no goroutine is ever launched and no CLI subprocess is ever
// spawned. There is intentionally no "successful run" test for this handler
// (mirroring handlers_run_workflow_test.go's TestHandleRunProjectWorkflow_*,
// which only ever exercises guard-ladder paths and "passes the check, then
// fails on project lookup" paths, never a real orchestrator.Start success).

// TestHandleRunDynamicWorkflow_Guards exercises the guard ladder for
// POST /api/v1/projects/{id}/dynamic-workflow: missing project ID, nil
// orchestrator, invalid body, and mode validation. None of these paths ever
// reach s.orchestrator.Start.
func TestHandleRunDynamicWorkflow_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project_id", "", false, false, `{"instructions":"do stuff"}`, http.StatusBadRequest, "project ID required"},
		{"orchestrator_nil", "", false, true, `{"instructions":"do stuff"}`, http.StatusServiceUnavailable, "orchestrator not available"},
		{"invalid_body", "real", false, true, "{not json", http.StatusBadRequest, "invalid request body"},
		{"mode_bogus", "real", false, true, `{"instructions":"x","mode":"bogus"}`, http.StatusBadRequest, "mode"},
		{"mode_auto_gate_off", "real", false, true, `{"instructions":"x","mode":"auto"}`, http.StatusBadRequest, "disabled"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/projects/proj-1/dynamic-workflow", "proj-1",
				func(s *Server) http.HandlerFunc { return s.handleRunDynamicWorkflow })
		})
	}
}

// TestHandleRunDynamicWorkflow_ModeAuto_GateEnabledAtProjectLevel verifies
// that a project-level dynamic_workflow_auto_enabled override (not merely a
// global one) unblocks mode=auto: the request must not receive the
// gate-disabled 400. The target project row doesn't exist in the DB, so
// s.orchestrator.Start fails fast on the project lookup ("project not
// found") well before the plan boundary / planner would ever run — safe, no
// subprocess is spawned. We only assert the specific gate-disabled message
// is absent; a 500 "project not found" is the expected (and safe) outcome.
func TestHandleRunDynamicWorkflow_ModeAuto_GateEnabledAtProjectLevel(t *testing.T) {
	s := newTakeControlServer(t)
	const projectID = "proj-gate-on"
	if err := s.pool.SetProjectConfig(projectID, service.DynamicWorkflowAutoEnabledKey, "true"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	body := `{"instructions":"do stuff","mode":"auto"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/dynamic-workflow", strings.NewReader(body))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	s.handleRunDynamicWorkflow(rr, req)

	if rr.Code == http.StatusBadRequest {
		var respBody map[string]string
		json.NewDecoder(rr.Body).Decode(&respBody)
		if strings.Contains(respBody["error"], "disabled") {
			t.Errorf("gate enabled at project level but still got gate-disabled error: %q", respBody["error"])
		}
	}
}

// TestHandleRunDynamicWorkflow_ModeEmptyAndApprove_SameNonGatePath verifies
// mode="" and mode="approve" both map to planAuto=false and take the exact
// same code path — neither ever consults service.DynamicAutoEnabled (only
// mode=="auto" does). Observed via: both hit s.orchestrator.Start against a
// project that doesn't exist and fail identically ("project not found"),
// confirming neither short-circuited into the auto-gate 400 branch.
func TestHandleRunDynamicWorkflow_ModeEmptyAndApprove_SameNonGatePath(t *testing.T) {
	s := newTakeControlServer(t)
	const projectID = "proj-nomode"

	modes := []string{"", "approve"}
	errs := make([]string, len(modes))
	for i, mode := range modes {
		body := fmt.Sprintf(`{"instructions":"x","mode":%q}`, mode)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/dynamic-workflow", strings.NewReader(body))
		req.SetPathValue("id", projectID)
		rr := httptest.NewRecorder()
		s.handleRunDynamicWorkflow(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("mode=%q: status = %d, want 500 (project not found — neither mode reaches the auto-gate)", mode, rr.Code)
		}
		var respBody map[string]string
		json.NewDecoder(rr.Body).Decode(&respBody)
		errs[i] = respBody["error"]
	}
	if errs[0] != errs[1] {
		t.Errorf("mode=\"\" and mode=\"approve\" produced different errors: %q vs %q", errs[0], errs[1])
	}
}
