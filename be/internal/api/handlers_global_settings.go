package api

import (
	"net/http"
	"strconv"

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

	retentionVal, err := svc.Get("session_retention_limit")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	retentionLimit := 100
	if retentionVal != "" {
		if parsed, parseErr := strconv.Atoi(retentionVal); parseErr == nil {
			retentionLimit = parsed
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"low_consumption_mode":    val == "true",
		"session_retention_limit": retentionLimit,
	})
}

// handlePatchGlobalSettings updates global settings.
// PATCH /api/v1/settings
func (s *Server) handlePatchGlobalSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LowConsumptionMode     *bool `json:"low_consumption_mode"`
		SessionRetentionLimit  *int  `json:"session_retention_limit"`
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

	if req.SessionRetentionLimit != nil {
		if *req.SessionRetentionLimit < 10 {
			writeError(w, http.StatusBadRequest, "session_retention_limit must be >= 10")
			return
		}
		if err := svc.Set("session_retention_limit", strconv.Itoa(*req.SessionRetentionLimit)); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
