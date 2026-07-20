package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/spawner/apirun"
)

// apiConsultant implements apirun.ConsultantSpawner for callers with no live
// spawner of their own — console sessions reached through
// console.Deps.Consultant. Mirrors apiDelegator: builds a standalone Spawner
// per call and routes to Spawner.ConsultHost, the hidden-host counterpart of
// Spawner.Consult.
type apiConsultant struct {
	o    *Orchestrator
	pool *db.Pool
}

var _ apirun.ConsultantSpawner = apiConsultant{}

// APIConsultant exposes the consult implementation for console sessions,
// mirroring APIDelegator.
func (o *Orchestrator) APIConsultant(pool *db.Pool) apirun.ConsultantSpawner {
	return apiConsultant{o: o, pool: pool}
}

func (c apiConsultant) Consult(ctx context.Context, callerSessionID, consultantID, question string) (string, error) {
	sess, err := repo.NewAgentSessionRepo(c.pool, c.o.clock).Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("consult: resolve caller session: %w", err)
	}
	sp, err := hiddenHostSpawner(ctx, c.o, c.pool, sess.ProjectID)
	if err != nil {
		return "", err
	}
	defer sp.Close()
	return sp.ConsultHost(ctx, sess.ProjectID, consultantID, question)
}
