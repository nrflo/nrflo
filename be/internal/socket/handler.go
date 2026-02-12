package socket

import (
	"encoding/json"
	"log"
	"strings"

	"be/internal/types"
	"be/internal/ws"
)

// Handle dispatches a request to the appropriate service method
func (h *Handler) Handle(req Request) Response {
	// Validate request
	if req.Method == "" {
		return MakeErrorResponse(req.ID, NewInvalidRequestError("method is required"))
	}

	// Route based on method prefix
	parts := strings.SplitN(req.Method, ".", 2)
	if len(parts) != 2 {
		return MakeErrorResponse(req.ID, NewMethodNotFoundError(req.Method))
	}

	resource := parts[0]
	action := parts[1]

	switch resource {
	case "findings":
		return h.handleFindings(req, action)
	case "agent":
		return h.handleAgent(req, action)
	case "ws":
		return h.handleWS(req, action)
	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError(req.Method))
	}
}

func (h *Handler) handleFindings(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "add":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsAddRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.findingsSvc.Add(projectID, params.TicketID, &params.FindingsAddRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventFindingsUpdated, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"agent_type": params.AgentType,
			"key":        params.Key,
			"action":     "add",
		})
		return MakeResponse(req.ID, map[string]string{"status": "added"})

	case "add-bulk":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsAddBulkRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.findingsSvc.AddBulk(projectID, params.TicketID, &params.FindingsAddBulkRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventFindingsUpdated, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"agent_type": params.AgentType,
			"action":     "add-bulk",
			"count":      len(params.KeyValues),
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status": "added",
			"count":  len(params.KeyValues),
		})

	case "get":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsGetRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		findings, err := h.findingsSvc.Get(projectID, params.TicketID, &params.FindingsGetRequest)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, findings)

	case "append":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsAppendRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.findingsSvc.Append(projectID, params.TicketID, &params.FindingsAppendRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventFindingsUpdated, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"agent_type": params.AgentType,
			"key":        params.Key,
			"action":     "append",
		})
		return MakeResponse(req.ID, map[string]string{"status": "appended"})

	case "append-bulk":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsAppendBulkRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.findingsSvc.AppendBulk(projectID, params.TicketID, &params.FindingsAppendBulkRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventFindingsUpdated, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"agent_type": params.AgentType,
			"action":     "append-bulk",
			"count":      len(params.KeyValues),
		})
		return MakeResponse(req.ID, map[string]interface{}{
			"status": "appended",
			"count":  len(params.KeyValues),
		})

	case "delete":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsDeleteRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		deleted, err := h.findingsSvc.Delete(projectID, params.TicketID, &params.FindingsDeleteRequest)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventFindingsUpdated, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"agent_type": params.AgentType,
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

func (h *Handler) handleAgent(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "complete":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.AgentCompleteRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.agentSvc.Complete(projectID, params.TicketID, &params.AgentCompleteRequest); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventAgentCompleted, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"action":     "complete",
			"agent_type": params.AgentType,
		})
		return MakeResponse(req.ID, map[string]string{"status": "completed"})

	case "fail":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.AgentCompleteRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.agentSvc.Fail(projectID, params.TicketID, &params.AgentCompleteRequest); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventAgentCompleted, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"action":     "fail",
			"agent_type": params.AgentType,
		})
		return MakeResponse(req.ID, map[string]string{"status": "failed"})

	case "continue":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.AgentCompleteRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.agentSvc.Continue(projectID, params.TicketID, &params.AgentCompleteRequest); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventAgentContinued, projectID, params.TicketID, params.Workflow, map[string]interface{}{
			"action":     "continue",
			"agent_type": params.AgentType,
		})
		return MakeResponse(req.ID, map[string]string{"status": "continued"})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("agent."+action))
	}
}

// handleWS handles WebSocket broadcast requests from spawner
func (h *Handler) handleWS(req Request, action string) Response {
	switch action {
	case "broadcast":
		var params struct {
			Type      string                 `json:"type"`
			ProjectID string                 `json:"project_id"`
			TicketID  string                 `json:"ticket_id"`
			Workflow  string                 `json:"workflow,omitempty"`
			Data      map[string]interface{} `json:"data,omitempty"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if params.Type == "" {
			return MakeErrorResponse(req.ID, NewValidationError("type is required"))
		}
		if params.ProjectID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("project_id is required"))
		}
		if params.TicketID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("ticket_id is required"))
		}

		log.Printf("[socket] ws.broadcast received: type=%s project=%s ticket=%s (hub=%v)",
			params.Type, params.ProjectID, params.TicketID, h.wsHub != nil)
		h.broadcast(params.Type, params.ProjectID, params.TicketID, params.Workflow, params.Data)
		return MakeResponse(req.ID, map[string]string{"status": "broadcast"})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("ws."+action))
	}
}
