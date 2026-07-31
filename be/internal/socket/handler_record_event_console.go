package socket

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/model"
)

// consolePreToolApproval routes a PreToolUse event through ConsoleHooks (when
// a live console engine is registered for the session), blocking until a
// human answers or the engine denies on timeout/stop. handled=false (no live
// engine — the common autonomous-session case) returns recorded unchanged, so
// today's response and row behavior stay byte-identical. A failed record
// (recorded carries an Error) is passed through untouched rather than
// overwritten with an approval: the CLI's console hook fails closed on an
// error response, so the DB failure denies the tool instead of vanishing.
func (h *Handler) consolePreToolApproval(ctx context.Context, req Request, sessionID string, event map[string]interface{}, recorded Response) Response {
	if h.consoleHooks == nil || recorded.Error != nil {
		return recorded
	}
	toolName, _ := event["tool_name"].(string)
	toolInput, _ := event["tool_input"].(map[string]interface{})
	toolUseID, _ := event["tool_use_id"].(string)

	decision, reason, handled := h.consoleHooks.ApproveConsoleTool(ctx, sessionID, toolName, toolInput, toolUseID)
	if !handled {
		return recorded
	}
	return MakeResponse(req.ID, map[string]interface{}{
		"status": "recorded",
		"permission_decision": map[string]string{
			"decision": decision,
			"reason":   reason,
		},
	})
}

// consoleTurnEnd notifies a live console engine (if any) that a Stop hook
// fired, so it can flush its transcript tail and emit turn_completed.
// Nil-safe no-op for autonomous sessions.
func (h *Handler) consoleTurnEnd(sessionID string) {
	if h.consoleHooks == nil {
		return
	}
	h.consoleHooks.ConsoleTurnEnd(sessionID)
}

// consoleToolResult notifies a live console engine (if any) that a
// PostToolUse/PostToolUseFailure hook fired. Nil-safe no-op for autonomous
// sessions.
func (h *Handler) consoleToolResult(sessionID, toolName string, isError bool) {
	if h.consoleHooks == nil {
		return
	}
	h.consoleHooks.ConsoleToolResult(sessionID, toolName, isError)
}

// consoleSessionReady notifies a live console engine (if any) that
// SessionStart fired, unblocking its TUI-ready wait. Nil-safe no-op for
// autonomous sessions.
func (h *Handler) consoleSessionReady(sessionID string) {
	if h.consoleHooks == nil {
		return
	}
	h.consoleHooks.ConsoleSessionReady(sessionID)
}

// handleUserPromptSubmit runs the existing echo/record split for a
// UserPromptSubmit event (engine-owned turns return recorded:false; otherwise
// the prompt is recorded as usual) and, when a ContextInjector is wired,
// attaches additional_context to the result so the CLI can surface it as
// hookSpecificOutput.additionalContext. A nil injector or an empty/error
// result leaves the response byte-identical to the pre-injector shape.
func (h *Handler) handleUserPromptSubmit(ctx context.Context, req Request, sessionID, prompt string) Response {
	var injected string
	if h.contextInjector != nil {
		injected = h.contextInjector.InjectUserPromptContext(ctx, sessionID, prompt)
	}

	var resp Response
	if h.consoleHooks != nil && h.consoleHooks.ConsoleUserPrompt(sessionID, prompt) {
		resp = MakeResponse(req.ID, map[string]interface{}{"recorded": false})
	} else {
		category := "user_input"
		if isTaskNotification(prompt) {
			category = model.MsgCategoryTaskNotification
		}
		resp = h.recordSimpleEvent(ctx, req, sessionID, prompt, category)
	}

	if injected == "" || resp.Error != nil {
		return resp
	}
	return addAdditionalContext(req.ID, resp, injected)
}

// isTaskNotification reports whether prompt is a Claude Code CLI harness
// <task-notification> envelope (injected when a backgrounded MCP
// get_delegation call resolves), rather than a human-typed prompt.
func isTaskNotification(prompt string) bool {
	return strings.HasPrefix(strings.TrimSpace(prompt), "<task-notification>")
}

// addAdditionalContext merges an additional_context key into resp's result
// map, preserving every existing key. Returns resp unchanged if the result
// isn't a JSON object (defensive; every UserPromptSubmit branch produces one).
func addAdditionalContext(id string, resp Response, injected string) Response {
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return resp
	}
	result["additional_context"] = injected
	return MakeResponse(id, result)
}
