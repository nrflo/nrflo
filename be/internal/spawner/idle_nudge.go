package spawner

import (
	"context"
	"fmt"
	"strings"

	"be/internal/logger"
	"be/internal/types"
	"be/internal/ws"
)

// checkIdleNudge checks whether an interactive-CLI agent has been silent past its idle
// window and either sends a finish-reminder nudge or triggers an auto-fail when the
// nudge cap is exhausted. Only active for cliInteractiveBackend (proc.nudgeMax > 0).
// Does not cause the proc to leave the running list — the agent keeps running after a
// nudge; the auto-fail path relies on RequestTerminalSignal to drive the kill.
func (s *Spawner) checkIdleNudge(ctx context.Context, proc *processInfo, req SpawnRequest) {
	if proc.nudgeMax == 0 {
		return
	}
	if proc.backend == nil || proc.backend.Name() != "cli_interactive" {
		return
	}

	now := s.config.Clock.Now()
	proc.messagesMutex.Lock()
	sinceLastMsg := now.Sub(proc.lastMessageTime)
	hasMsg := proc.hasReceivedMessage
	proc.messagesMutex.Unlock()

	// Choose idle window based on whether the agent has produced any output yet.
	idleWindow := proc.idleAfterMessageTimeout
	if !hasMsg {
		idleWindow = proc.idleStartTimeout
	}
	if idleWindow <= 0 {
		return
	}

	if sinceLastMsg <= idleWindow {
		return
	}

	// Idle window exceeded.
	if proc.nudgeCount < proc.nudgeMax {
		s.sendNudge(ctx, proc, req)
		return
	}

	// Cap reached — wait for another full idle window since the last nudge before failing.
	if proc.lastNudgeAt.IsZero() || now.Sub(proc.lastNudgeAt) <= idleWindow {
		return
	}

	s.handleNudgeAutoFail(ctx, proc, req)
}

// sendNudge writes the finish-reminder injectable to the agent's PTY stdin and
// broadcasts agent.nudged. Treats the write as activity so the idle window resets.
func (s *Spawner) sendNudge(ctx context.Context, proc *processInfo, req SpawnRequest) {
	attempt := proc.nudgeCount + 1

	// Build standard vars for injectable expansion (mirrors template.go stdVars).
	modelPart := proc.modelID
	if idx := strings.Index(proc.modelID, ":"); idx >= 0 {
		modelPart = proc.modelID[idx+1:]
	}
	stdVars := map[string]string{
		"AGENT":      proc.agentType,
		"TICKET_ID":  proc.ticketID,
		"PROJECT_ID": proc.projectID,
		"WORKFLOW":   proc.workflowName,
		"MODEL_ID":   proc.modelID,
		"MODEL":      modelPart,
	}
	body := s.expandInjectable("finish-reminder", stdVars)

	// Write to PTY stdin (best-effort — log on error, do not abort).
	if s.config.PTYManager != nil {
		sess := s.config.PTYManager.Get(proc.sessionID)
		if sess != nil {
			if _, err := sess.Write([]byte(body + "\n")); err != nil {
				logger.Warn(ctx, "idle nudge: pty write error",
					"session_id", proc.sessionID, "attempt", attempt, "error", err)
			}
		}
	}

	// Broadcast agent.nudged event.
	s.broadcast(ws.EventAgentNudged, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"session_id": proc.sessionID,
		"agent_type": proc.agentType,
		"model_id":   proc.modelID,
		"attempt":    attempt,
		"max":        proc.nudgeMax,
	})

	// Persist the nudge count increment in DB.
	if s.config.AgentSvcReal != nil {
		newCount, err := s.config.AgentSvcReal.IncrementNudgeCount(proc.sessionID)
		if err != nil {
			logger.Warn(ctx, "idle nudge: increment nudge_count error",
				"session_id", proc.sessionID, "error", err)
		} else {
			proc.nudgeCount = newCount
		}
	} else {
		proc.nudgeCount = attempt
	}

	proc.lastNudgeAt = s.config.Clock.Now()

	// Treat nudge as agent activity so the idle window restarts from now.
	proc.messagesMutex.Lock()
	proc.lastMessageTime = s.config.Clock.Now()
	proc.hasReceivedMessage = true
	proc.messagesMutex.Unlock()

	logger.Info(ctx, "idle nudge sent",
		"session_id", proc.sessionID, "agent_type", proc.agentType,
		"attempt", attempt, "max", proc.nudgeMax)
}

// handleNudgeAutoFail marks the agent as failed with reason "unresponsive_after_nudges",
// requests a terminal kill signal, and records an error.
func (s *Spawner) handleNudgeAutoFail(ctx context.Context, proc *processInfo, req SpawnRequest) {
	logger.Warn(ctx, "idle nudge: auto-fail after cap exhausted",
		"session_id", proc.sessionID, "agent_type", proc.agentType,
		"nudge_count", proc.nudgeCount)

	reason := "unresponsive_after_nudges"

	if s.config.AgentSvcReal != nil {
		if _, err := s.config.AgentSvcReal.Fail(&types.AgentRequest{
			SessionID: proc.sessionID,
			Reason:    reason,
		}); err != nil {
			logger.Warn(ctx, "idle nudge: fail request error",
				"session_id", proc.sessionID, "error", err)
		}
	}

	s.RequestTerminalSignal(proc.sessionID, "fail")

	if s.config.ErrorSvc != nil {
		msg := fmt.Sprintf("%s: unresponsive after %d reminders", proc.agentType, proc.nudgeMax)
		if err := s.config.ErrorSvc.RecordError(proc.projectID, "agent", proc.sessionID, msg); err != nil {
			logger.Warn(ctx, "idle nudge: record error failed",
				"session_id", proc.sessionID, "error", err)
		}
	}
}
