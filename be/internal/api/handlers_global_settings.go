package api

import (
	"net/http"

	"be/internal/service"
)

// handleGetGlobalSettings returns global settings.
// GET /api/v1/settings
func (s *Server) handleGetGlobalSettings(w http.ResponseWriter, r *http.Request) {
	svc := service.NewGlobalSettingsService(s.pool, s.clock)

	val, err := svc.Get("low_consumption_mode")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"low_consumption_mode": val == "true",
	})
}

// handlePatchGlobalSettings updates global settings.
// PATCH /api/v1/settings
func (s *Server) handlePatchGlobalSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LowConsumptionMode *bool `json:"low_consumption_mode"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewGlobalSettingsService(s.pool, s.clock)

	if req.LowConsumptionMode != nil {
		val := "false"
		if *req.LowConsumptionMode {
			val = "true"
		}
		if err := svc.Set("low_consumption_mode", val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
