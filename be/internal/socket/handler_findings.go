package socket

import (
	"encoding/json"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

func (h *Handler) handleFindings(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "add":
		var params types.FindingsAddRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, err := h.findingsSvc.Add(&params)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		service.BroadcastFromCtx(h.wsHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
			"agent_type": bctx.AgentType,
			"key":        params.Key,
			"action":     "add",
		})
		return MakeResponse(req.ID, map[string]string{"status": "added"})

	case "add-bulk":
		var params types.FindingsAddBulkRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, err := h.findingsSvc.AddBulk(&params)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		service.BroadcastFromCtx(h.wsHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
			"agent_type": bctx.AgentType,
			"action":     "add-bulk",
			"count":      len(params.KeyValues),
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status": "added",
			"count":  len(params.KeyValues),
		})

	case "get":
		var params types.FindingsGetRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if params.AgentType != "" && params.Layer != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError("agent_type and layer are mutually exclusive"))
		}
		findings, err := h.findingsSvc.Get(&params)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, findings)

	case "append":
		var params types.FindingsAppendRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, err := h.findingsSvc.Append(&params)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		service.BroadcastFromCtx(h.wsHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
			"agent_type": bctx.AgentType,
			"key":        params.Key,
			"action":     "append",
		})
		return MakeResponse(req.ID, map[string]string{"status": "appended"})

	case "append-bulk":
		var params types.FindingsAppendBulkRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, err := h.findingsSvc.AppendBulk(&params)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		service.BroadcastFromCtx(h.wsHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
			"agent_type": bctx.AgentType,
			"action":     "append-bulk",
			"count":      len(params.KeyValues),
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status": "appended",
			"count":  len(params.KeyValues),
		})

	case "delete":
		var params types.FindingsDeleteRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, deleted, err := h.findingsSvc.Delete(&params)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		service.BroadcastFromCtx(h.wsHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
			"agent_type": bctx.AgentType,
			"action":     "delete",
			"deleted":    deleted,
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status":  "deleted",
			"deleted": deleted,
		})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("findings."+action))
	}
}
