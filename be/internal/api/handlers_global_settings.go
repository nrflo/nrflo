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

	apiModeVal, err := svc.Get("api_mode_enabled")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	apiNativeToolsVal, err := svc.Get("api_native_tools_enabled")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dynamicAutoVal, err := svc.Get(service.DynamicWorkflowAutoEnabledKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	captureThinkingVal, err := svc.Get("capture_thinking_enabled")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	claudeSysPromptOverrideVal, err := svc.Get("claude_system_prompt_override_enabled")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	apiViaCLIEnabled, err := svc.GetAPIViaCLIEnabled()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	observerEnabled, err := svc.GetExperimentalObserverEnabled()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	observerSysCtx, err := svc.GetObserverSystemContext()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	observerProvider, err := svc.GetObserverProvider()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	observerModel, err := svc.GetObserverModel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]interface{}{
		"low_consumption_mode":                  val == "true",
		"simplified_agents_graph":               simplifiedAgentsGraphVal == "true",
		"experimental":                          experimentalVal == "true",
		"api_mode_enabled":                      apiModeVal == "true",
		"api_native_tools_enabled":              apiNativeToolsVal == "true",
		"dynamic_workflow_auto_enabled":         dynamicAutoVal == "true",
		"capture_thinking_enabled":              captureThinkingVal == "true",
		"claude_system_prompt_override_enabled": claudeSysPromptOverrideVal == "true",
		"api_via_cli_enabled":                   apiViaCLIEnabled,
		"experimental_observer_enabled":         observerEnabled,
		"observer_system_context":               observerSysCtx,
		"observer_provider":                     observerProvider,
		"observer_model":                        observerModel,
	}
	for _, ms := range menuSettings {
		v, err := boolWithDefault(svc, ms.key, ms.def)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp[ms.key] = v
	}
	for _, ws := range watcherIntSettings {
		v, err := intWithDefault(svc, ws.key, ws.def)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp[ws.key] = v
	}
	for _, ws := range watcherFloatSettings {
		v, err := floatWithDefault(svc, ws.key, ws.def)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp[ws.key] = v
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
