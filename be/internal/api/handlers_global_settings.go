package api

import (
	"encoding/json"
	"errors"
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
	retentionLimit := 1000
	if retentionVal != "" {
		if parsed, parseErr := strconv.Atoi(retentionVal); parseErr == nil {
			retentionLimit = parsed
		}
	}

	contextSaveViaAgentVal, err := svc.Get("context_save_via_agent")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	simplifiedAgentsGraphVal, err := svc.Get("simplified_agents_graph")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	experimentalVal, err := svc.Get("experimental")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	stallStartVal, err := svc.Get("stall_start_timeout_sec")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stallRunningVal, err := svc.Get("stall_running_timeout_sec")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]interface{}{
		"low_consumption_mode":     val == "true",
		"context_save_via_agent":   contextSaveViaAgentVal == "true",
		"simplified_agents_graph":  simplifiedAgentsGraphVal == "true",
		"experimental":             experimentalVal == "true",
		"session_retention_limit":  retentionLimit,
		"api_mode_enabled":         s.apiMode,
	}
	if stallStartVal != "" {
		if parsed, parseErr := strconv.Atoi(stallStartVal); parseErr == nil {
			resp["stall_start_timeout_sec"] = parsed
		}
	} else {
		resp["stall_start_timeout_sec"] = nil
	}
	if stallRunningVal != "" {
		if parsed, parseErr := strconv.Atoi(stallRunningVal); parseErr == nil {
			resp["stall_running_timeout_sec"] = parsed
		}
	} else {
		resp["stall_running_timeout_sec"] = nil
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePatchGlobalSettings updates global settings.
// PATCH /api/v1/settings
func (s *Server) handlePatchGlobalSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LowConsumptionMode     *bool           `json:"low_consumption_mode"`
		ContextSaveViaAgent    *bool           `json:"context_save_via_agent"`
		SimplifiedAgentsGraph  *bool           `json:"simplified_agents_graph"`
		Experimental           *bool           `json:"experimental"`
		SessionRetentionLimit  *int            `json:"session_retention_limit"`
		StallStartTimeoutSec   json.RawMessage `json:"stall_start_timeout_sec"`
		StallRunningTimeoutSec json.RawMessage `json:"stall_running_timeout_sec"`
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

	if req.ContextSaveViaAgent != nil {
		val := "false"
		if *req.ContextSaveViaAgent {
			val = "true"
		}
		if err := svc.Set("context_save_via_agent", val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.SimplifiedAgentsGraph != nil {
		val := "false"
		if *req.SimplifiedAgentsGraph {
			val = "true"
		}
		if err := svc.Set("simplified_agents_graph", val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.Experimental != nil {
		val := "false"
		if *req.Experimental {
			val = "true"
		}
		if err := svc.Set("experimental", val); err != nil {
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

	if err := applyOptionalIntSetting(svc, req.StallStartTimeoutSec, "stall_start_timeout_sec", w); err != nil {
		return
	}
	if err := applyOptionalIntSetting(svc, req.StallRunningTimeoutSec, "stall_running_timeout_sec", w); err != nil {
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// applyOptionalIntSetting handles a json.RawMessage field that can be absent (nil),
// null ("null" → clear), or an integer (>= 0 → set). Returns a non-nil error sentinel
// when an HTTP error was already written and the caller should return.
func applyOptionalIntSetting(svc *service.GlobalSettingsService, raw json.RawMessage, key string, w http.ResponseWriter) error {
	if raw == nil {
		return nil // absent → no-op
	}
	if string(raw) == "null" {
		if err := svc.Set(key, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return err
		}
		return nil
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		writeError(w, http.StatusBadRequest, key+" must be an integer or null")
		return err
	}
	if v < 0 {
		writeError(w, http.StatusBadRequest, key+" must be >= 0")
		return errors.New("bad request")
	}
	if err := svc.Set(key, strconv.Itoa(v)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return err
	}
	return nil
}
