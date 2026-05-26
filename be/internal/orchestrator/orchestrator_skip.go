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
	"be/internal/ws"
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

// partitionLayerSkips splits a layer's phases into skipped and runnable sets based
// on the workflow instance's skip_tags (per-agent, not all-or-nothing). Reloads
// skip_tags from DB each call (agents in a running layer may add tags concurrently).
// tagOf maps each skipped agent to the skip tag that matched. On DB error or when no
// skip_tags are set, every phase is runnable.
func (o *Orchestrator) partitionLayerSkips(ctx context.Context, wfiID string, phases []service.SpawnerPhaseDef, agentTags map[string]string) (skipped, runnable []service.SpawnerPhaseDef, tagOf map[string]string) {
	tagOf = make(map[string]string)

	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB for skip check", "err", err)
		return nil, phases, tagOf
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		logger.Error(ctx, "failed to load WFI for skip check", "err", err)
		return nil, phases, tagOf
	}

	skipTags := wi.GetSkipTags()
	if len(skipTags) == 0 {
		return nil, phases, tagOf
	}
	skipSet := make(map[string]bool, len(skipTags))
	for _, t := range skipTags {
		skipSet[t] = true
	}

	for _, phase := range phases {
		if tag := agentTags[phase.Agent]; tag != "" && skipSet[tag] {
			skipped = append(skipped, phase)
			tagOf[phase.Agent] = tag
		} else {
			runnable = append(runnable, phase)
		}
	}
	return skipped, runnable, tagOf
}

// applyLayerSkips partitions a layer by skip_tags, persists skipped sessions, and
// broadcasts a per-agent skipped completion for each skipped agent. It returns the
// runnable subset to spawn. wholeLayerSkipped is true only when every agent in the
// layer was skipped — in that case EventLayerSkipped is also broadcast and the caller
// should advance past the layer without spawning. A partial skip returns the
// remaining runnable agents with wholeLayerSkipped=false.
func (o *Orchestrator) applyLayerSkips(ctx context.Context, wfiID string, req RunRequest, phases []service.SpawnerPhaseDef, agentTags map[string]string, pool *db.Pool) (runnable []service.SpawnerPhaseDef, wholeLayerSkipped bool) {
	skipped, runnable, tagOf := o.partitionLayerSkips(ctx, wfiID, phases, agentTags)
	if len(skipped) == 0 {
		return runnable, false
	}

	o.createSkippedSessions(ctx, wfiID, req, skipped, pool)
	skippedNames := make([]string, len(skipped))
	for i, phase := range skipped {
		skippedNames[i] = phase.Agent
		o.wsHub.Broadcast(ws.NewEvent(ws.EventAgentCompleted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"agent_id":   phase.Agent,
			"agent_type": phase.Agent,
			"result":     "skipped",
			"skip_tag":   tagOf[phase.Agent],
		}))
	}

	if len(runnable) > 0 {
		logger.Info(ctx, "partial layer skip due to tag", "skipped", skippedNames, "runnable", len(runnable))
		return runnable, false
	}

	logger.Info(ctx, "layer skipped due to tag", "layer", phases[0].Layer, "agents", skippedNames)
	o.wsHub.Broadcast(ws.NewEvent(ws.EventLayerSkipped, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"layer":       phases[0].Layer,
		"skip_tag":    tagOf[skipped[0].Agent],
		"agents":      skippedNames,
	}))
	return nil, true
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
