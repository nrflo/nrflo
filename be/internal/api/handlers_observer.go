package api

import (
	"errors"
	"net/http"

	"be/internal/repo"
	"be/internal/service"
)

// handleLaunchObserver starts a new observer session.
// POST /api/v1/observers
func (s *Server) handleLaunchObserver(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope      string `json:"scope"`
		ProjectID  string `json:"project_id,omitempty"`
		WorkflowID string `json:"workflow_id,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch req.Scope {
	case "workflow":
		if req.ProjectID == "" || req.WorkflowID == "" {
			writeError(w, http.StatusBadRequest, "project_id and workflow_id required for workflow scope")
			return
		}
	case "project":
		if req.ProjectID == "" {
			writeError(w, http.StatusBadRequest, "project_id required for project scope")
			return
		}
	case "global":
		// no IDs required
	default:
		writeError(w, http.StatusBadRequest, "scope must be one of: workflow, project, global")
		return
	}

	sessionID, err := s.observerSvc.Launch(req.Scope, req.ProjectID, req.WorkflowID)
	if err != nil {
		if errors.Is(err, service.ErrObserverDisabled) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"session_id": sessionID})
}

// handleListObservers returns active observer sessions.
// GET /api/v1/observers
func (s *Server) handleListObservers(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)

	sessionRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	sessions, err := sessionRepo.ListActiveObservers(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}
