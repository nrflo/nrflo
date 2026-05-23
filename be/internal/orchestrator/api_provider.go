package orchestrator

import (
	"context"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/anthropic"
	"be/internal/spawner/apirun/provider/openai"
	"be/internal/ws"
)

// loadProjectEnv reads per-project env vars from DB and formats them as "KEY=value" strings.
// On error, logs a warning and returns an empty slice — must not block workflow start.
func loadProjectEnv(ctx context.Context, pool *db.Pool, projectID string, clk clock.Clock) []string {
	svc := service.NewProjectEnvVarService(pool, clk)
	vars, err := svc.List(projectID)
	if err != nil {
		logger.Warn(ctx, "failed to load project env vars, proceeding without them", "project_id", projectID, "err", err)
		return nil
	}
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, fmt.Sprintf("%s=%s", v.Name, v.Value))
	}
	return out
}

// projectEnvAdapter implements anthropic.ProjectEnvVarRepo from a pre-loaded
// per-project env var map. It ignores the projectID argument since vars are
// already scoped at construction time.
type projectEnvAdapter struct {
	vars map[string]string
}

func newProjectEnvAdapter(pool *db.Pool, clk clock.Clock, projectID string) *projectEnvAdapter {
	svc := service.NewProjectEnvVarService(pool, clk)
	vars, err := svc.List(projectID)
	if err != nil {
		return &projectEnvAdapter{vars: map[string]string{}}
	}
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		m[v.Name] = v.Value
	}
	return &projectEnvAdapter{vars: m}
}

func (a *projectEnvAdapter) Get(_ string, name string) (string, bool, error) {
	v, ok := a.vars[name]
	return v, ok, nil
}

// buildAPIProvider resolves credentials and constructs a provider.Provider for the
// given providerName ("anthropic" or "openai"). Returns an error if credentials are
// missing or the provider name is unknown — the spawner fails the spawn fast on error.
func buildAPIProvider(ctx context.Context, pool *db.Pool, clk clock.Clock, providerName, projectID string) (provider.Provider, error) {
	envRepo := newProjectEnvAdapter(pool, clk, projectID)
	switch providerName {
	case "anthropic":
		creds, err := anthropic.ResolveAPIKey(ctx, envRepo, projectID)
		if err != nil {
			return nil, err
		}
		logger.Info(ctx, "api provider configured", "project_id", projectID, "provider", providerName, "method", string(creds.Method))
		return anthropic.New(creds), nil
	case "openai":
		creds, err := openai.Resolve(ctx, envRepo, projectID)
		if err != nil {
			return nil, err
		}
		logger.Info(ctx, "api provider configured", "project_id", projectID, "provider", providerName)
		return openai.New(creds), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", providerName)
	}
}

// loadAPIModelConfigs loads enabled API model configs from the database and
// builds a map keyed by row id, suitable for spawner.Config.APIModelConfigs.
// Called once at workflow start.
func (o *Orchestrator) loadAPIModelConfigs(pool *db.Pool) (map[string]spawner.APIModelConfig, error) {
	apiModelSvc := service.NewAPIModelService(pool, o.clock)
	models, err := apiModelSvc.ListEnabled()
	if err != nil {
		return nil, fmt.Errorf("failed to load API model configs: %w", err)
	}
	configs := make(map[string]spawner.APIModelConfig, len(models))
	for _, m := range models {
		configs[m.ID] = spawner.APIModelConfig{
			Provider:        m.Provider,
			MappedModel:     m.MappedModel,
			ContextLength:   m.ContextLength,
			ReasoningEffort: m.ReasoningEffort,
		}
	}
	return configs, nil
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
