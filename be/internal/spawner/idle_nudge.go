package spawner

import (
	"context"
	"fmt"
	"strings"

	"be/internal/logger"
	"be/internal/types"
	"be/internal/ws"
)

// checkIdleNudge runs the idle-time checks for an interactive-CLI agent. It
// first routes a swallowed server-side API error to a rate-limit relaunch
// (handleInBandRateLimit) — this runs for every cli_interactive agent including
// the nudge-less api-via-cli lane and, when it fires, kills the proc with
// finalStatus=CONTINUE for the monitor to relaunch. Otherwise, when the agent
// has been silent past its idle window (proc.nudgeMax > 0), it sends a
// finish-reminder nudge or triggers an auto-fail once the nudge cap is spent.
// The nudge path itself does not remove the proc from the running list; its
// auto-fail relies on RequestTerminalSignal to drive the kill.
func (s *Spawner) checkIdleNudge(ctx context.Context, proc *processInfo, req SpawnRequest) {
	if proc.backend == nil || proc.backend.Name() != "cli_interactive" {
		return
	}

	// Before nudging, catch a turn that ended on a swallowed server-side API
	// error (e.g. 529 Overloaded): no Stop hook fires and nudging cannot revive
	// it, so relaunch with rate-limit backoff instead. Runs for every
	// interactive agent, including the nudge-less api-via-cli lane.
	if s.handleInBandRateLimit(ctx, proc, req) {
		return
	}

	if proc.nudgeMax == 0 {
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

// dispatchNudgeRequest finds the running proc matching an in-band nudge
// request (drained from nudgeRequestCh by monitorAll) and fires
// triggerImmediateNudge for it. No-op if the session already finished.
func (s *Spawner) dispatchNudgeRequest(ctx context.Context, running []*processInfo, req SpawnRequest, nr nudgeRequest) {
	for _, proc := range running {
		if proc.sessionID == nr.sessionID {
			s.triggerImmediateNudge(ctx, proc, req, nr.reason)
			return
		}
	}
}

// triggerImmediateNudge fires the same sendNudge/handleNudgeAutoFail tail as
// checkIdleNudge's idle-window-exceeded branch, but on-demand — invoked from
// monitorAll's nudgeRequestCh drain when the socket handler classifies a
// Claude Notification hook as idle-waiting or permission-prompt. Guards
// mirror checkIdleNudge's scoping: only cli_interactive backends with
// nudging enabled (nudgeMax>0) are eligible, which structurally excludes
// codex (backend name "codex") and the nudge-less api-via-cli lane.
// Deliberately does NOT call handleInBandRateLimit or any wall-clock check —
// those stay exclusively in checkIdleNudge.
func (s *Spawner) triggerImmediateNudge(ctx context.Context, proc *processInfo, req SpawnRequest, reason string) {
	if proc.backend == nil || proc.backend.Name() != "cli_interactive" || proc.nudgeMax == 0 {
		return
	}

	logger.Info(ctx, "idle nudge: in-band notification signal",
		"session_id", proc.sessionID, "agent_type", proc.agentType, "reason", reason)

	if proc.nudgeCount < proc.nudgeMax {
		s.sendNudge(ctx, proc, req)
		return
	}

	s.handleNudgeAutoFail(ctx, proc, req)
}

// sendNudge writes the finish-reminder injectable to the agent's PTY stdin and
// records the nudge. Treats the write as activity so the idle window resets.
func (s *Spawner) sendNudge(ctx context.Context, proc *processInfo, req SpawnRequest) {
	body := s.nudgeBody(proc)

	// Write to PTY stdin (best-effort — log on error, do not abort).
	if s.config.PTYManager != nil {
		sess := s.config.PTYManager.Get(proc.sessionID)
		if sess != nil {
			if _, err := sess.Write([]byte(body + "\n")); err != nil {
				logger.Warn(ctx, "idle nudge: pty write error",
					"session_id", proc.sessionID, "attempt", proc.nudgeCount+1, "error", err)
			}
		}
	}

	s.recordNudgeSent(ctx, proc, req)
}

// nudgeBody expands the finish-reminder injectable for a proc (mirrors
// template.go stdVars). Shared by the PTY and app-server nudge paths.
func (s *Spawner) nudgeBody(proc *processInfo) string {
	modelPart := proc.modelID
	if idx := strings.Index(proc.modelID, ":"); idx >= 0 {
		modelPart = proc.modelID[idx+1:]
	}
	return s.expandInjectable("finish-reminder", map[string]string{
		"AGENT":      proc.agentType,
		"TICKET_ID":  proc.ticketID,
		"PROJECT_ID": proc.projectID,
		"WORKFLOW":   proc.workflowName,
		"MODEL_ID":   proc.modelID,
		"MODEL":      modelPart,
	})
}

// recordNudgeSent broadcasts agent.nudged, persists the nudge_count increment,
// and resets the idle window. Shared by the PTY (sendNudge) and app-server
// nudge paths — the delivery of the reminder is the caller's responsibility.
func (s *Spawner) recordNudgeSent(ctx context.Context, proc *processInfo, req SpawnRequest) {
	attempt := proc.nudgeCount + 1

	s.broadcast(ws.EventAgentNudged, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"session_id": proc.sessionID,
		"agent_type": proc.agentType,
		"model_id":   proc.modelID,
		"attempt":    attempt,
		"max":        proc.nudgeMax,
	})

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
func (s *Spawner) handleNudgeAutoFail(ctx context.Context, proc *processInfo, _ SpawnRequest) {
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
