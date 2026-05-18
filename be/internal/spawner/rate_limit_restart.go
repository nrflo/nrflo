package spawner

import (
	"context"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// handleRateLimitRetry handles an agent that exited due to a provider rate limit.
// Mirrors handleStallRestart: broadcasts the event, kills the (already-exited)
// process, flushes messages, registers the stop as continued, persists the
// rate_limit_until_ts, increments rateLimitRetryCount, and sets
// proc.finalStatus=CONTINUE so the existing relaunch site in monitorAll fires.
//
// Call site: handleCompletion when ClassifyExit returns RetryClassRateLimit.
// The process has already exited at this point, so the SIGTERM→SIGKILL sequence
// is a safe no-op (doneCh is already closed).
func (s *Spawner) handleRateLimitRetry(ctx context.Context, proc *processInfo, req SpawnRequest, matchedPattern string) {
	upcomingCount := proc.rateLimitRetryCount + 1
	delay := computeRateLimitDelay(proc.rateLimitConfig, upcomingCount)

	s.broadcast(ws.EventAgentRateLimited, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"session_id":         proc.sessionID,
		"agent_type":         proc.agentType,
		"wait_seconds":       int(delay.Seconds()),
		"total_wait_seconds": int(proc.rateLimitTotalWait.Seconds()) + int(delay.Seconds()),
		"matched_pattern":    matchedPattern,
		"retry_count":        upcomingCount,
	})

	// Kill the process gracefully (no-op when already exited).
	proc.backend.Kill(ctx, proc, syscall.SIGTERM)
	gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
	if gracePeriod == 0 {
		gracePeriod = 5 * time.Second
	}
	select {
	case <-proc.doneCh:
	case <-s.config.Clock.After(gracePeriod):
		proc.backend.Kill(ctx, proc, syscall.SIGKILL)
		<-proc.doneCh
	}

	s.saveMessages(proc)

	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", "rate_limit", proc.modelID)

	if pool := s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
		sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
		rateLimitUntil := s.config.Clock.Now().Add(delay).UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		sessionRepo.UpdateRateLimitUntil(proc.sessionID, rateLimitUntil, upcomingCount, matchedPattern)
	}

	proc.rateLimitRetryCount++
	proc.finalStatus = "CONTINUE"
}

// waitForRateLimitRetry sleeps for the exponential-backoff delay before the
// next spawn. Uses the clock abstraction so tests can control time without
// real sleeps. Returns true if the wait completed, false if ctx was cancelled.
func (s *Spawner) waitForRateLimitRetry(ctx context.Context, proc *processInfo, req SpawnRequest) bool {
	delay := computeRateLimitDelay(proc.rateLimitConfig, proc.rateLimitRetryCount)

	logger.Info(ctx, "rate-limit retry: waiting before relaunch",
		"delay", delay,
		"retry_count", proc.rateLimitRetryCount,
		"model", proc.modelID,
		"session_id", proc.sessionID,
	)

	select {
	case <-ctx.Done():
		return false
	case <-s.config.Clock.After(delay):
		proc.rateLimitTotalWait += delay
		return true
	}
}
