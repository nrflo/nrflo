package socket

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/logger"
	"be/internal/spawner"
	"be/internal/ws"
)

// dispatchRecordEvent routes each hook event type to a specific recorder so
// the agent_messages table captures everything visible in the Claude TUI:
// tool calls, user prompts, notifications, turn boundaries, session events.
// Completion is still signaled by `agent finished/fail/continue` — Stop only
// records turn boundaries for visibility.
func (h *Handler) dispatchRecordEvent(ctx context.Context, req Request, sessionID, hookEventName string, event map[string]interface{}) Response {
	switch hookEventName {
	case "PreToolUse":
		recorded := h.recordPreToolUse(ctx, req, sessionID, event)
		return h.consolePreToolApproval(ctx, req, sessionID, event, recorded)
	case "PostToolUse":
		return h.recordPostToolUse(ctx, req, sessionID, event)
	case "PostToolUseFailure":
		return h.recordPostToolFailure(ctx, req, sessionID, event)
	case "UserPromptSubmit":
		// A live console engine that submitted this exact turn itself
		// (claudeEngine.SendUserTurn writes the user_input row before typing
		// the prompt into the PTY) owns the row — recording the hook echo
		// would double it. Human-typed prompts from an attached terminal
		// return handled=false and are recorded here, their only writer. A
		// wired ContextInjector additionally attaches additional_context for
		// console-kind sessions — see handleUserPromptSubmit.
		return h.handleUserPromptSubmit(ctx, req, sessionID, asString(event["prompt"]))
	case "UserPromptExpansion":
		cmd := asString(event["command_name"])
		args := asString(event["command_args"])
		msg := "/" + cmd
		if args != "" {
			msg += " " + args
		}
		return h.recordSimpleEvent(ctx, req, sessionID, msg, "user_input")
	case "Notification":
		return h.handleNotification(ctx, req, sessionID, event)
	case "SubagentStart":
		agentType := asString(event["agent_type"])
		prompt := asString(event["prompt"])
		msg := "subagent started"
		if agentType != "" {
			msg = "[" + agentType + "] subagent started"
		}
		if prompt != "" {
			msg += ": " + prompt
		}
		return h.recordSimpleEvent(ctx, req, sessionID, msg, "subagent")
	case "SubagentStop":
		return h.recordSimpleEvent(ctx, req, sessionID, "subagent complete", "subagent")
	case "StopFailure":
		msg := "turn failed (api error)"
		if errStr := extractErrorMessage(event); errStr != "" {
			msg = "turn failed: " + errStr
		}
		return h.recordSimpleEvent(ctx, req, sessionID, msg, "text")
	case "PreCompact":
		h.tailThinking(ctx, sessionID, asString(event["transcript_path"]))
		trigger := asString(event["trigger"])
		msg := "context compaction"
		if trigger != "" {
			msg += " (" + trigger + ")"
		}
		return h.recordSimpleEvent(ctx, req, sessionID, msg, "text")
	case "SessionStart":
		// Don't record — but use as a TUI-ready signal so the spawner can
		// release the prompt-delivery wait. Idempotent on the spawner side.
		if h.signaler != nil {
			if err := h.signaler.SignalSessionReady(sessionID); err != nil {
				logger.Info(ctx, "record_event: SignalSessionReady error (best-effort)", "error", err)
			}
		}
		h.consoleSessionReady(sessionID)
		return MakeResponse(req.ID, map[string]string{"status": "ready"})
	case "Stop":
		h.consoleTurnEnd(sessionID)
		return h.handleStopHook(ctx, req, sessionID)
	case "SessionEnd":
		// Predictable per-session noise — ignored. Clean up offset state.
		h.thinkingMu.Lock()
		delete(h.thinkingOffsets, sessionID)
		h.thinkingMu.Unlock()
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	default:
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	}
}

// asString coerces a hook field to a string. Strings pass through; numbers,
// bools, and structured payloads (objects/arrays) get a sensible textual form
// — that way schema drift in Claude's hook payloads degrades gracefully
// instead of silently dropping data.
func asString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%v", x)
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return ""
	}
}

// recordPostToolFailure inserts a "[Tool failed] <error>" row when a tool
// invocation errored. PostToolUse only fires on success per Claude docs, so
// we'd otherwise miss tool failures entirely.
// closeToolSpan stamps ended_at onto the matching PreToolUse row (best-effort)
// so the trace timeline can render the tool call as a duration bar.
func (h *Handler) closeToolSpan(ctx context.Context, sessionID string, event map[string]interface{}) {
	toolUseID, _ := event["tool_use_id"].(string)
	if toolUseID == "" {
		return
	}
	if err := h.agentSvc.MarkToolEnded(sessionID, toolUseID); err != nil {
		logger.Info(ctx, "record_event: MarkToolEnded error (best-effort)", "error", err, "session_id", sessionID)
	}
}

