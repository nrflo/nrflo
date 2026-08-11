package service

import (
	"context"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/anthropic"
	"be/internal/spawner/apirun/provider/custom"
	"be/internal/spawner/apirun/provider/openai"
	"be/internal/spawner/apirun/provider/openrouter"
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

// HasAPICredentials reports whether providerName's credentials resolve for
// projectID without constructing the provider — the static half of
// BuildAPIProvider. Chain walkers use it to skip an api-mode entry that can
// only fail at build time instead of erroring on every spawn/fold.
// Custom-registry providers carry credentials in their row, so any
// non-builtin name reports true and lets the build decide.
func HasAPICredentials(ctx context.Context, pool *db.Pool, clk clock.Clock, providerName, projectID string) bool {
	envRepo := newProjectEnvAdapter(pool, clk, projectID)
	var err error
	switch providerName {
	case "anthropic":
		_, err = anthropic.ResolveAPIKey(ctx, envRepo, projectID)
	case "openai":
		_, err = openai.Resolve(ctx, envRepo, projectID)
	case "openrouter":
		_, err = openrouter.Resolve(ctx, envRepo, projectID)
	}
	return err == nil
}

// BuildAPIProvider resolves credentials and constructs a provider.Provider for
// the given providerName: a builtin ("anthropic", "openai", "openrouter") or
// the name of an enabled row in the custom_providers registry (Rule 6: the
// default case resolves the registry rather than name-checking). Shared by the
// orchestrator (autonomous api-mode agents) and the console api engine (chat
// sessions) so there is exactly one credential-resolution path. Returns an
// error if credentials are missing or the provider name is unknown/disabled.
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
	case "openrouter":
		creds, err := openrouter.Resolve(ctx, envRepo, projectID)
		if err != nil {
			return nil, err
		}
		logger.Info(ctx, "api provider configured", "project_id", projectID, "provider", providerName)
		return openrouter.New(creds), nil
	default:
		cp, err := NewCustomProviderService(pool, clk).GetEnabled(providerName)
		if err != nil {
			return nil, fmt.Errorf("unknown or disabled provider %q: %w", providerName, err)
		}
		logger.Info(ctx, "api provider configured", "project_id", projectID, "provider", providerName, "wire", cp.APIWire)
		return custom.New(custom.Config{
			Name:    cp.Name,
			BaseURL: cp.BaseURL,
			APIKey:  cp.APIKey,
			Wire:    cp.APIWire,
		}), nil
	}
}
