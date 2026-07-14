package service

import (
	"context"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/anthropic"
	"be/internal/spawner/apirun/provider/openai"
)

// projectEnvAdapter implements anthropic.ProjectEnvVarRepo/openai.ProjectEnvVarRepo
// from a pre-loaded per-project env var map. It ignores the projectID argument
// since vars are already scoped at construction time.
type projectEnvAdapter struct {
	vars map[string]string
}

func newProjectEnvAdapter(pool *db.Pool, clk clock.Clock, projectID string) *projectEnvAdapter {
	svc := NewProjectEnvVarService(pool, clk)
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

// BuildAPIProvider resolves credentials and constructs a provider.Provider for
// the given providerName ("anthropic" or "openai"). Shared by the
// orchestrator (autonomous api-mode agents) and the console api engine (chat
// sessions) so there is exactly one credential-resolution path. Returns an
// error if credentials are missing or the provider name is unknown.
func BuildAPIProvider(ctx context.Context, pool *db.Pool, clk clock.Clock, providerName, projectID string) (provider.Provider, error) {
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
