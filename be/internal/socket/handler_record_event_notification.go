package socket

import (
	"context"
	"strings"

	"be/internal/logger"
	"be/internal/model"
)

// handleNotification records a Claude Notification hook event and, when the
// payload indicates the agent is parked (idle-waiting or a permission
// prompt) for an autonomous workflow_agent session, asks the wired
// TerminalSignaler to fire the existing idle-nudge machinery immediately
// instead of waiting out the wall-clock idle window. Console/console_chat/
// observer sessions (and lookup failures) always keep today's byte-identical
// recording — only workflow_agent sessions get the permission-marker
// rewrite and a nudge trigger. Kept in its own file so handler_record_event.go
// stays under 300 lines.
func (h *Handler) handleNotification(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	kind, tool := classifyNotification(event)

	isWorkflowAgent := false
	if kind, err := h.agentSvc.GetSessionKind(sessionID); err == nil {
		isWorkflowAgent = kind == model.AgentSessionKindWorkflowAgent
	}

	content := asString(event["message"])
	category := "text"
	if kind == "permission" && isWorkflowAgent {
		category = "permission"
		if tool != "" {
			content = "[permission prompt] " + tool
		} else if content == "" {
			content = "[permission prompt]"
		}
	}

	resp := h.recordSimpleEvent(ctx, req, sessionID, content, category)

	if isWorkflowAgent && (kind == "idle" || kind == "permission") {
		if h.signaler != nil {
			if err := h.signaler.TriggerIdleNudge(sessionID, kind); err != nil {
				logger.Info(ctx, "record_event: TriggerIdleNudge error (best-effort)", "error", err, "session_id", sessionID, "kind", kind)
			}
		}
	}

	return resp
}

// classifyNotification determines whether a Notification hook payload
// indicates the agent is parked waiting for input ("idle"), blocked on a
// permission prompt ("permission"), or something else ("unknown"). Checks a
// structured field first (in case Claude ever emits one), then falls back to
// a substring match on the message text. For "permission", also returns the
// tool name when it can be extracted from the message. Unknown payloads
// no-op and are recorded exactly as before this feature existed.
func classifyNotification(event map[string]interface{}) (kind, tool string) {
	if t := asString(event["type"]); t != "" {
		switch t {
		case "idle", "waiting_for_input":
			return "idle", ""
		case "permission", "permission_prompt":
			return "permission", asString(event["tool_name"])
		}
	}
	if t := asString(event["notification_type"]); t != "" {
		switch t {
		case "idle", "waiting_for_input":
			return "idle", ""
		case "permission", "permission_prompt":
			return "permission", asString(event["tool_name"])
		}
	}

	message := asString(event["message"])
	if message == "" {
		return "unknown", ""
	}

	lower := strings.ToLower(message)
	if strings.Contains(lower, "waiting for your input") {
		return "idle", ""
	}
	if strings.Contains(lower, "needs your permission") || strings.Contains(lower, "permission to use") {
		if idx := strings.Index(lower, "permission to use "); idx >= 0 {
			rest := message[idx+len("permission to use "):]
			rest = strings.TrimSpace(rest)
			if end := strings.IndexAny(rest, " .\n"); end >= 0 {
				rest = rest[:end]
			}
			return "permission", rest
		}
		return "permission", ""
	}

	return "unknown", ""
}
