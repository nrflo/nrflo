package spawner

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/ws"
)

// handleGracefulTimeout sends SIGTERM, waits for grace period, then SIGKILL
func (s *Spawner) handleGracefulTimeout(ctx context.Context, proc *processInfo, req SpawnRequest) {
	proc.elapsed = time.Since(proc.startTime)

	// Send SIGTERM first
	if proc.cmd.Process != nil {
		proc.cmd.Process.Signal(syscall.SIGTERM)
	}

	// Grace period for clean shutdown
	gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
	if gracePeriod == 0 {
		gracePeriod = 5 * time.Second
	}

	select {
	case <-proc.doneCh:
		// Exited gracefully after SIGTERM
	case <-time.After(gracePeriod):
		// Force kill
		if proc.cmd.Process != nil {
			proc.cmd.Process.Kill()
		}
		<-proc.doneCh // Wait for the wait goroutine to finish
	}

	proc.finalStatus = "TIMEOUT"
	logger.Warn(ctx, "agent timed out", "model", proc.modelID, "timeout", proc.timeout)

	// Final messages flush
	s.saveMessages(proc)

	// Register agent stop with timeout reason (also updates status to failed + sets ended_at)
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.sessionID, proc.agentID, "fail", "timeout", proc.modelID)

	if s.config.ErrorSvc != nil {
		s.config.ErrorSvc.RecordError(req.ProjectID, "agent", proc.sessionID, fmt.Sprintf("%s: timeout after %ds", proc.agentType, int(proc.timeout.Seconds())))
	}
}

// handleCompletion handles a completed agent process with hybrid completion semantics
func (s *Spawner) handleCompletion(ctx context.Context, proc *processInfo, req SpawnRequest) {
	exitCode := 0
	if proc.cmd.ProcessState != nil {
		exitCode = proc.cmd.ProcessState.ExitCode()
	}

	var result, resultReason string

	if exitCode != 0 {
		// Non-zero exit = immediate fail
		result = "fail"
		resultReason = "exit_code"
		proc.finalStatus = "FAIL"
	} else {
		// Exit 0: check DB for explicit fail/continue/callback set before process exited
		if explicit := s.getAgentResult(proc); explicit == "fail" || explicit == "continue" || explicit == "callback" {
			result = explicit
			resultReason = "explicit"
		} else {
			result = "pass"
			resultReason = "implicit"
		}

		switch result {
		case "pass":
			proc.finalStatus = "PASS"
		case "continue":
			proc.finalStatus = "CONTINUE"
		case "callback":
			proc.finalStatus = "CALLBACK"
		default:
			proc.finalStatus = "FAIL"
		}
	}

	// Save messages to database
	s.saveMessages(proc)

	// Register agent stop with reason
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.sessionID, proc.agentID, result, resultReason, proc.modelID)

	if result == "fail" && s.config.ErrorSvc != nil {
		s.config.ErrorSvc.RecordError(req.ProjectID, "agent", proc.sessionID, fmt.Sprintf("%s: %s", proc.agentType, resultReason))
	}

	logger.Info(ctx, "agent completed", "model", proc.modelID, "status", proc.finalStatus, "exit_code", exitCode, "reason", resultReason, "duration", proc.elapsed.Round(time.Second))
}

// relaunchForContinuation spawns a new agent process to continue where the previous one left off.
// It preserves the ancestor session chain and increments the continuation count.
func (s *Spawner) relaunchForContinuation(ctx context.Context, oldProc *processInfo, req SpawnRequest, phase string) (*processInfo, error) {
	// Determine ancestor session ID (root of the continuation chain)
	ancestorID := oldProc.ancestorSessionID
	if ancestorID == "" {
		// First continuation — the old session is the ancestor
		ancestorID = oldProc.sessionID
	}

	// Spawn a new process with the same model
	newProc, err := s.spawnSingle(req, oldProc.modelID, phase, oldProc.workflowInstanceID)
	if err != nil {
		return nil, err
	}

	// Carry over continuation tracking
	newProc.ancestorSessionID = ancestorID
	newProc.restartCount = oldProc.restartCount + 1
	newProc.restartThreshold = oldProc.restartThreshold
	newProc.maxFailRestarts = oldProc.maxFailRestarts
	newProc.failRestartCount = oldProc.failRestartCount
	newProc.stallRestartCount = oldProc.stallRestartCount
	newProc.stallStartTimeout = oldProc.stallStartTimeout
	newProc.stallRunningTimeout = oldProc.stallRunningTimeout

	// Update the ancestor_session_id and restart_count on the new DB session record
	if pool := s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
		sessionRepo.UpdateAncestorSession(newProc.sessionID, ancestorID)
		sessionRepo.UpdateRestartCount(newProc.sessionID, newProc.restartCount)
	}

	// Broadcast continuation event
	s.broadcast(ws.EventAgentContinued, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"old_session_id":     oldProc.sessionID,
		"new_session_id":     newProc.sessionID,
		"ancestor_session":   ancestorID,
		"restart_count":      newProc.restartCount,
		"agent_type":         req.AgentType,
		"model_id":           oldProc.modelID,
	})

	logger.Info(ctx, "agent continuation started", "model", oldProc.modelID, "new_session", newProc.sessionID, "ancestor", ancestorID, "restart_count", newProc.restartCount)

	return newProc, nil
}

// getAgentResult reads the explicit result from agent_sessions table
func (s *Spawner) getAgentResult(proc *processInfo) string {
	pool := s.pool()
	if pool == nil {
		return ""
	}

	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	session, err := sessionRepo.Get(proc.sessionID)
	if err != nil {
		return ""
	}

	if session.Result.Valid {
		return session.Result.String
	}
	return ""
}

// maybeFlushMessages flushes messages to DB if interval elapsed
func (s *Spawner) maybeFlushMessages(proc *processInfo) {
	interval := time.Duration(s.config.MessageFlushIntervalMs) * time.Millisecond
	if interval == 0 {
		interval = 2 * time.Second
	}

	shouldFlush := time.Since(proc.lastMessagesFlush) >= interval

	if shouldFlush {
		if proc.messagesDirty {
			s.saveMessages(proc)
			proc.messagesDirty = false
		}
		proc.lastMessagesFlush = s.config.Clock.Now()
	}
}
