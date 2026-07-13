package api

import (
	"net/http"

	"be/internal/orchestrator"
	"be/internal/service"
)

// handleRunDynamicWorkflow starts the bundled plan-driven `dynamic` workflow
// (service.DynamicWorkflow) as a project-scoped run, mirroring
// handleRunProjectWorkflow. POST /api/v1/projects/{id}/dynamic-workflow
// body: {instructions, mode?}. mode="auto" is refused (400) unless
// service.DynamicAutoEnabled is true for the project. Observe/drive the
// returned instance via GET .../plan[/revisions] and POST .../plan/revise|approve
// (registerPlanRoutes) or GET /projects/{id}/workflow.
func (s *Server) handleRunDynamicWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body struct {
		Instructions string `json:"instructions"`
		Mode         string `json:"mode"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	planAuto := false
	switch body.Mode {
	case "", "approve":
	case "auto":
		if !service.DynamicAutoEnabled(s.pool, projectID) {
			writeError(w, http.StatusBadRequest, "mode=auto is disabled (dynamic_workflow_auto_enabled=false)")
			return
		}
		planAuto = true
	default:
		writeError(w, http.StatusBadRequest, "mode must be \"approve\" or \"auto\"")
		return
	}

	result, err := s.orchestrator.Start(r.Context(), orchestrator.RunRequest{
		ProjectID:       projectID,
		WorkflowName:    service.DynamicWorkflow,
		Instructions:    body.Instructions,
		ScopeType:       "project",
		PlanAutoApprove: planAuto,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
