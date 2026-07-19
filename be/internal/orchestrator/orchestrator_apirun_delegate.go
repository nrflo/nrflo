package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// apiDelegator implements apirun.Delegator for callers with no live spawner
// of their own — console sessions reached through console.Deps.Delegator. It
// builds a standalone Spawner from the orchestrator's shared services per
// call (mirrors runLoop's baseCfg, minus the layer-execution-only fields) so
// Spawner.Delegate/GetDelegation can spawn API-mode _t1_executor/
// _t2_extractor workers exactly like an in-run agent's delegate call.
type apiDelegator struct {
	o    *Orchestrator
	pool *db.Pool
}

var _ apirun.Delegator = apiDelegator{}

// APIDelegator exposes the delegate implementation for console sessions,
// mirroring APIWorkflowControl.
func (o *Orchestrator) APIDelegator(pool *db.Pool) apirun.Delegator {
	return apiDelegator{o: o, pool: pool}
}

func (d apiDelegator) spawnerFor(ctx context.Context, projectID string) (*spawner.Spawner, error) {
	project, err := service.NewProjectService(d.pool, d.o.clock).Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("delegate: resolve project: %w", err)
	}
	projectRoot := ""
	if project.RootPath.Valid {
		projectRoot = project.RootPath.String
	}
	modelConfigs, err := d.o.loadModelConfigs(d.pool)
	if err != nil {
		return nil, fmt.Errorf("delegate: load model configs: %w", err)
	}

	return spawner.New(spawner.Config{
		DataPath:     d.o.dataPath,
		ProjectRoot:  projectRoot,
		WSHub:        d.o.wsHub,
		Pool:         d.pool,
		Clock:        d.o.clock,
		ModelConfigs: modelConfigs,
		ErrorSvc:     d.o.errorSvc,
		BuildAPIProvider: func(ctx context.Context, providerName, providerProjectID string) (provider.Provider, error) {
			return service.BuildAPIProvider(ctx, d.pool, d.o.clock, providerName, providerProjectID)
		},
		AgentSvc:           newAPIAgentSvc(d.pool, d.o.clock, d.o.wsHub),
		FindingsSvc:        service.NewFindingsService(d.pool, d.o.clock),
		ProjectFindingsSvc: service.NewProjectFindingsService(d.pool, d.o.clock),
		AgentSvcReal:       service.NewAgentService(d.pool, d.o.clock),
		WorkflowSvc:        service.NewWorkflowService(d.pool, d.o.clock),
		TicketSvc:          service.NewTicketService(d.pool, d.o.clock),
		DispatchRepo:       repo.NewDispatchRepo(d.pool, d.o.clock),
		ArtifactSvc:        service.NewArtifactService(d.pool, d.o.clock, d.o.wsHub, d.o.dataPath),
		ProjectEnv:         loadProjectEnv(ctx, d.pool, projectID, d.o.clock),
		APIMode:            true,
	}), nil
}

func (d apiDelegator) Delegate(ctx context.Context, callerSessionID string, req apirun.DelegateRequest) (string, error) {
	sess, err := repo.NewAgentSessionRepo(d.pool, d.o.clock).Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve caller session: %w", err)
	}
	sp, err := d.spawnerFor(ctx, sess.ProjectID)
	if err != nil {
		return "", err
	}
	defer sp.Close()
	return sp.Delegate(ctx, callerSessionID, req)
}

func (d apiDelegator) GetDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error) {
	sess, err := repo.NewAgentSessionRepo(d.pool, d.o.clock).Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve caller session: %w", err)
	}
	sp, err := d.spawnerFor(ctx, sess.ProjectID)
	if err != nil {
		return "", err
	}
	defer sp.Close()
	return sp.GetDelegation(ctx, callerSessionID, delegationID)
}
