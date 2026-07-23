package api

import (
	"net/http"

	"be/internal/repo"
	"be/internal/service"
)

// handleGetStepCursors returns the per-node stepwise cursor progress for one
// workflow instance. GET /api/v1/workflow-instances/{iid}/steps
func (s *Server) handleGetStepCursors(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	if iid == "" {
		writeError(w, http.StatusBadRequest, "workflow instance ID required")
		return
	}

	svc := service.NewWorkflowService(s.pool, s.clock)
	wi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(iid)
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow instance not found")
		return
	}

	if _, err := repo.NewProjectRepo(s.pool, s.clock).Get(wi.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	cursors := svc.BuildStepCursors(iid)
	if cursors == nil {
		cursors = map[string]*service.StepCursorProgress{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workflow_instance_id": iid,
		"cursors":              cursors,
	})
}
