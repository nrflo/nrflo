package socket

import (
	"context"

	"be/internal/logger"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handleStopHook enforces completion at end-of-turn. For an autonomous running
// session that ended a turn without calling a completion tool, it returns a block
// decision (carrying a finish-reminder) so the CLI keeps the agent going; once the
// block budget is spent it fails the session explicitly instead of letting it
// stall into an implicit pass. Sessions that already have a result, or aren't
// autonomous (interactive/plan/waiting), pass through untouched.
func (h *Handler) handleStopHook(ctx context.Context, req Request, sessionID string) Response {
	block, reason, capExceeded, err := h.agentSvc.StopHookDecision(sessionID)
	if err != nil {
		logger.Info(ctx, "record_event: stop hook decision error (best-effort)", "error", err, "session_id", sessionID)
		return MakeResponse(req.ID, map[string]string{"status": "recorded"})
	}
	if capExceeded {
		bctx, ferr := h.agentSvc.Fail(&types.AgentRequest{SessionID: sessionID, Reason: "unresponsive_after_stop_blocks"})
		if ferr != nil {
			logger.Error(ctx, "record_event: stop-cap fail error", "error", ferr, "session_id", sessionID)
			return MakeResponse(req.ID, map[string]string{"status": "recorded"})
		}
		logger.Warn(ctx, "stop hook: session unresponsive after block budget — failing", "session_id", sessionID, "agent_type", bctx.AgentType)
		service.BroadcastFromCtx(h.wsHub, ws.EventAgentCompleted, bctx, map[string]interface{}{
			"action":     "fail",
			"agent_type": bctx.AgentType,
			"session_id": bctx.SessionID,
			"model_id":   bctx.ModelID,
			"result":     "fail",
		})
		if h.signaler != nil {
			if sigErr := h.signaler.RequestTerminalSignal(bctx.ProjectID, bctx.TicketID, bctx.Workflow, bctx.SessionID, "fail"); sigErr != nil {
				logger.Info(ctx, "record_event: stop-cap terminal signal error (best-effort)", "error", sigErr)
			}
		}
		return MakeResponse(req.ID, map[string]string{"status": "recorded"})
	}
	if block {
		return MakeResponse(req.ID, map[string]interface{}{
			"status":        "recorded",
			"stop_decision": map[string]interface{}{"block": true, "reason": reason},
		})
	}
	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}
