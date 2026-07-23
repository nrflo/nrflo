package tools_builtin

import (
	"encoding/json"
	"fmt"

	"be/internal/service"
	"be/internal/service/stepengine"
	"be/internal/spawner/apirun"
)

// renderNext stamps the task-boundary signal (a step just landed) and
// returns the newly-current step: step_id/revision MUST be the NEW values —
// the stepwise-guidance injectable tells the agent to use exactly what it is
// shown here for its next complete_step call.
func renderNext(env apirun.ToolEnv, engine *stepengine.Engine, outcome stepengine.Outcome) (string, bool, error) {
	if env.Steps != nil {
		env.Steps.NoteStepBoundary(env.SessionID)
	}
	total := outcome.CurrentIndex + 1
	if state, err := engine.State(env.WorkflowInstanceID, env.NodeID); err == nil {
		total = len(state.Steps)
	}
	payload := map[string]interface{}{
		"step_id":     outcome.NextStep.StepID,
		"revision":    outcome.Revision,
		"step_index":  outcome.CurrentIndex + 1,
		"total":       total,
		"title":       outcome.NextStep.Title,
		"instruction": outcome.NextStep.Instruction,
	}
	if len(outcome.Flags) > 0 {
		payload["flags"] = outcome.Flags
	}
	return marshalResult(payload)
}

// renderDone returns the final-completion instruction — recording any
// summary findings and calling agent_finished is left to the agent; this
// tool never signals completion itself.
func renderDone(outcome stepengine.Outcome) (string, bool, error) {
	payload := map[string]interface{}{
		"done":        true,
		"instruction": "All steps are complete. Record any final summary findings with findings_add, then call agent_finished.",
	}
	if len(outcome.Flags) > 0 {
		payload["flags"] = outcome.Flags
	}
	return marshalResult(payload)
}

// renderRotate is reached only after Advance has already committed the step
// completion — this leg just asks the spawner to rotate and tells the agent
// to stop; it must never rely on the idle-watcher's own rotation firing.
func renderRotate(env apirun.ToolEnv) (string, bool, error) {
	if env.Steps != nil {
		env.Steps.RequestStepRotation(env.SessionID)
	}
	return "step accepted — stop working now. This session is rotating; a fresh session will resume from the server-owned cursor.", false, nil
}

// renderRejected aggregates the rejection message with a remaining-attempts
// count for reasons that count toward the evidence cap (missing/invalid
// evidence, a failed check); a guard-miss rejection (stale_revision/
// step_mismatch) restates the current step_id/revision without touching the
// counter. At/over the cap, the session is force-failed via failSession so
// the row lands result=fail/result_reason=step_evidence_exhausted.
func renderRejected(env apirun.ToolEnv, engine *stepengine.Engine, stepID string, outcome stepengine.Outcome) (string, bool, error) {
	rej := outcome.Rejection
	if rej == nil {
		return "rejected", true, nil
	}
	if !rej.CountsTowardEvidenceCap() {
		return fmt.Sprintf("%s (current step_id=%q revision=%d)", rej.Message, stepID, outcome.Revision), true, nil
	}

	count, err := engine.RecordRejection(env.WorkflowInstanceID, env.NodeID, stepID)
	if err != nil {
		return err.Error(), true, nil
	}
	rejectionCap := service.StepRejectionCap(env.Pool, env.ProjectID)
	if count >= rejectionCap {
		return failSession(env, service.ResultReasonStepEvidenceExhausted)
	}
	return fmt.Sprintf("%s (attempt %d of %d)", rej.Message, count, rejectionCap), true, nil
}

func marshalResult(payload map[string]interface{}) (string, bool, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(b), false, nil
}
