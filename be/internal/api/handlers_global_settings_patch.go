package api

import (
	"encoding/json"
	"net/http"

	"be/internal/service"
)

func (s *Server) handlePatchGlobalSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LowConsumptionMode                 *bool           `json:"low_consumption_mode"`
		SimplifiedAgentsGraph              *bool           `json:"simplified_agents_graph"`
		Experimental                       *bool           `json:"experimental"`
		APIModeEnabled                     *bool           `json:"api_mode_enabled"`
		APINativeToolsEnabled              *bool           `json:"api_native_tools_enabled"`
		DynamicWorkflowAutoEnabled         *bool           `json:"dynamic_workflow_auto_enabled"`
		ClaudeSystemPromptOverrideEnabled  *bool           `json:"claude_system_prompt_override_enabled"`
		StallStartTimeoutSec               json.RawMessage `json:"stall_start_timeout_sec"`
		StallRunningTimeoutSec             json.RawMessage `json:"stall_running_timeout_sec"`
		RefineryFoldStartContextPct        json.RawMessage `json:"refinery_fold_start_context_pct"`
		RefineryConsoleFoldStartContextPct json.RawMessage `json:"refinery_console_fold_start_context_pct"`
		CaptureThinkingEnabled             *bool           `json:"capture_thinking_enabled"`
		APIViaCLIEnabled                   *bool           `json:"api_via_cli_enabled"`
		ExperimentalObserverEnabled        *bool           `json:"experimental_observer_enabled"`
		ObserverSystemContext              *string         `json:"observer_system_context"`
		ObserverProvider                   *string         `json:"observer_provider"`
		ObserverModel                      *string         `json:"observer_model"`
		ConsoleYolo                        *bool           `json:"console_yolo"`
		menuPatchFields
		watcherPatchFields
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

	if req.APIModeEnabled != nil {
		val := "false"
		if *req.APIModeEnabled {
			val = "true"
		}
		if err := svc.Set("api_mode_enabled", val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.APINativeToolsEnabled != nil {
		val := "false"
		if *req.APINativeToolsEnabled {
			val = "true"
		}
		if err := svc.Set("api_native_tools_enabled", val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.DynamicWorkflowAutoEnabled != nil {
		val := "false"
		if *req.DynamicWorkflowAutoEnabled {
			val = "true"
		}
		if err := svc.Set(service.DynamicWorkflowAutoEnabledKey, val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.ClaudeSystemPromptOverrideEnabled != nil {
		val := "false"
		if *req.ClaudeSystemPromptOverrideEnabled {
			val = "true"
		}
		if err := svc.Set("claude_system_prompt_override_enabled", val); err != nil {
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
	if err := applyOptionalBoundedIntSetting(svc, req.RefineryFoldStartContextPct, service.RefineryFoldStartContextPctKey, 0, 100, w); err != nil {
		return
	}
	if err := applyOptionalBoundedIntSetting(svc, req.RefineryConsoleFoldStartContextPct, service.RefineryConsoleFoldStartContextPctKey, 0, 100, w); err != nil {
		return
	}

	if req.APIViaCLIEnabled != nil {
		if err := svc.SetAPIViaCLIEnabled(*req.APIViaCLIEnabled); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.CaptureThinkingEnabled != nil {
		if err := svc.SetCaptureThinkingEnabled(*req.CaptureThinkingEnabled); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.ExperimentalObserverEnabled != nil {
		if err := svc.SetExperimentalObserverEnabled(*req.ExperimentalObserverEnabled); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.ObserverSystemContext != nil {
		if err := svc.SetObserverSystemContext(*req.ObserverSystemContext); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.ObserverProvider != nil {
		if err := svc.SetObserverProvider(*req.ObserverProvider); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.ObserverModel != nil {
		if err := svc.SetObserverModel(*req.ObserverModel); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.ConsoleYolo != nil {
		val := "false"
		if *req.ConsoleYolo {
			val = "true"
		}
		if err := svc.Set("console_yolo", val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := applyMenuToggles(req.menuPatchFields, svc, w); err != nil {
		return
	}

	if err := applyWatcherSettings(req.watcherPatchFields, svc, w); err != nil {
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
