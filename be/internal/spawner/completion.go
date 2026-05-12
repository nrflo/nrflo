package spawner

import (
	"context"
	"encoding/json"
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
	proc.backend.Kill(ctx, proc, syscall.SIGTERM)

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
		proc.backend.Kill(ctx, proc, syscall.SIGKILL)
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
	if proc.cmd != nil && proc.cmd.ProcessState != nil {
		exitCode = proc.cmd.ProcessState.ExitCode()
	} else if proc.cmd == nil && proc.waitErr != nil {
		// Non-exec.Cmd backends (e.g. cli_interactive) signal failure via waitErr.
		exitCode = 1
	}

	var result, resultReason string

	// Always check the DB for an explicit result first. Agents call
	// `nrflo agent finished/fail/continue/callback`, the socket handler writes
	// the result + reason, then dispatches a terminal signal that kills the
	// process — Claude (and other TUIs) exit non-zero when killed by signal,
	// so trusting exit_code alone would clobber the explicit signal.
	switch explicit := s.getAgentResult(proc); explicit {
	case "fail":
		result = "fail"
		resultReason = s.getAgentResultReason(proc)
		if resultReason == "" {
			resultReason = "explicit"
		}
		proc.finalStatus = "FAIL"
	case "continue":
		result = "continue"
		resultReason = s.getAgentResultReason(proc)
		if resultReason == "" {
			resultReason = "explicit"
		}
		proc.finalStatus = "CONTINUE"
	case "callback":
		result = "callback"
		resultReason = s.getAgentResultReason(proc)
		if resultReason == "" {
			resultReason = "explicit"
		}
		proc.finalStatus = "CALLBACK"
	case "pass":
		result = "pass"
		resultReason = s.getAgentResultReason(proc)
		if resultReason == "" {
			resultReason = "explicit"
		}
		proc.finalStatus = "PASS"
	default:
		// No explicit signal — fall back to exit code.
		if exitCode != 0 {
			result = "fail"
			resultReason = "exit_code"
			proc.finalStatus = "FAIL"
		} else {
			result = "pass"
			resultReason = "implicit"
			proc.finalStatus = "PASS"
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
	newProc, err := s.spawnSingle(ctx, req, oldProc.modelID, phase, oldProc.workflowInstanceID)
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

	// Copy findings from old session so relaunched agents do not lose keys other than to_resume.
	s.copyFindingsForContinuation(ctx, oldProc.sessionID, newProc.sessionID)

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

// getAgentResultReason reads the result_reason column for an agent session.
func (s *Spawner) getAgentResultReason(proc *processInfo) string {
	pool := s.pool()
	if pool == nil {
		return ""
	}
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	session, err := sessionRepo.Get(proc.sessionID)
	if err != nil {
		return ""
	}
	if session.ResultReason.Valid {
		return session.ResultReason.String
	}
	return ""
}

// copyFindingsForContinuation merges findings from oldSessionID into newSessionID non-destructively
// (new-session keys win on conflict). Covers both low-context and fail-restart relaunches.
// All errors are logged as warnings so they never block the relaunch.
func (s *Spawner) copyFindingsForContinuation(ctx context.Context, oldSessionID, newSessionID string) {
	pool := s.pool()
	if pool == nil {
		return
	}
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)

	oldSession, err := sessionRepo.Get(oldSessionID)
	if err != nil {
		logger.Warn(ctx, "findings carryover: failed to load old session", "old_session_id", oldSessionID, "err", err)
		return
	}

	oldMap := oldSession.GetFindings()
	if len(oldMap) == 0 {
		return
	}

	newSession, err := sessionRepo.Get(newSessionID)
	if err != nil {
		logger.Warn(ctx, "findings carryover: failed to load new session", "new_session_id", newSessionID, "err", err)
		return
	}

	newMap := newSession.GetFindings()

	for k, v := range oldMap {
		if _, exists := newMap[k]; !exists {
			newMap[k] = v
		}
	}

	jsonBytes, err := json.Marshal(newMap)
	if err != nil {
		logger.Warn(ctx, "findings carryover: failed to marshal merged findings", "new_session_id", newSessionID, "err", err)
		return
	}

	if err := sessionRepo.UpdateFindings(newSessionID, string(jsonBytes)); err != nil {
		logger.Warn(ctx, "findings carryover: failed to persist merged findings", "new_session_id", newSessionID, "err", err)
	}
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
