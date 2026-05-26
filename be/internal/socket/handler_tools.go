package socket

import (
	"encoding/json"
)

func (h *Handler) handleTools(req Request, action string) Response {
	if h.toolDispatcher == nil {
		return MakeErrorResponse(req.ID, NewInternalError("tool dispatcher not available"))
	}

	var params struct {
		SessionID  string `json:"session_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}

	switch action {
	case "list":
		tools, err := h.toolDispatcher.ListTools(params.InstanceID, params.SessionID)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, tools)

	case "call":
		var callParams struct {
			SessionID  string          `json:"session_id"`
			InstanceID string          `json:"instance_id"`
			Name       string          `json:"name"`
			Input      json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if callParams.Name == "" {
			return MakeErrorResponse(req.ID, NewValidationError("name is required"))
		}
		output, isError, err := h.toolDispatcher.CallTool(callParams.InstanceID, callParams.SessionID, callParams.Name, callParams.Input)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]interface{}{
			"output":   output,
			"is_error": isError,
		})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("tools."+action))
	}
}
