package api

import (
	"net/http"

	"be/internal/usagelimits"
)

// handleGetUsageLimits returns cached CLI usage limits data.
func (s *Server) handleGetUsageLimits(w http.ResponseWriter, r *http.Request) {
	data := s.usageLimitsCache.Get()
	if data == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"claude":     usagelimits.ToolUsage{Available: false},
			"codex":      usagelimits.ToolUsage{Available: false},
			"fetched_at": nil,
		})
		return
	}
	writeJSON(w, http.StatusOK, data)
}
