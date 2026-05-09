package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/ws"
)

// handleListProjectEnvVars returns all env vars for a project.
func (s *Server) handleListProjectEnvVars(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	svc := service.NewProjectEnvVarService(s.pool, s.clock)
	vars, err := svc.List(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, vars)
}

// handlePutProjectEnvVar creates or updates a named env var for a project.
func (s *Server) handlePutProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	name := r.PathValue("name")
	if projectID == "" || name == "" {
		writeError(w, http.StatusBadRequest, "project id and name are required")
		return
	}

	var req struct {
		Value string `json:"value"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewProjectEnvVarService(s.pool, s.clock)
	v, err := svc.Upsert(projectID, name, req.Value)
	if err != nil {
		if strings.Contains(err.Error(), "invalid env var name") ||
			strings.Contains(err.Error(), "is reserved") ||
			strings.Contains(err.Error(), "exceeds maximum length") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		s.wsHub.BroadcastGlobal(ws.NewEvent(ws.EventProjectEnvVarsUpdated, projectID, "", "", map[string]interface{}{
			"project_id": projectID,
		}))
	}

	writeJSON(w, http.StatusOK, v)
}

// handleDeleteProjectEnvVar removes a named env var from a project.
func (s *Server) handleDeleteProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	name := r.PathValue("name")
	if projectID == "" || name == "" {
		writeError(w, http.StatusBadRequest, "project id and name are required")
		return
	}

	svc := service.NewProjectEnvVarService(s.pool, s.clock)
	if err := svc.Delete(projectID, name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		s.wsHub.BroadcastGlobal(ws.NewEvent(ws.EventProjectEnvVarsUpdated, projectID, "", "", map[string]interface{}{
			"project_id": projectID,
		}))
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
