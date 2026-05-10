package api

import (
	"math"
	"net/http"
	"strconv"

	"be/internal/service"
)

// handleListLiveAgentSessions returns currently running agent sessions for a project.
func (s *Server) handleListLiveAgentSessions(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required (use X-Project header or ?project= query param)")
		return
	}

	svc := service.NewAgentSessionLogService(s.pool, s.clock)
	sessions, err := svc.ListLive(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []service.LiveAgentSessionResponse{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// handleListAgentSessionLogs returns paginated finished agent session logs for a project.
func (s *Server) handleListAgentSessionLogs(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required (use X-Project header or ?project= query param)")
		return
	}

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 20
	if v := r.URL.Query().Get("per_page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			if p > 100 {
				p = 100
			}
			perPage = p
		}
	}

	svc := service.NewAgentSessionLogService(s.pool, s.clock)
	sessions, total, err := svc.List(projectID, page, perPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []service.AgentSessionLogResponse{}
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions":    sessions,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}
