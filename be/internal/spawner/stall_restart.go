package spawner

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// checkStall checks if a process has stalled and returns true if a stall restart was triggered.
// Must be called only for still-running processes that are not in low-context save mode.
func (s *Spawner) checkStall(ctx context.Context, proc *processInfo, req SpawnRequest) bool {
	if proc.lowContextSaving {
		return false
	}
	if proc.stallRestartCount >= maxStallRestarts {
		return false
	}

	now := s.config.Clock.Now()
	proc.messagesMutex.Lock()
	sinceLastMsg := now.Sub(proc.lastMessageTime)
	hasMsg := proc.hasReceivedMessage
	proc.messagesMutex.Unlock()

	if !hasMsg && proc.stallStartTimeout > 0 && sinceLastMsg > proc.stallStartTimeout {
		logger.Warn(ctx, "stall detected: no output since start",
			"agent_type", proc.agentType, "session_id", proc.sessionID, "elapsed", sinceLastMsg)
		s.handleStallRestart(ctx, proc, req, "start_stall")
		return true
	}

	if hasMsg && proc.stallRunningTimeout > 0 && sinceLastMsg > proc.stallRunningTimeout {
		logger.Warn(ctx, "stall detected: no output mid-execution",
			"agent_type", proc.agentType, "session_id", proc.sessionID, "elapsed", sinceLastMsg)
		s.handleStallRestart(ctx, proc, req, "running_stall")
		return true
	}

	return false
}

// handleStallRestart kills a stalled agent and prepares it for continuation.
// Unlike low-context restart, no context save is attempted (agent is frozen).
// Unlike fail restart, no delay before retry (agent is stuck).
func (s *Spawner) handleStallRestart(ctx context.Context, proc *processInfo, req SpawnRequest, reason string) {
	stallType := "start"
	if reason == "running_stall" {
		stallType = "running"
	}

	// Broadcast stall restart event before killing
	proc.messagesMutex.Lock()
	sinceLastMsg := s.config.Clock.Now().Sub(proc.lastMessageTime)
	proc.messagesMutex.Unlock()

	s.broadcast(ws.EventAgentStallRestart, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"session_id":              proc.sessionID,
		"agent_type":              proc.agentType,
		"stall_type":              stallType,
		"stall_count":             proc.stallRestartCount + 1,
		"time_since_last_message": fmt.Sprintf("%.0fs", sinceLastMsg.Seconds()),
	})

	// Kill agent: SIGTERM → grace → SIGKILL
	if proc.cmd.Process != nil {
		proc.cmd.Process.Signal(syscall.SIGTERM)
	}

	gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
	if gracePeriod == 0 {
		gracePeriod = 5 * time.Second
	}

	select {
	case <-proc.doneCh:
	case <-time.After(gracePeriod):
		if proc.cmd.Process != nil {
			proc.cmd.Process.Kill()
		}
		<-proc.doneCh
	}

	// Flush pending messages
	s.saveMessages(proc)

	// Register stop with stall reason and mark as continued
	resultReason := "stall_restart_" + reason
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", resultReason, proc.modelID)

	// Update session status to continued
	if pool := s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
		sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
	}

	// Increment stall restart count and set continuation status
	proc.stallRestartCount++
	proc.finalStatus = "CONTINUE"
}

// waitBeforeStallRetry waits for defaultFailRetryDelay (15s) before retrying a stalled agent.
// Returns true if the wait completed, false if the context was cancelled (should not retry).
// Broadcasts an agent.stall_waiting event before sleeping.
func (s *Spawner) waitBeforeStallRetry(ctx context.Context, proc *processInfo, req SpawnRequest) bool {
	s.broadcast(ws.EventAgentStallWaiting, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"agent_type":          proc.agentType,
		"session_id":          proc.sessionID,
		"model_id":            proc.modelID,
		"delay_seconds":       int(defaultFailRetryDelay.Seconds()),
		"stall_restart_count": proc.stallRestartCount,
	})
	logger.Info(ctx, "waiting before stall restart", "delay", defaultFailRetryDelay, "model", proc.modelID)
	select {
	case <-ctx.Done():
		return false
	case <-time.After(defaultFailRetryDelay):
		return true
	}
}
