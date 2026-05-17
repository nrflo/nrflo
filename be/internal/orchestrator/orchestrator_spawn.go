package orchestrator

import (
	"context"
	"errors"

	"be/internal/service"
	"be/internal/spawner"
)

// spawnResult holds the outcome of one spawned agent.
type spawnResult struct {
	agent       string
	err         error
	callbackErr *spawner.CallbackError // non-nil when the agent requested a callback
}

// spawnPhases spawns all phases concurrently using baseCfg as the template configuration.
// For each phase, it adds per-run session register/unregister hooks to baseCfg.
// Returns all results in arbitrary order; callbackErr is set when the agent requested a callback.
func (o *Orchestrator) spawnPhases(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	phases []service.SpawnerPhaseDef,
	parentSession string,
	baseCfg spawner.Config,
) []spawnResult {
	ch := make(chan spawnResult, len(phases))
	for _, phase := range phases {
		phase := phase
		go func() {
			cfg := baseCfg
			cfg.OnSessionRegister = func(sid string, s *spawner.Spawner) {
				o.mu.Lock()
				if rs, ok := o.runs[wfiID]; ok {
					rs.spawners[sid] = s
				}
				o.mu.Unlock()
			}
			cfg.OnSessionUnregister = func(sid string) {
				o.mu.Lock()
				if rs, ok := o.runs[wfiID]; ok {
					delete(rs.spawners, sid)
				}
				o.mu.Unlock()
			}
			sp := spawner.New(cfg)
			err := sp.Spawn(ctx, spawner.SpawnRequest{
				AgentType:          phase.Agent,
				TicketID:           req.TicketID,
				ProjectID:          req.ProjectID,
				WorkflowName:       req.WorkflowName,
				ParentSession:      parentSession,
				ScopeType:          req.ScopeType,
				WorkflowInstanceID: wfiID,
			})
			sp.Close()
			sr := spawnResult{agent: phase.Agent, err: err}
			var cbErr *spawner.CallbackError
			if errors.As(err, &cbErr) {
				sr.callbackErr = cbErr
				sr.err = nil
			}
			ch <- sr
		}()
	}
	results := make([]spawnResult, 0, len(phases))
	for range phases {
		results = append(results, <-ch)
	}
	return results
}
