package api

import (
	"net/http"

	"be/internal/service"
)

// handleGetDailyStats computes and returns daily stats for the current project.
// GET /api/v1/daily-stats?range=today|week|month|all
func (s *Server) handleGetDailyStats(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "today"
	}
	if !service.ValidRange(rangeParam) {
		writeError(w, http.StatusBadRequest, "invalid range: "+rangeParam)
		return
	}

	svc := service.NewDailyStatsService(s.pool, s.clock)
	stats, err := svc.GetRange(projectID, rangeParam)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
