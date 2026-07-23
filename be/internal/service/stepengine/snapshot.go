package stepengine

import (
	"encoding/json"

	"be/internal/model"
)

// promptModeStepwise mirrors service.PromptModeStepwise's value ("stepwise").
// Duplicated as a literal rather than imported — stepengine never imports
// service (see package doc in stepengine.go).
const promptModeStepwise = "stepwise"

// decodeSteps decodes an agent_definitions.steps JSON array (already
// canonically re-marshaled and structurally validated at write time by
// service.validateStepDefinitions — this only sanity-checks the snapshot,
// never re-implements that validation). Empty or malformed JSON is
// ErrBadSnapshot.
func decodeSteps(raw []byte) ([]model.StepDefinition, error) {
	if len(raw) == 0 {
		return nil, ErrBadSnapshot
	}
	var steps []model.StepDefinition
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil, ErrBadSnapshot
	}
	if len(steps) == 0 {
		return nil, ErrBadSnapshot
	}
	return steps, nil
}

// Snapshot idempotently creates the step cursor for (instanceID, nodeID)
// from def.Steps. A relaunch/retry calling Snapshot again is a no-op —
// Insert's ON CONFLICT DO NOTHING preserves in-progress revision/index/
// completed state; Snapshot then Gets and returns the live row either way.
func (e *Engine) Snapshot(instanceID, nodeID string, def *model.AgentDefinition) (*model.AgentStepCursor, error) {
	if def == nil || def.PromptMode != promptModeStepwise || def.Steps == nil || *def.Steps == "" {
		return nil, ErrNotStepwise
	}
	if _, err := decodeSteps([]byte(*def.Steps)); err != nil {
		return nil, err
	}

	cursor := &model.AgentStepCursor{
		WorkflowInstanceID: instanceID,
		NodeID:             nodeID,
		StepsSnapshot:      *def.Steps,
		Revision:           1,
		CurrentIndex:       0,
		Completed:          "[]",
		Rejections:         "{}",
	}
	if err := e.cursorRepo.Insert(cursor); err != nil {
		return nil, err
	}
	return e.cursorRepo.Get(instanceID, nodeID)
}

// resolveWorktreeRoot returns the working-tree root for instanceID, using
// the same precedence as handoff.resolveContext (context.go:38-46):
// workflow_instances.worktree_path when non-empty, else the project's
// root_path. Best-effort: any lookup failure degrades to "".
func (e *Engine) resolveWorktreeRoot(instanceID string) string {
	wi, err := e.wfiRepo.Get(instanceID)
	if err != nil {
		return ""
	}
	if wi.WorktreePath.Valid && wi.WorktreePath.String != "" {
		return wi.WorktreePath.String
	}
	proj, err := e.projectRepo.Get(wi.ProjectID)
	if err != nil {
		return ""
	}
	if proj.RootPath.Valid {
		return proj.RootPath.String
	}
	return ""
}
