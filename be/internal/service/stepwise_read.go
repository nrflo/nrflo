package service

import (
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service/stepengine"
)

// StepProgressStep is one step's read-time progress within a
// StepCursorProgress, derived from the cursor's snapshot + completed +
// rejections against the live current_index.
type StepProgressStep struct {
	StepID          string   `json:"step_id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"` // "pending" | "active" | "rejected_retrying" | "done"
	CompletedAt     string   `json:"completed_at,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	EvidenceKeys    []string `json:"evidence_keys,omitempty"`
	Rejections      int      `json:"rejections,omitempty"`
	Rotated         bool     `json:"rotated,omitempty"`
	RotationAllowed bool     `json:"rotation_allowed,omitempty"`
}

// StepCursorProgress is the per-node read model for one agent_step_cursors
// row, surfaced on the v4 workflow state under "step_cursors" and on
// GET /api/v1/workflow-instances/{iid}/steps.
type StepCursorProgress struct {
	NodeID        string             `json:"node_id"`
	Revision      int                `json:"revision"`
	CurrentIndex  int                `json:"current_index"`
	Total         int                `json:"total"`
	CurrentStepID string             `json:"current_step_id,omitempty"`
	Done          bool               `json:"done"`
	UpdatedAt     string             `json:"updated_at"`
	Steps         []StepProgressStep `json:"steps"`
}

// BuildStepCursors builds the per-node stepwise progress read model for a
// workflow instance. Returns an empty map (never nil error) for non-stepwise
// instances — callers gate on len(...) > 0 before adding it to a payload.
func (s *WorkflowService) BuildStepCursors(instanceID string) map[string]*StepCursorProgress {
	cursors, err := repo.NewAgentStepCursorRepo(s.pool, s.clock).ListByInstance(instanceID)
	if err != nil || len(cursors) == 0 {
		return nil
	}

	result := make(map[string]*StepCursorProgress, len(cursors))
	for _, c := range cursors {
		st, err := stepengine.DecodeCursor(c)
		if err != nil {
			continue
		}
		result[c.NodeID] = buildStepCursorProgress(c, st)
	}
	return result
}

func buildStepCursorProgress(c *model.AgentStepCursor, st *stepengine.State) *StepCursorProgress {
	completedByStep := make(map[string]model.CompletedStep, len(st.Completed))
	for _, cs := range st.Completed {
		completedByStep[cs.StepID] = cs
	}

	total := len(st.Steps)
	progress := &StepCursorProgress{
		NodeID:       c.NodeID,
		Revision:     st.Revision,
		CurrentIndex: st.CurrentIndex,
		Total:        total,
		Done:         st.CurrentIndex >= total,
		UpdatedAt:    c.UpdatedAt.Format(time.RFC3339Nano),
		Steps:        make([]StepProgressStep, 0, total),
	}
	if !progress.Done && st.CurrentIndex < total {
		progress.CurrentStepID = st.Steps[st.CurrentIndex].StepID
	}

	for i, def := range st.Steps {
		sp := StepProgressStep{
			StepID:          def.StepID,
			Title:           def.Title,
			RotationAllowed: def.RotationAllowed,
		}
		switch {
		case i < st.CurrentIndex:
			sp.Status = "done"
		case i == st.CurrentIndex:
			if st.Rejections[def.StepID] > 0 {
				sp.Status = "rejected_retrying"
			} else {
				sp.Status = "active"
			}
		default:
			sp.Status = "pending"
		}
		sp.Rejections = st.Rejections[def.StepID]
		if cs, ok := completedByStep[def.StepID]; ok {
			sp.CompletedAt = cs.CompletedAt
			sp.SessionID = cs.SessionID
			sp.Summary = cs.Summary
			sp.EvidenceKeys = cs.EvidenceKeys
			sp.Rotated = cs.Rotated
		}
		progress.Steps = append(progress.Steps, sp)
	}
	return progress
}
