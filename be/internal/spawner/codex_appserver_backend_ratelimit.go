package spawner

import (
	"context"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// handleRateLimit mirrors apiBackend.Start's rate-limit dance: broadcast,
// register a continue stop, persist rate-limit state, wait, and set
// finalStatus=CONTINUE so monitorAll relaunches (or FAIL when exhausted).
func (b *codexAppServerBackend) handleRateLimit(proc *processInfo, req SpawnRequest, matched string) {
	b.s.saveMessages(proc)
	if !proc.rateLimitConfig.Enabled || proc.rateLimitTotalWait >= proc.rateLimitConfig.MaxWait {
		proc.finalStatus = "FAIL"
		b.s.registerAgentStopWithReason(proc.projectID, proc.ticketID, proc.workflowName,
			proc.sessionID, proc.agentID, "fail", "rate_limit_exhausted", proc.modelID)
		return
	}
	upcomingCount := proc.rateLimitRetryCount + 1
	delay := computeRateLimitDelay(proc.rateLimitConfig, upcomingCount)
	b.s.broadcast(ws.EventAgentRateLimited, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
		"session_id":         proc.sessionID,
		"agent_type":         proc.agentType,
		"wait_seconds":       int(delay.Seconds()),
		"total_wait_seconds": int(proc.rateLimitTotalWait.Seconds()) + int(delay.Seconds()),
		"matched_pattern":    matched,
		"retry_count":        upcomingCount,
	})
	b.s.registerAgentStopWithReason(proc.projectID, proc.ticketID, proc.workflowName,
		proc.sessionID, proc.agentID, "continue", "rate_limit", proc.modelID)
	if pool := b.s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, b.s.config.Clock)
		sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
		rateLimitUntil := b.s.config.Clock.Now().Add(delay).UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		sessionRepo.UpdateRateLimitUntil(proc.sessionID, rateLimitUntil, upcomingCount, "")
	}
	proc.rateLimitRetryCount++
	b.s.waitForRateLimitRetry(context.Background(), proc, req)
	proc.finalStatus = "CONTINUE"
}
