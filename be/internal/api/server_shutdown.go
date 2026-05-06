package api

import (
	"context"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/ws"
)

// shutdownCleanup marks all in-flight rows as failed/canceled after the orchestrator
// has cancelled all run contexts. Passes run in fixed order:
//
//  1. agent_sessions — bearer tokens auto-invalidate once status flips
//  2. workflow_instances — ticket reopen for ticket-scope rows + EventOrchestrationFailed
//  3. wfChainRunner (workflow_chain_runs + steps)
//  4. chainRunner (chain_executions + items + locks)
//  5. schedule_runs
func (s *Server) shutdownCleanup(ctx context.Context) {
	s.sweepAgentSessions(ctx)
	s.sweepWorkflowInstances(ctx)
	if s.wfChainRunner != nil {
		s.wfChainRunner.FailAllRunning()
	}
	if s.chainRunner != nil {
		s.chainRunner.FailAllRunning()
	}
	s.sweepScheduleRuns(ctx)
}

func (s *Server) sweepAgentSessions(ctx context.Context) {
	sessionRepo := s.agentSessionRepo()
	n, err := sessionRepo.FailAllRunning()
	if err != nil {
		logger.Error(ctx, "shutdown: failed to sweep agent_sessions", "err", err)
		return
	}
	if n > 0 {
		logger.Info(ctx, "shutdown: marked agent sessions failed", "count", n)
	}
}

func (s *Server) sweepWorkflowInstances(ctx context.Context) {
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	instances, err := wfiRepo.ListActive()
	if err != nil {
		logger.Error(ctx, "shutdown: failed to list active workflow instances", "err", err)
		return
	}
	ticketSvc := s.ticketService()
	count := 0
	for _, wi := range instances {
		n, err := wfiRepo.FailIfActive(wi.ID)
		if err != nil {
			logger.Error(ctx, "shutdown: failed to fail workflow instance", "instance_id", wi.ID, "err", err)
			continue
		}
		if n == 0 {
			continue
		}
		count++
		if wi.ScopeType == "ticket" && wi.TicketID != "" {
			if err := ticketSvc.Reopen(wi.ProjectID, wi.TicketID); err != nil {
				logger.Warn(ctx, "shutdown: failed to reopen ticket", "ticket_id", wi.TicketID, "err", err)
			}
		}
		s.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationFailed, wi.ProjectID, wi.TicketID, "", map[string]interface{}{
			"instance_id": wi.ID,
			"reason":      "server_shutdown",
		}))
	}
	if count > 0 {
		logger.Info(ctx, "shutdown: marked workflow instances failed", "count", count)
	}
}

func (s *Server) sweepScheduleRuns(ctx context.Context) {
	srRepo := repo.NewScheduleRunRepo(s.pool, s.clock)
	n, err := srRepo.FailRunning("server_shutdown")
	if err != nil {
		logger.Error(ctx, "shutdown: failed to sweep schedule_runs", "err", err)
		return
	}
	if n > 0 {
		logger.Info(ctx, "shutdown: marked schedule runs failed", "count", n)
	}
}
