package api

import (
	"net/http"

	"be/internal/service"
	"be/internal/types"
)

// handleGetProjectFindings returns all project findings as a JSON map.
// GET /api/v1/projects/{id}/findings
func (s *Server) handleGetProjectFindings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID is required")
		return
	}

	svc := service.NewProjectFindingsService(s.pool, s.clock)
	result, err := svc.Get(projectID, &types.ProjectFindingsGetRequest{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
