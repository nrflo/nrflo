package socket

import (
	"context"
	"encoding/json"

	"be/internal/service"
	"be/internal/ws"
)

// handleAgentContextUpdate processes agent.context_update: persists the
// session's context_left percentage, broadcasts it, and — when a console
// engine is registered for this session — forwards it to ConsoleHooks so the
// engine can emit a token_usage event (nil-safe; a no-op for autonomous
// sessions).
func (h *Handler) handleAgentContextUpdate(_ context.Context, req Request) Response {
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
		service.BroadcastFromCtx(h.wsHub, ws.EventAgentContextUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflow,
		}, map[string]interface{}{
			"session_id":   params.SessionID,
			"context_left": params.ContextLeft,
		})
	}
	if h.consoleHooks != nil {
		h.consoleHooks.ConsoleContextLeft(params.SessionID, params.ContextLeft)
	}
	return MakeResponse(req.ID, map[string]string{"status": "updated"})
}
