package orchestrator

import (
	"context"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/anthropic"
	"be/internal/ws"
)

// buildAPIProvider resolves the project's Anthropic API key and constructs a
// provider.Provider for API-mode agents. Returns nil if no key is configured —
// the spawner will fail any api-mode spawn at prepareSpawn time with a clear
// error, mirroring the CLI failure mode of a missing binary.
func buildAPIProvider(ctx context.Context, pool *db.Pool, projectID string, clk clock.Clock) provider.Provider {
	credRepo := repo.NewAPICredentialRepo(pool, clk)
	key, err := anthropic.ResolveAPIKey(ctx, credRepo, projectID)
	if err != nil {
		logger.Info(ctx, "anthropic api key not configured (api-mode agents will fail to spawn)", "project_id", projectID)
		return nil
	}
	return anthropic.New(key)
}

// apiAgentSvc adapts service.AgentService into apirun.AgentSvc, broadcasting
// the standard agent.context_updated WS event after the DB write so that the
// UI sees API-mode context updates without code changes.
type apiAgentSvc struct {
	svc *service.AgentService
	hub *ws.Hub
}

func newAPIAgentSvc(pool *db.Pool, clk clock.Clock, hub *ws.Hub) apirun.AgentSvc {
	return &apiAgentSvc{
		svc: service.NewAgentService(pool, clk),
		hub: hub,
	}
}

func (a *apiAgentSvc) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	projectID, ticketID, workflowName, err := a.svc.UpdateContextLeft(sessionID, pct)
	if err != nil {
		return projectID, ticketID, workflowName, err
	}
	if a.hub != nil && projectID != "" {
		a.hub.Broadcast(ws.NewEvent(ws.EventAgentContextUpdated, projectID, ticketID, workflowName, map[string]interface{}{
			"session_id":   sessionID,
			"context_left": pct,
		}))
	}
	return projectID, ticketID, workflowName, nil
}
