package spawner

import (
	"context"

	"be/internal/logger"
	"be/internal/repo"
)

// copyFindingsForContinuation merges findings from oldSessionID into newSessionID non-destructively
// (new-session keys win on conflict). Covers low-context, fail-restart, and tier-fallback relaunches.
// All errors are logged as warnings so they never block the relaunch.
func (s *Spawner) copyFindingsForContinuation(ctx context.Context, oldSessionID, newSessionID string) {
	pool := s.pool()
	if pool == nil {
		return
	}
	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)

	oldFindings, err := findingRepo.GetOwn("session", oldSessionID)
	if err != nil || len(oldFindings) == 0 {
		return
	}

	newFindings, err := findingRepo.GetOwn("session", newSessionID)
	if err != nil {
		logger.Warn(ctx, "findings carryover: failed to load new session findings", "new_session_id", newSessionID, "err", err)
		return
	}

	// Load new session for denorm
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	newSession, err := sessionRepo.Get(newSessionID)
	if err != nil {
		logger.Warn(ctx, "findings carryover: failed to load new session", "new_session_id", newSessionID, "err", err)
		return
	}
	modelID := ""
	if newSession.ModelID.Valid {
		modelID = newSession.ModelID.String
	}
	denorm := repo.Denorm{
		ProjectID:          newSession.ProjectID,
		WorkflowInstanceID: newSession.WorkflowInstanceID,
		AgentType:          newSession.AgentType,
		ModelID:            modelID,
	}
	actor := repo.Actor{Source: "system", ID: "continuation"}

	for k, v := range oldFindings {
		if _, exists := newFindings[k]; !exists {
			if err := findingRepo.Upsert("session", newSessionID, k, v, denorm, actor); err != nil {
				logger.Warn(ctx, "findings carryover: failed to copy key", "key", k, "err", err)
			}
		}
	}
}
