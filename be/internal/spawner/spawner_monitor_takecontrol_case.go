package spawner

import (
	"context"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/ws"
)

// handleTakeControlRequest processes one take-control request pulled off
// s.takeControlCh inside monitorAll's select loop: validates the backend
// supports it, viewer-attaches (cli_interactive) or kills+registers a resume
// launch, then blocks until the interactive session completes. Extracted
// verbatim out of monitorAll to keep that file under its line cap; running is
// passed by value and the (possibly shortened) slice returned, mirroring how
// monitorAll owned it inline. completedProc is non-nil only when the
// interactive session fully finished (kill+resume path); nil for a
// viewer-attach or an unmatched/rejected request.
func (s *Spawner) handleTakeControlRequest(ctx context.Context, running []*processInfo, req SpawnRequest, takeControlSessionID string) (updatedRunning []*processInfo, completedProc *processInfo) {
	for i, proc := range running {
		if proc.sessionID != takeControlSessionID {
			continue
		}
		if proc.backend == nil || !proc.backend.SupportsTakeControl() {
			cliName, _ := parseModelID(proc.modelID)
			logger.Error(ctx, "take-control: backend does not support take-control", "cli", cliName, "session_id", takeControlSessionID)
			s.rejectTakeControl(req, proc, takeControlSessionID, "api_mode_unsupported")
			return running, nil
		}

		// Interactive backend: viewer-attach — broadcast but do NOT kill or block.
		// The agent keeps running; the viewer connects via /api/v1/pty/{session_id}.
		// No exit-interactive call is made on disconnect (completePtyInteractive is skipped).
		if proc.backend.Name() == "cli_interactive" {
			logger.Info(ctx, "take-control: viewer attach (interactive backend)", "session_id", takeControlSessionID)
			s.broadcast(ws.EventAgentViewerAttached, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
				"session_id": proc.sessionID,
				"agent_type": proc.agentType,
				"model_id":   proc.modelID,
			})
			s.signalTakeControlReady(takeControlSessionID)
			return running, nil
		}

		// Kill+resume needs adapter resume support — no hardcoded claude default left in the PTY manager.
		if !canResumeTakeControl(proc) {
			logger.Error(ctx, "take-control: resume unsupported for this session", "session_id", takeControlSessionID)
			s.rejectTakeControl(req, proc, takeControlSessionID, "resume_unsupported")
			return running, nil
		}

		logger.Info(ctx, "take-control: killing agent", "session_id", takeControlSessionID)

		// Kill process: SIGTERM → grace → SIGKILL
		proc.backend.Kill(ctx, proc, syscall.SIGTERM)
		gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
		if gracePeriod == 0 {
			gracePeriod = 5 * time.Second
		}
		select {
		case <-proc.doneCh:
		case <-time.After(gracePeriod):
			proc.backend.Kill(ctx, proc, syscall.SIGKILL)
			<-proc.doneCh
		}

		// Flush messages, register the resume launch, and register stop.
		s.saveMessages(proc)
		s.registerTakeControlResumeLaunch(proc)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "user_interactive", "take_control", proc.modelID)

		s.broadcast(ws.EventAgentTakeControl, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"session_id": proc.sessionID,
			"agent_type": proc.agentType,
			"model_id":   proc.modelID,
		})

		// Status is now user_interactive and the agent is killed — unblock
		// any HTTP caller waiting in WaitForTakeControlReady before settling into the interactive wait.
		s.signalTakeControlReady(takeControlSessionID)

		// Remove from running
		running = append(running[:i], running[i+1:]...)

		// Create interactive wait channel and block until interactive session completes
		waitCh := make(chan struct{})
		s.mu.Lock()
		s.interactiveWaits[proc.sessionID] = waitCh
		s.mu.Unlock()

		logger.Info(ctx, "take-control: waiting for interactive session to complete", "session_id", takeControlSessionID)
		select {
		case <-waitCh:
			logger.Info(ctx, "take-control: interactive session completed", "session_id", takeControlSessionID)
		case <-ctx.Done():
			logger.Warn(ctx, "take-control: cancelled while waiting for interactive session", "session_id", takeControlSessionID)
		}

		s.mu.Lock()
		delete(s.interactiveWaits, proc.sessionID)
		_, wasKilled := s.killedInteractive[proc.sessionID]
		if wasKilled {
			delete(s.killedInteractive, proc.sessionID)
		}
		s.mu.Unlock()

		if wasKilled {
			proc.finalStatus = "FAIL"
		} else {
			proc.finalStatus = "PASS"
		}
		proc.elapsed = time.Since(proc.startTime)
		return running, proc
	}

	// Session is not in our running list (already finished, or owned by a
	// different spawner). Unblock any caller waiting on the readiness channel
	// so it doesn't hang to its timeout.
	s.signalTakeControlReady(takeControlSessionID)
	return running, nil
}
