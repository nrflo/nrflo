package socket

import (
	"context"
	"encoding/json"
	"strconv"

	"be/internal/logger"
)

// maxDetailLen bounds every logged detail value so a fat tool name or error
// string can never turn one log line into a payload dump.
const maxDetailLen = 120

// handleAgentRecordEvent is the agent.record_event socket entrypoint: decode
// params/event, log exactly one line for the hook, then dispatch to the
// behavior switch in dispatchRecordEvent. Kept in its own file so
// handler_record_event.go stays under 300 lines.
func (h *Handler) handleAgentRecordEvent(ctx context.Context, req Request) Response {
	var params struct {
		Event      json.RawMessage `json:"event"`
		SessionID  string          `json:"session_id"`
		InstanceID string          `json:"instance_id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return h.rejectRecordEvent(ctx, req, "invalid params: "+err.Error(), "", NewInvalidParamsError(err.Error()))
	}
	if params.SessionID == "" {
		return h.rejectRecordEvent(ctx, req, "session_id is required", "", NewValidationError("session_id is required"))
	}

	var event map[string]interface{}
	if err := json.Unmarshal(params.Event, &event); err != nil {
		return h.rejectRecordEvent(ctx, req, "invalid event JSON: "+err.Error(), params.SessionID, NewInvalidParamsError("invalid event JSON: "+err.Error()))
	}

	hookEventName, _ := event["hook_event_name"].(string)
	logRecordEvent(ctx, hookEventName, params.SessionID, event)

	resp := h.dispatchRecordEvent(ctx, req, params.SessionID, hookEventName, event)
	if resp.Error != nil {
		logger.Warn(ctx, "record_event: dispatch failed", "hook_event_name", hookEventName, "session_id", params.SessionID, "reason", resp.Error.Message)
	}
	return resp
}

// rejectRecordEvent logs a WARN with the rejection reason and returns the
// given error response unchanged.
func (h *Handler) rejectRecordEvent(ctx context.Context, req Request, reason, sessionID string, errInfo *ErrorInfo) Response {
	logger.Warn(ctx, "record_event: rejected", "reason", reason, "session_id", sessionID)
	return MakeErrorResponse(req.ID, errInfo)
}

// logRecordEvent emits exactly one log line per received hook event: INFO for
// lifecycle events, DEBUG for high-volume ones, WARN for anything unrecognized.
func logRecordEvent(ctx context.Context, hookEventName, sessionID string, event map[string]interface{}) {
	level, detailKey, detailVal := recordEventLogFields(hookEventName, event)
	detailVal = truncateDetail(detailVal)

	switch level {
	case "INFO":
		if detailKey != "" {
			logger.Info(ctx, "record_event: "+hookEventName+" received", "session_id", sessionID, detailKey, detailVal)
		} else {
			logger.Info(ctx, "record_event: "+hookEventName+" received", "session_id", sessionID)
		}
	case "DEBUG":
		if detailKey != "" {
			logger.Debug(ctx, "record_event: "+hookEventName+" received", "session_id", sessionID, detailKey, detailVal)
		} else {
			logger.Debug(ctx, "record_event: "+hookEventName+" received", "session_id", sessionID)
		}
	default:
		logger.Warn(ctx, "record_event: unknown hook event", "hook_event_name", hookEventName, "session_id", sessionID)
	}
}

// recordEventLogFields maps a hook_event_name to its log level and a single
// compact detail key/value. Divergence between event types lives only here —
// dispatchRecordEvent stays focused on behavior.
func recordEventLogFields(hookEventName string, event map[string]interface{}) (level, detailKey, detailVal string) {
	switch hookEventName {
	case "SessionStart":
		return "INFO", "source", asString(event["source"])
	case "Stop":
		return "INFO", "stop_hook_active", asString(event["stop_hook_active"])
	case "SubagentStart":
		return "INFO", "agent_type", asString(event["agent_type"])
	case "SubagentStop":
		return "INFO", "", ""
	case "StopFailure":
		return "INFO", "error", extractErrorMessage(event)
	case "PreCompact":
		return "INFO", "trigger", asString(event["trigger"])
	case "Notification":
		kind, tool := classifyNotification(event)
		if tool != "" {
			return "INFO", "kind", kind + ":" + tool
		}
		return "INFO", "kind", kind
	case "PostToolUseFailure":
		return "INFO", "tool", asString(event["tool_name"])
	case "PreToolUse", "PostToolUse":
		return "DEBUG", "tool", asString(event["tool_name"])
	case "UserPromptSubmit":
		return "DEBUG", "prompt_len", strconv.Itoa(len(asString(event["prompt"])))
	case "UserPromptExpansion":
		return "DEBUG", "command", asString(event["command_name"])
	case "SessionEnd":
		return "DEBUG", "", ""
	default:
		return "WARN", "", ""
	}
}

// truncateDetail bounds a logged detail value to maxDetailLen so a fat tool
// name or error string can never dump a payload.
func truncateDetail(v string) string {
	if len(v) <= maxDetailLen {
		return v
	}
	return v[:maxDetailLen] + "..."
}
