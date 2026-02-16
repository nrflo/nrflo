package socket

import (
	"encoding/json"
	"strings"

	"be/internal/types"
	"be/internal/ws"
)

func (h *Handler) handleProjectFindings(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "add":
		var params types.ProjectFindingsAddRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.projectFindingsSvc.Add(projectID, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventProjectFindingsUpdated, projectID, "", "", map[string]interface{}{
			"key":    params.Key,
			"action": "add",
		})
		return MakeResponse(req.ID, map[string]string{"status": "added"})

	case "add-bulk":
		var params types.ProjectFindingsAddBulkRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.projectFindingsSvc.AddBulk(projectID, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventProjectFindingsUpdated, projectID, "", "", map[string]interface{}{
			"action": "add-bulk",
			"count":  len(params.KeyValues),
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status": "added",
			"count":  len(params.KeyValues),
		})

	case "get":
		var params types.ProjectFindingsGetRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		findings, err := h.projectFindingsSvc.Get(projectID, &params)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, findings)

	case "append":
		var params types.ProjectFindingsAppendRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.projectFindingsSvc.Append(projectID, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventProjectFindingsUpdated, projectID, "", "", map[string]interface{}{
			"key":    params.Key,
			"action": "append",
		})
		return MakeResponse(req.ID, map[string]string{"status": "appended"})

	case "append-bulk":
		var params types.ProjectFindingsAppendBulkRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.projectFindingsSvc.AppendBulk(projectID, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventProjectFindingsUpdated, projectID, "", "", map[string]interface{}{
			"action": "append-bulk",
			"count":  len(params.KeyValues),
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status": "appended",
			"count":  len(params.KeyValues),
		})

	case "delete":
		var params types.ProjectFindingsDeleteRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		deleted, err := h.projectFindingsSvc.Delete(projectID, &params)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventProjectFindingsUpdated, projectID, "", "", map[string]interface{}{
			"action":  "delete",
			"deleted": deleted,
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status":  "deleted",
			"deleted": deleted,
		})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("project_findings."+action))
	}
}
