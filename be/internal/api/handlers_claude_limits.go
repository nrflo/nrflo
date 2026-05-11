package api

import (
	"net/http"

	"be/internal/service"
)

// handleGetClaudeLimits returns the latest Claude API rate limit state.
// GET /api/v1/claude-limits
func (s *Server) handleGetClaudeLimits(w http.ResponseWriter, r *http.Request) {
	svc := service.NewClaudeLimitsService(s.pool, s.clock)
	limits, err := svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]interface{}{}

	if limits.FiveHourUsedPct >= 0 {
		resp["five_hour_used_pct"] = limits.FiveHourUsedPct
	} else {
		resp["five_hour_used_pct"] = nil
	}
	if limits.FiveHourResetsAt != "" {
		resp["five_hour_resets_at"] = limits.FiveHourResetsAt
	} else {
		resp["five_hour_resets_at"] = nil
	}
	if limits.SevenDayUsedPct >= 0 {
		resp["seven_day_used_pct"] = limits.SevenDayUsedPct
	} else {
		resp["seven_day_used_pct"] = nil
	}
	if limits.SevenDayResetsAt != "" {
		resp["seven_day_resets_at"] = limits.SevenDayResetsAt
	} else {
		resp["seven_day_resets_at"] = nil
	}
	if limits.UpdatedAt != "" {
		resp["updated_at"] = limits.UpdatedAt
	} else {
		resp["updated_at"] = nil
	}

	writeJSON(w, http.StatusOK, resp)
}
