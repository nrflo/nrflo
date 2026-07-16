package console

import (
	"fmt"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner"
)

// chatModelResolver resolves a chat session's model id into spec.Model plus
// any engine-specific extras (reasoning effort, context length, provider).
// effort is an optional create-time override; when the model resolves to a
// registry row it must be allowed by the selected mode's effort list.
type chatModelResolver interface {
	Resolve(pool *db.Pool, clk clock.Clock, spec *spawner.EngineSpec, modelID, effort string) error
}

// modelResolverFor is the one place a console-chat engine name selects a
// model-resolution strategy.
func modelResolverFor(engine string) chatModelResolver {
	if engine == "api" {
		return apiModelResolver{}
	}
	return cliModelResolver{engine: engine}
}

// cliModelResolver permits raw CLI model names but validates registered rows
// against the provider-derived engine and CLI-mode configuration.
type cliModelResolver struct{ engine string }

func (r cliModelResolver) Resolve(pool *db.Pool, clk clock.Clock, spec *spawner.EngineSpec, modelID, effort string) error {
	row, err := service.NewModelService(pool, clk).Get(modelID)
	if err != nil {
		if !strings.HasPrefix(err.Error(), "model not found:") {
			return err
		}
		spec.Model = modelID
		spec.ReasoningEffort = effort
		return nil
	}
	registeredEngine := cliEngineForProvider(row.Provider)
	if registeredEngine != r.engine {
		return fmt.Errorf("model %q uses provider %s (%s CLI), not %s", modelID, row.Provider, registeredEngine, r.engine)
	}
	if !row.Enabled {
		return fmt.Errorf("model %q is disabled in the models registry", modelID)
	}
	if row.CLIModel == "" {
		return fmt.Errorf("model %q does not support CLI mode", modelID)
	}
	if err := service.ValidateEffortAllowed(effort, row.CLIEfforts); err != nil {
		return err
	}
	spec.Model = row.CLIModel
	spec.ReasoningEffort = row.DefaultEffort
	if effort != "" {
		spec.ReasoningEffort = effort
	}
	spec.FallbackModels = row.FallbackModels
	spec.MaxContext = row.CLIContext
	return nil
}

// apiModelResolver requires an enabled row with direct-API support.
type apiModelResolver struct{}

func (apiModelResolver) Resolve(pool *db.Pool, clk clock.Clock, spec *spawner.EngineSpec, modelID, effort string) error {
	row, err := service.NewModelService(pool, clk).Get(modelID)
	if err != nil {
		return fmt.Errorf("model %q not found in models registry", modelID)
	}
	if !row.Enabled {
		return fmt.Errorf("model %q is disabled in the models registry", modelID)
	}
	if row.APIModel == "" {
		return fmt.Errorf("model %q does not support direct API mode", modelID)
	}
	if err := service.ValidateEffortAllowed(effort, row.APIEfforts); err != nil {
		return err
	}
	spec.Model = row.APIModel
	spec.APIProvider = row.Provider
	spec.ReasoningEffort = row.DefaultEffort
	if effort != "" {
		spec.ReasoningEffort = effort
	}
	spec.MaxContext = row.APIContext
	return nil
}
