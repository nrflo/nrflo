package spawner

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/ws"
)

// handleGracefulTimeout sends SIGTERM, waits for grace period, then SIGKILL
func (s *Spawner) handleGracefulTimeout(proc *processInfo, req SpawnRequest) {
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
	fmt.Fprintf(os.Stderr, "  %s timed out after %v\n", proc.modelID, proc.timeout)

	// Final messages flush
	s.saveMessages(proc)

	// Register agent stop with timeout reason (also updates status to failed + sets ended_at)
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.sessionID, proc.agentID, "fail", "timeout", proc.modelID)
}

// handleCompletion handles a completed agent process with hybrid completion semantics
func (s *Spawner) handleCompletion(proc *processInfo, req SpawnRequest) {
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
		// Exit 0: check for explicit completion within grace period
		gracePeriod := time.Duration(s.config.CompletionGraceSec) * time.Second
		if gracePeriod == 0 {
			gracePeriod = 60 * time.Second
		}

		deadline := time.Now().Add(gracePeriod)
		for time.Now().Before(deadline) {
			explicit := s.getAgentResult(proc)
			if explicit == "pass" {
				result = "pass"
				resultReason = "explicit"
				break
			} else if explicit == "fail" {
				result = "fail"
				resultReason = "explicit"
				break
			} else if explicit == "continue" {
				result = "continue"
				resultReason = "explicit"
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		if result == "" {
			// No explicit completion within grace period
			result = "fail"
			resultReason = "no_complete"
		}

		switch result {
		case "pass":
			proc.finalStatus = "PASS"
		case "continue":
			proc.finalStatus = "CONTINUE"
		default:
			proc.finalStatus = "FAIL"
		}
	}

	// Save messages to database
	s.saveMessages(proc)

	// Register agent stop with reason
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.sessionID, proc.agentID, result, resultReason, proc.modelID)

	fmt.Printf("  %s: %s (exit code: %d, reason: %s, duration: %v)\n",
		proc.modelID, proc.finalStatus, exitCode, resultReason, proc.elapsed.Round(time.Second))
}

// relaunchForContinuation spawns a new agent process to continue where the previous one left off.
// It preserves the ancestor session chain and increments the continuation count.
func (s *Spawner) relaunchForContinuation(oldProc *processInfo, req SpawnRequest, phase string) (*processInfo, error) {
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
	newProc.continuationCount = oldProc.continuationCount + 1

	// Update the ancestor_session_id on the new DB session record
	database, dbErr := db.Open(s.config.DataPath)
	if dbErr == nil {
		sessionRepo := repo.NewAgentSessionRepo(database)
		sessionRepo.UpdateAncestorSession(newProc.sessionID, ancestorID)
		database.Close()
	}

	// Broadcast continuation event
	s.broadcast(ws.EventAgentContinued, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"old_session_id":     oldProc.sessionID,
		"new_session_id":     newProc.sessionID,
		"ancestor_session":   ancestorID,
		"continuation_count": newProc.continuationCount,
		"agent_type":         req.AgentType,
		"model_id":           oldProc.modelID,
	})

	fmt.Printf("  Started continuation %s (PID: %d, Session: %s, Ancestor: %s)\n",
		oldProc.modelID, newProc.cmd.Process.Pid, newProc.sessionID, ancestorID)

	return newProc, nil
}

// getAgentResult reads the explicit result from agent_sessions table
func (s *Spawner) getAgentResult(proc *processInfo) string {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return ""
	}
	defer database.Close()

	sessionRepo := repo.NewAgentSessionRepo(database)
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
		if proc.messagesDirty || proc.rawOutputDirty {
			s.saveMessages(proc)
			proc.messagesDirty = false
		}
		s.saveContextLeft(proc)
		proc.lastMessagesFlush = time.Now()
	}
}
