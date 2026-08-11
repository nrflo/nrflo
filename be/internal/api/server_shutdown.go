package api

import (
	"context"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/ws"
)

// shutdownCleanup marks all in-flight rows as failed/canceled after the orchestrator
// has cancelled all run contexts. Console-chat engines are stopped first
// (ChatService.StopAll) so no engine process (PTY child, app-server child)
// outlives the server once its row is marked failed; then the shared
// sweepInFlight passes run.
func (s *Server) shutdownCleanup(ctx context.Context) {
	if s.consoleChat != nil {
		s.consoleChat.StopAll()
	}
	s.sweepInFlight(ctx, "server_shutdown")
}

// startupOrphanSweep recovers rows a previous process left in-flight (crash,
// kill -9, power loss — the graceful path already swept them at shutdown).
// Runs from Start() before any listener or engine exists, when nothing can
// genuinely be running, so failing every 'running'/'user_interactive' row is
// safe by construction.
func (s *Server) startupOrphanSweep(ctx context.Context) {
	s.sweepInFlight(ctx, "startup_orphan_sweep")
}

// sweepInFlight runs the fixed-order cleanup passes shared by graceful
// shutdown and the startup orphan sweep:
//
//  1. agent_sessions — bearer tokens auto-invalidate once status flips
//  2. workflow_instances — ticket reopen for ticket-scope rows + EventOrchestrationFailed
//  3. wfChainRunner (workflow_chain_runs + steps)
//  4. chainRunner (chain_executions + items + locks)
//  5. schedule_runs
func (s *Server) sweepInFlight(ctx context.Context, reason string) {
	s.sweepAgentSessions(ctx, reason)
	s.sweepWorkflowInstances(ctx, reason)
	if s.wfChainRunner != nil {
		s.wfChainRunner.FailAllRunning()
	}
	if s.chainRunner != nil {
		s.chainRunner.FailAllRunning()
	}
	s.sweepScheduleRuns(ctx, reason)
}

func (s *Server) sweepAgentSessions(ctx context.Context, reason string) {
	sessionRepo := s.agentSessionRepo()
	n, err := sessionRepo.FailAllRunning(reason)
	if err != nil {
		logger.Error(ctx, "sweep: failed to sweep agent_sessions", "reason", reason, "err", err)
		return
	}
	if n > 0 {
		logger.Info(ctx, "sweep: marked agent sessions failed", "reason", reason, "count", n)
	}
}

func (s *Server) sweepWorkflowInstances(ctx context.Context, reason string) {
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	instances, err := wfiRepo.ListActive()
	if err != nil {
		logger.Error(ctx, "sweep: failed to list active workflow instances", "reason", reason, "err", err)
		return
	}
	ticketSvc := s.ticketService()
	count := 0
	for _, wi := range instances {
		n, err := wfiRepo.FailIfActive(wi.ID)
		if err != nil {
			logger.Error(ctx, "sweep: failed to fail workflow instance", "instance_id", wi.ID, "err", err)
			continue
		}
		if n == 0 {
			continue
		}
		count++
		if wi.ScopeType == "ticket" && wi.TicketID != "" {
			if err := ticketSvc.Reopen(wi.ProjectID, wi.TicketID); err != nil {
				logger.Warn(ctx, "sweep: failed to reopen ticket", "ticket_id", wi.TicketID, "err", err)
			}
		}
		s.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationFailed, wi.ProjectID, wi.TicketID, "", map[string]interface{}{
			"instance_id": wi.ID,
			"reason":      reason,
		}))
	}
	if count > 0 {
		logger.Info(ctx, "sweep: marked workflow instances failed", "reason", reason, "count", count)
	}
}

func (s *Server) sweepScheduleRuns(ctx context.Context, reason string) {
	srRepo := repo.NewScheduleRunRepo(s.pool, s.clock)
	n, err := srRepo.FailRunning(reason)
	if err != nil {
		logger.Error(ctx, "sweep: failed to sweep schedule_runs", "reason", reason, "err", err)
		return
	}
	if n > 0 {
		logger.Info(ctx, "sweep: marked schedule runs failed", "reason", reason, "count", n)
	}
}
