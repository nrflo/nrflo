package socket

import (
	"context"
	"encoding/json"

	"be/internal/logger"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// handleAgentRecordEvent processes agent.record_event socket requests from
// Claude --settings PreToolUse/PostToolUse hooks. Stop/SessionEnd/UserPromptSubmit
// are intentionally ignored — completion is signaled by agent continue/fail.
func (h *Handler) handleAgentRecordEvent(ctx context.Context, req Request) Response {
	var params struct {
		Event      json.RawMessage `json:"event"`
		SessionID  string          `json:"session_id"`
		InstanceID string          `json:"instance_id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.SessionID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}

	var event map[string]interface{}
	if err := json.Unmarshal(params.Event, &event); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError("invalid event JSON: "+err.Error()))
	}

	hookEventName, _ := event["hook_event_name"].(string)

	switch hookEventName {
	case "PreToolUse":
		return h.recordPreToolUse(ctx, req, params.SessionID, event)
	case "PostToolUse":
		return h.recordPostToolUse(ctx, req, params.SessionID, event)
	case "Stop", "SessionEnd", "UserPromptSubmit":
		// Stop hook intentionally unhandled: completion is signaled by agent continue/fail.
		logger.Info(ctx, "record_event: hook ignored", "hook_event_name", hookEventName, "session_id", params.SessionID)
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	default:
		logger.Info(ctx, "record_event: unknown hook event", "hook_event_name", hookEventName, "session_id", params.SessionID)
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	}
}

func (h *Handler) recordPreToolUse(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	toolName, _ := event["tool_name"].(string)
	toolInput, _ := event["tool_input"].(map[string]interface{})

	content := spawner.FormatToolDetail(toolName, toolInput)
	category := spawner.ToolCategory(toolName)

	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(sessionID, content, category)
	if err != nil {
		logger.Error(ctx, "record_event: failed to record pre-tool message", "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	if projectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventMessagesUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflowName,
		}, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	// Bump stall detection (best-effort)
	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, sessionID); sigErr != nil {
			logger.Info(ctx, "record_event: BumpLastMessage error (best-effort)", "error", sigErr)
		}
	}

	// Update context % when token usage is present
	h.maybeUpdateContextFromEvent(ctx, sessionID, projectID, ticketID, workflowName, event)

	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}

func (h *Handler) recordPostToolUse(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	toolName, _ := event["tool_name"].(string)
	category := spawner.ToolCategory(toolName)

	content := "[" + toolName + " result]"
	if toolResponse, ok := event["tool_response"].(string); ok && toolResponse != "" {
		if len(toolResponse) > 200 {
			toolResponse = toolResponse[:200] + "..."
		}
		content = "[" + toolName + " result] " + toolResponse
	}

	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(sessionID, content, category)
	if err != nil {
		logger.Error(ctx, "record_event: failed to record post-tool message", "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	if projectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventMessagesUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflowName,
		}, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	// Bump stall detection (best-effort)
	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, sessionID); sigErr != nil {
			logger.Info(ctx, "record_event: BumpLastMessage error (best-effort)", "error", sigErr)
		}
	}

	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}

// maybeUpdateContextFromEvent parses token usage from a hook event and updates context_left.
func (h *Handler) maybeUpdateContextFromEvent(ctx context.Context, sessionID, projectID, ticketID, workflowName string, event map[string]interface{}) {
	usage, _ := event["usage"].(map[string]interface{})
	if usage == nil {
		return
	}
	input, _ := usage["input_tokens"].(float64)
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)
	cacheCreate, _ := usage["cache_creation_input_tokens"].(float64)
	output, _ := usage["output_tokens"].(float64)
	totalUsed := int(input + cacheRead + cacheCreate + output)
	if totalUsed == 0 {
		return
	}

	// Use 200000 as default max context (no max_context column in agent_sessions)
	ctxLeft := spawner.ComputeContextLeftPct(totalUsed, 200000)

	updProjectID, updTicketID, updWorkflow, updErr := h.agentSvc.UpdateContextLeft(sessionID, ctxLeft)
	if updErr != nil {
		logger.Error(ctx, "record_event: failed to update context", "error", updErr)
		return
	}
	if updProjectID == "" {
		updProjectID = projectID
		updTicketID = ticketID
		updWorkflow = workflowName
	}
	if updProjectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventAgentContextUpdated, service.BroadcastCtx{
			ProjectID: updProjectID,
			TicketID:  updTicketID,
			Workflow:  updWorkflow,
		}, map[string]interface{}{
			"session_id":   sessionID,
			"context_left": ctxLeft,
		})
	}
}
