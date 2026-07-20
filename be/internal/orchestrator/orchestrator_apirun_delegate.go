package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/spawner/apirun"
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
	return hiddenHostSpawner(ctx, d.o, d.pool, projectID)
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
