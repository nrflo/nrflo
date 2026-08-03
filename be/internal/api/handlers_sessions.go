package api

import (
	"net/http"
	"strconv"

	"be/internal/service"
)

// handleListSessions returns GET /api/v1/sessions: the calling project's
// Sessions-tab listing across every agent_sessions kind. Requires X-Project/
// ?project like the rest of the project-scoped surface.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}
	resp, err := service.ListSessions(s.pool, s.clock, projectID, sessionListLimit(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListSessionsGlobal returns GET /api/v1/sessions/global: every
// project's sessions, newest first — no X-Project required, mirroring
// handleGetActiveWorkflows.
func (s *Server) handleListSessionsGlobal(w http.ResponseWriter, r *http.Request) {
	resp, err := service.ListSessionsGlobal(s.pool, s.clock, sessionListLimit(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// sessionListLimit parses ?limit, 0 (repo default) on absent/invalid.
func sessionListLimit(r *http.Request) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			return l
		}
	}
	return 0
}
