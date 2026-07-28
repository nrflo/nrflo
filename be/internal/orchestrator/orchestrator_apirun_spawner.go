package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun/provider"
)

// hiddenHostSpawner builds a standalone *spawner.Spawner from the
// orchestrator's shared services for one hidden-host call (delegate or
// consult) from a caller with no live spawner of its own — a console
// session. Mirrors runLoop's baseCfg, minus the layer-execution-only fields.
// Shared by apiDelegator.spawnerFor and apiConsultant.spawnerFor.
func hiddenHostSpawner(ctx context.Context, o *Orchestrator, pool *db.Pool, projectID string) (*spawner.Spawner, error) {
	project, err := service.NewProjectService(pool, o.clock).Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("hidden-host spawn: resolve project: %w", err)
	}
	projectRoot := ""
	if project.RootPath.Valid {
		projectRoot = project.RootPath.String
	}
	modelConfigs, err := o.loadModelConfigs(pool)
	if err != nil {
		return nil, fmt.Errorf("hidden-host spawn: load model configs: %w", err)
	}

	return spawner.New(spawner.Config{
		DataPath:     o.dataPath,
		ProjectRoot:  projectRoot,
		WSHub:        o.wsHub,
		Pool:         pool,
		Clock:        o.clock,
		ModelConfigs: modelConfigs,
		ErrorSvc:     o.errorSvc,
		BuildAPIProvider: func(ctx context.Context, providerName, providerProjectID string) (provider.Provider, error) {
			return service.BuildAPIProvider(ctx, pool, o.clock, providerName, providerProjectID)
		},
		AgentSvc:           newAPIAgentSvc(pool, o.clock, o.wsHub),
		FindingsSvc:        service.NewFindingsService(pool, o.clock),
		ProjectFindingsSvc: service.NewProjectFindingsService(pool, o.clock),
		AgentSvcReal:       service.NewAgentService(pool, o.clock),
		WorkflowSvc:        service.NewWorkflowService(pool, o.clock),
		TicketSvc:          service.NewTicketService(pool, o.clock),
		DispatchRepo:       repo.NewDispatchRepo(pool, o.clock),
		ArtifactSvc:        service.NewArtifactService(pool, o.clock, o.wsHub, o.dataPath),
		ProjectEnv:         loadProjectEnv(ctx, pool, projectID, o.clock),
		APIMode:            true,
		PTYManager:         o.PTYManager,
		// cli_interactive workers reach their nrflo tools only through the
		// socket bridge, which resolves sessions via auxSpawners; without this
		// registration a CLI-mode delegate/consult worker gets an empty
		// tools/list and no heartbeat, and stalls out while healthy.
		OnSessionRegister:   o.registerAuxSpawner,
		OnSessionUnregister: o.unregisterAuxSpawner,
	}), nil
}
