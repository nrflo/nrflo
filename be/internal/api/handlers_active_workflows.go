package api

import (
	"net/http"

	"be/internal/repo"
	"be/internal/service"
)

// handleGetActiveWorkflows returns all active workflow instances across all projects.
// No X-Project header required — this is a global endpoint.
func (s *Server) handleGetActiveWorkflows(w http.ResponseWriter, r *http.Request) {
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)

	instances, err := wfiRepo.ListActive()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows := make([]map[string]interface{}, 0, len(instances))
	for _, wi := range instances {
		if service.IsHiddenWorkflowName(wi.WorkflowID) {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"instance_id": wi.ID,
			"project_id":  wi.ProjectID,
			"ticket_id":   wi.TicketID,
			"workflow_id": wi.WorkflowID,
			"scope_type":  wi.ScopeType,
			"status":      string(wi.Status),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workflows": rows,
		"count":     len(rows),
	})
}
