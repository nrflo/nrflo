package orchestrator

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// buildAgentTags creates a map of agent_id → tag from the spawner agent configs.
func buildAgentTags(svcAgents map[string]service.SpawnerAgentConfig) map[string]string {
	tags := make(map[string]string, len(svcAgents))
	for id, cfg := range svcAgents {
		if cfg.Tag != "" {
			tags[id] = cfg.Tag
		}
	}
	return tags
}

// shouldSkipLayer checks if a layer should be skipped based on workflow instance skip_tags.
// Reloads skip_tags from DB each call (agents in a running layer may add tags concurrently).
// Returns (shouldSkip, matchingTag).
func (o *Orchestrator) shouldSkipLayer(ctx context.Context, wfiID string, phases []service.SpawnerPhaseDef, agentTags map[string]string) (bool, string) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB for skip check", "err", err)
		return false, ""
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		logger.Error(ctx, "failed to load WFI for skip check", "err", err)
		return false, ""
	}

	skipTags := wi.GetSkipTags()
	if len(skipTags) == 0 {
		return false, ""
	}

	skipSet := make(map[string]bool, len(skipTags))
	for _, t := range skipTags {
		skipSet[t] = true
	}

	for _, phase := range phases {
		tag := agentTags[phase.Agent]
		if tag != "" && skipSet[tag] {
			return true, tag
		}
	}

	return false, ""
}

// createSkippedSessions creates agent_sessions with status=skipped for each agent in a skipped layer.
func (o *Orchestrator) createSkippedSessions(ctx context.Context, wfiID string, req RunRequest, phases []service.SpawnerPhaseDef, pool *db.Pool) {
	sessionRepo := repo.NewAgentSessionRepo(pool, o.clock)
	now := o.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")

	for _, phase := range phases {
		session := &model.AgentSession{
			ID:                 uuid.New().String(),
			ProjectID:          req.ProjectID,
			TicketID:           req.TicketID,
			WorkflowInstanceID: wfiID,
			Phase:              phase.Agent,
			AgentType:          phase.Agent,
			Status:             model.AgentSessionSkipped,
			Result:             sql.NullString{String: "skipped", Valid: true},
			StartedAt:          sql.NullString{String: now, Valid: true},
			EndedAt:            sql.NullString{String: now, Valid: true},
		}
		if err := sessionRepo.Create(session); err != nil {
			logger.Error(ctx, "failed to create skipped session", "agent", phase.Agent, "err", err)
		}
	}
}
