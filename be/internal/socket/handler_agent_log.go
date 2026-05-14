package socket

import (
	"bytes"
	"context"
	"encoding/json"

	"be/internal/logger"
	"be/internal/service"
	"be/internal/ws"
)

func (h *Handler) handleAgentLog(ctx context.Context, req Request) Response {
	var params struct {
		SessionID string          `json:"session_id"`
		Type      string          `json:"type"`
		Message   string          `json:"message"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.SessionID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}
	if params.Message == "" {
		return MakeErrorResponse(req.ID, NewValidationError("message is required"))
	}

	category := params.Type
	if category == "" {
		category = "text"
	}

	payloadJSON := ""
	if len(params.Payload) > 0 && string(params.Payload) != "null" {
		var buf bytes.Buffer
		if err := json.Compact(&buf, params.Payload); err == nil {
			payloadJSON = buf.String()
		}
	}

	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(params.SessionID, params.Message, category, payloadJSON)
	if err != nil {
		logger.Error(ctx, "agent.log: failed to record message", "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	if projectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventMessagesUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflowName,
		}, map[string]interface{}{
			"session_id": params.SessionID,
			"category":   category,
		})
	}

	logFields := []interface{}{"session_id", params.SessionID, "type", category, "message", params.Message}
	if payloadJSON != "" {
		logFields = append(logFields, "payload", payloadJSON)
	}
	logger.Info(ctx, "agent.log", logFields...)

	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, params.SessionID); sigErr != nil {
			logger.Info(ctx, "agent.log: BumpLastMessage error (best-effort)", "error", sigErr)
		}
		if sigErr := h.signaler.SetLastMessage(projectID, ticketID, workflowName, params.SessionID, params.Message); sigErr != nil {
			logger.Info(ctx, "agent.log: SetLastMessage error (best-effort)", "error", sigErr)
		}
	}

	return MakeResponse(req.ID, map[string]string{"status": "logged"})
}