func (h *Handler) recordPostToolFailure(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	toolName, _ := event["tool_name"].(string)
	h.closeToolSpan(ctx, sessionID, event)
	h.consoleToolResult(sessionID, toolName, true)
	category := spawner.ToolCategory(toolName)
	content := "[" + toolName + " failed]"
	if msg := extractErrorMessage(event); msg != "" {
		content += " " + msg
	} else if body := extractToolResultBody(event); body != "" {
		content += " " + body
	}
	return h.recordSimpleEvent(ctx, req, sessionID, content, category)
}

// extractErrorMessage pulls a human-readable error string from an event.
// Claude error fields appear as strings on some hooks, structured objects
// (with `message` / `code` / `type` fields) on others.
func extractErrorMessage(event map[string]interface{}) string {
	switch v := event["error"].(type) {
	case string:
		return v
	case map[string]interface{}:
		for _, k := range []string{"message", "msg", "detail", "description"} {
			if s, ok := v[k].(string); ok && s != "" {
				return s
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return ""
}

// recordSimpleEvent inserts a single agent_messages row with the given content +
// category, broadcasts messages.updated, and bumps stall detection.
func (h *Handler) recordSimpleEvent(ctx context.Context, req Request, sessionID, content, category string) Response {
	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(sessionID, content, category, "")
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	broadcastMessageEvent(h.wsHub, ws.EventMessagesUpdated, projectID, ticketID, workflowName, sessionID, map[string]interface{}{
		"session_id": sessionID,
	})
	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, sessionID); sigErr != nil {
			logger.Info(ctx, "record_event: BumpLastMessage error (best-effort)", "error", sigErr)
		}
		// Also push the content through SetLastMessage so the periodic
		// "agent status" log line shows what the agent is doing in
		// interactive CLI mode (where the PTY ferry drops raw bytes).
		if sigErr := h.signaler.SetLastMessage(projectID, ticketID, workflowName, sessionID, content); sigErr != nil {
			logger.Info(ctx, "record_event: SetLastMessage error (best-effort)", "error", sigErr)
		}
	}
	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}

func (h *Handler) recordPreToolUse(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	h.tailThinking(ctx, sessionID, asString(event["transcript_path"]))

	toolName, _ := event["tool_name"].(string)
	toolInput, _ := event["tool_input"].(map[string]interface{})

	content := spawner.FormatToolDetail(toolName, toolInput)
	category := spawner.ToolCategory(toolName)

	// Persist tool_use_id + the raw tool input so PostToolUse can close the
	// span (trace tool bars) and the UI tool card can render the invocation.
	toolUseID, _ := event["tool_use_id"].(string)
	var rawInput []byte
	if len(toolInput) > 0 {
		rawInput, _ = json.Marshal(toolInput)
	}
	payload := spawner.BuildToolInvokePayload(toolUseID, rawInput)

	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(sessionID, content, category, payload)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	broadcastMessageEvent(h.wsHub, ws.EventMessagesUpdated, projectID, ticketID, workflowName, sessionID, map[string]interface{}{
		"session_id": sessionID,
	})

	// Bump stall detection (best-effort) and surface tool name in status log
	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, sessionID); sigErr != nil {
			logger.Info(ctx, "record_event: BumpLastMessage error (best-effort)", "error", sigErr)
		}
		if sigErr := h.signaler.SetLastMessage(projectID, ticketID, workflowName, sessionID, content); sigErr != nil {
			logger.Info(ctx, "record_event: SetLastMessage error (best-effort)", "error", sigErr)
		}
	}

	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}

// recordPostToolUse inserts a "[tool] → output" result row so the agent log
// shows what each tool returned (matching api-mode). PostToolUse fires only on
// success per Claude docs; a tool that returns its own error content inline
// (e.g. an MCP isError result) still surfaces here as the captured body.
func (h *Handler) recordPostToolUse(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	toolName, _ := event["tool_name"].(string)
	h.closeToolSpan(ctx, sessionID, event)
	h.consoleToolResult(sessionID, toolName, false)
	if spawner.IsHiddenResultTool(toolName) {
		// Read/Bash/Edit success rows are suppressed: the PreToolUse invoke row
		// already shows the file/command and the output is log noise. PostToolUse
		// fires only on success, so errors (a separate path) stay visible.
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	}
	body := extractToolResultBody(event)
	if body == "" {
		// Tool returned nothing renderable — ack without inserting an empty row.
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	}
	content := spawner.FormatToolResult(toolName, body, false)
	return h.recordSimpleEvent(ctx, req, sessionID, content, "tool")
}
