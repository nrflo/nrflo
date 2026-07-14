package console

import (
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner"
)

// chatModelResolver resolves a chat session's model id into spec.Model plus
// any engine-specific extras (reasoning effort, context length, provider).
// cli_models and api_models are separate tables whose ids collide (both seed
// "sonnet"/"haiku"), so resolution must diverge by engine — this is the one
// legitimate switch beyond spawner.GetConsoleEngine (modelResolverFor below),
// the same factory shape as GetConsoleEngine/console.GetDriver.
type chatModelResolver interface {
	Resolve(pool *db.Pool, clk clock.Clock, spec *spawner.EngineSpec, modelID string) error
}

// modelResolverFor is the one place a console-chat engine name selects a
// model-resolution strategy.
func modelResolverFor(engine string) chatModelResolver {
	if engine == "api" {
		return apiModelResolver{}
	}
	return cliModelResolver{engine: engine}
}

// cliModelResolver mirrors buildChatEngineSpec's original cli_models lookup:
// an id absent from the registry passes through raw — still a legal CLI
// model name, mirroring cli/console_client.go's resolveCLIModel. A row that
// exists but belongs to another engine, or is disabled, is an error.
type cliModelResolver struct{ engine string }

func (r cliModelResolver) Resolve(pool *db.Pool, clk clock.Clock, spec *spawner.EngineSpec, modelID string) error {
	row, err := service.NewCLIModelService(pool, clk).Get(modelID)
	if err != nil {
		// Unknown id: keep spec.Model as the raw value, no effort/fallback.
		return nil
	}
	if row.CLIType != r.engine {
		return fmt.Errorf("model %q is registered for cli %s, not %s", modelID, row.CLIType, r.engine)
	}
	if !row.Enabled {
		return fmt.Errorf("model %q is disabled in the cli_models registry", modelID)
	}
	spec.Model = row.MappedModel
	spec.ReasoningEffort = row.ReasoningEffort
	spec.FallbackModels = row.FallbackModels
	spec.MaxContext = row.ContextLength
	return nil
}

// apiModelResolver resolves against api_models — a distinct id namespace
// from cli_models that happens to collide on some ids (e.g. "sonnet"). Unlike
// the CLI resolver, an unknown/disabled id is always an error: a direct-API
// call cannot pass a raw model name through without a resolved provider.
type apiModelResolver struct{}

func (apiModelResolver) Resolve(pool *db.Pool, clk clock.Clock, spec *spawner.EngineSpec, modelID string) error {
	row, err := service.NewAPIModelService(pool, clk).Get(modelID)
	if err != nil {
		return fmt.Errorf("model %q not found in api_models registry", modelID)
	}
	if !row.Enabled {
		return fmt.Errorf("model %q is disabled in the api_models registry", modelID)
	}
	spec.Model = row.MappedModel
	spec.APIProvider = row.Provider
	spec.ReasoningEffort = row.ReasoningEffort
	spec.MaxContext = row.ContextLength
	return nil
}
