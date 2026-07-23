package spawner

import (
	"context"
	"encoding/json"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/service/stepengine"
)

// stepwiseCompletionGuard reports whether proc is a stepwise agent whose
// server-owned cursor is still short of its last step, despite the agent
// signaling result=pass. Degrades silently (returns false) on anything that
// prevents judging — no pool, non-stepwise def, or a cursor/State error —
// mirroring snapshotStepCursor's posture: a bookkeeping failure must never
// invent a phase failure.
func (s *Spawner) stepwiseCompletionGuard(ctx context.Context, proc *processInfo, req SpawnRequest) bool {
	if !s.stepwiseDefFor(proc.agentType, req.ProjectID, req.WorkflowName) {
		return false
	}
	pool := s.pool()
	if pool == nil {
		return false
	}
	engine := stepengine.New(pool, s.config.Clock, nil)
	state, err := engine.State(proc.workflowInstanceID, proc.nodeID)
	if err != nil {
		logger.Warn(ctx, "stepwise: completion guard could not read cursor, allowing pass", "error", err, "wfi_id", proc.workflowInstanceID, "node_id", proc.nodeID)
		return false
	}
	if state.CurrentIndex >= len(state.Steps) {
		return false
	}
	s.writeStepsIncompleteFinding(proc, state)
	return true
}

// writeStepsIncompleteFinding persists a steps_incomplete finding on the
// session scope, mirroring writeValidationFailureFinding's shape exactly.
func (s *Spawner) writeStepsIncompleteFinding(proc *processInfo, state *stepengine.State) {
	pool := s.pool()
	if pool == nil {
		return
	}

	completedIDs := make([]string, 0, len(state.Completed))
	for _, c := range state.Completed {
		completedIDs = append(completedIDs, c.StepID)
	}
	pendingTitles := make([]string, 0, len(state.Steps)-state.CurrentIndex)
	for _, step := range state.Steps[state.CurrentIndex:] {
		pendingTitles = append(pendingTitles, step.Title)
	}

	payload, marshalErr := json.Marshal(map[string]interface{}{
		"step_id":            state.Steps[state.CurrentIndex].StepID,
		"step_index":         state.CurrentIndex + 1,
		"total":              len(state.Steps),
		"completed_step_ids": completedIDs,
		"pending_titles":     pendingTitles,
	})
	if marshalErr != nil {
		logger.Warn(context.Background(), "stepwise: failed to marshal steps_incomplete finding", "session", proc.sessionID, "err", marshalErr)
		return
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	denorm := repo.Denorm{
		ProjectID:          proc.projectID,
		WorkflowInstanceID: proc.workflowInstanceID,
		AgentType:          proc.agentType,
		ModelID:            proc.modelID,
	}
	actor := repo.Actor{Source: "system", ID: "stepwise"}

	if upsertErr := findingRepo.Upsert("session", proc.sessionID, service.FindingKeyStepsIncomplete, json.RawMessage(payload), denorm, actor); upsertErr != nil {
		logger.Warn(context.Background(), "stepwise: failed to write steps_incomplete finding", "session", proc.sessionID, "err", upsertErr)
	}
}
