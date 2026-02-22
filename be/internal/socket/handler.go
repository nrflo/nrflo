package socket

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/logger"
	"be/internal/types"
	"be/internal/ws"
)

// Handle dispatches a request to the appropriate service method
func (h *Handler) Handle(req Request) Response {
	// Validate request
	if req.Method == "" {
		return MakeErrorResponse(req.ID, NewInvalidRequestError("method is required"))
	}

	trx := logger.NewTrx()
	ctx := logger.WithTrx(context.Background(), trx)

	// Route based on method prefix
	parts := strings.SplitN(req.Method, ".", 2)
	if len(parts) != 2 {
		logger.Warn(ctx, "unknown socket method", "method", req.Method)
		return MakeErrorResponse(req.ID, NewMethodNotFoundError(req.Method))
	}

	resource := parts[0]
	action := parts[1]

	switch resource {
	case "findings":
		return h.handleFindings(req, action)
	case "project_findings":
		return h.handleProjectFindings(req, action)
	case "agent":
		return h.handleAgent(ctx, req, action)
	case "workflow":
		return h.handleWorkflow(ctx, req, action)
	case "ws":
		return h.handleWS(ctx, req, action)
	default:
		logger.Warn(ctx, "unknown socket method", "method", req.Method)
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
		h.broadcast(ws.EventFindingsUpdated, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
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
		h.broadcast(ws.EventFindingsUpdated, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
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
		h.broadcast(ws.EventFindingsUpdated, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
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
		h.broadcast(ws.EventFindingsUpdated, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
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
		h.broadcast(ws.EventFindingsUpdated, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
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

func (h *Handler) handleAgent(ctx context.Context, req Request, action string) Response {
	// context_update doesn't require project — session_id is globally unique
	if action == "context_update" {
		var params struct {
			SessionID   string `json:"session_id"`
			ContextLeft int    `json:"context_left"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if params.SessionID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
		}
		projectID, ticketID, workflow, err := h.agentSvc.UpdateContextLeft(params.SessionID, params.ContextLeft)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		if projectID != "" {
			h.broadcast(ws.EventAgentContextUpdated, projectID, ticketID, workflow, map[string]interface{}{
				"session_id":   params.SessionID,
				"context_left": params.ContextLeft,
			})
		}
		return MakeResponse(req.ID, map[string]string{"status": "updated"})
	}

	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "fail":
		var params types.AgentRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, err := h.agentSvc.Fail(&params)
		if err != nil {
			logger.Error(ctx, "socket handler error", "method", req.Method, "error", err)
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		logger.Warn(ctx, "agent fail received", "agent_type", bctx.AgentType, "ticket", bctx.TicketID, "workflow", bctx.Workflow)
		h.broadcast(ws.EventAgentCompleted, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
			"action":     "fail",
			"agent_type": bctx.AgentType,
			"session_id": bctx.SessionID,
			"model_id":   bctx.ModelID,
			"result":     "fail",
		})
		return MakeResponse(req.ID, map[string]string{"status": "failed"})

	case "continue":
		var params types.AgentRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, err := h.agentSvc.Continue(&params)
		if err != nil {
			logger.Error(ctx, "socket handler error", "method", req.Method, "error", err)
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		logger.Info(ctx, "agent continue received", "agent_type", bctx.AgentType, "ticket", bctx.TicketID, "workflow", bctx.Workflow)
		h.broadcast(ws.EventAgentContinued, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
			"action":     "continue",
			"agent_type": bctx.AgentType,
			"session_id": bctx.SessionID,
			"model_id":   bctx.ModelID,
		})
		return MakeResponse(req.ID, map[string]string{"status": "continued"})

	case "callback":
		var params types.AgentCallbackRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		bctx, err := h.agentSvc.Callback(&params)
		if err != nil {
			logger.Error(ctx, "socket handler error", "method", req.Method, "error", err)
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		logger.Info(ctx, "agent callback received", "agent_type", bctx.AgentType, "ticket", bctx.TicketID, "level", params.Level)
		h.broadcast(ws.EventAgentCompleted, bctx.ProjectID, bctx.TicketID, bctx.Workflow, map[string]interface{}{
			"action":     "callback",
			"agent_type": bctx.AgentType,
			"level":      params.Level,
			"model_id":   bctx.ModelID,
			"result":     "callback",
		})
		return MakeResponse(req.ID, map[string]string{"status": "callback"})

	default:
		logger.Warn(ctx, "unknown socket method", "method", "agent."+action)
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("agent."+action))
	}
}

func (h *Handler) handleWorkflow(ctx context.Context, req Request, action string) Response {
	switch action {
	case "skip":
		var params struct {
			InstanceID string `json:"instance_id"`
			Tag        string `json:"tag"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if params.InstanceID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("instance_id is required"))
		}
		if params.Tag == "" {
			return MakeErrorResponse(req.ID, NewValidationError("tag is required"))
		}
		logger.Info(ctx, "workflow skip received", "instance_id", params.InstanceID, "tag", params.Tag)
		projectID, ticketID, workflow, err := h.workflowSvc.AddSkipTag(params.InstanceID, params.Tag)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			if strings.Contains(err.Error(), "not in workflow groups") {
				return MakeErrorResponse(req.ID, NewValidationError(err.Error()))
			}
			logger.Error(ctx, "socket handler error", "method", req.Method, "error", err)
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		h.broadcast(ws.EventSkipTagAdded, projectID, ticketID, workflow, map[string]interface{}{
			"instance_id": params.InstanceID,
			"tag":         params.Tag,
		})
		return MakeResponse(req.ID, map[string]string{"status": "added", "tag": params.Tag})

	default:
		logger.Warn(ctx, "unknown socket method", "method", "workflow."+action)
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("workflow."+action))
	}
}

// handleWS handles WebSocket broadcast requests from spawner
func (h *Handler) handleWS(ctx context.Context, req Request, action string) Response {
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
		// ticket_id is optional for project-scoped workflows
		logger.Info(ctx, "ws broadcast", "type", params.Type, "project", params.ProjectID)
		h.broadcast(params.Type, params.ProjectID, params.TicketID, params.Workflow, params.Data)
		return MakeResponse(req.ID, map[string]string{"status": "broadcast"})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("ws."+action))
	}
}
