package socket

import "context"

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

// consoleSessionReady notifies a live console engine (if any) that
// SessionStart fired, unblocking its TUI-ready wait. Nil-safe no-op for
// autonomous sessions.
func (h *Handler) consoleSessionReady(sessionID string) {
	if h.consoleHooks == nil {
		return
	}
	h.consoleHooks.ConsoleSessionReady(sessionID)
}
