package stepengine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"be/internal/model"
)

// Evidence is the agent-supplied completion payload for one Advance call.
type Evidence struct {
	SessionID       string
	Summary         string
	FindingKeys     []string
	ContextTokens   int
	RotateThreshold int
}

// Advance is the exactly-once, CAS-guarded step transition. The caller
// passes the revision/step_id it believes is current; a guard miss is either
// an idempotent same-revision replay (returns the already-issued outcome
// with Replayed=true, never re-mutating the row) or a Rejection — never a Go
// error. Go errors are reserved for infrastructure failures (DB, corrupt
// snapshot, unknown node).
func (e *Engine) Advance(ctx context.Context, instanceID, nodeID, stepID string, revision int, ev Evidence) (Outcome, error) {
	cursor, err := e.cursorRepo.Get(instanceID, nodeID)
	if err != nil {
		return Outcome{}, ErrNoCursor
	}
	steps, err := decodeSteps([]byte(cursor.StepsSnapshot))
	if err != nil {
		return Outcome{}, ErrBadSnapshot
	}
	completed, err := decodeCompleted(cursor.Completed)
	if err != nil {
		return Outcome{}, ErrBadSnapshot
	}

	if cursor.CurrentIndex >= len(steps) {
		return Outcome{Kind: OutcomeDone, Revision: cursor.Revision, CurrentIndex: cursor.CurrentIndex}, nil
	}
	currentStep := steps[cursor.CurrentIndex]

	if revision != cursor.Revision || stepID != currentStep.StepID {
		if isReplay(cursor.Revision, revision, stepID, completed) {
			return e.replayWithSignals(instanceID, nodeID, steps, cursor, completed, ev), nil
		}
		return rejectedOutcome(cursor, guardMissReason(revision, cursor.Revision), "cursor is at revision %d, step %q — resubmit with the current values", cursor.Revision, currentStep.StepID), nil
	}

	ec := EvidenceContext{InstanceID: instanceID, NodeID: nodeID, SessionID: ev.SessionID, RepoRoot: e.resolveWorktreeRoot(instanceID)}
	evResult, err := e.ValidateEvidence(currentStep, ec)
	if err != nil {
		return Outcome{}, err
	}
	if !evResult.OK {
		return rejectedOutcome(cursor, evResult.RejectionReason(), "%s", evResult.RejectionMessage()), nil
	}

	if e.checks != nil && len(currentStep.Checks) > 0 {
		failedIdx, exitCode, outputTail, err := e.checks.RunChecks(ctx, currentStep.Checks)
		if err != nil {
			return Outcome{}, err
		}
		if failedIdx >= 0 {
			return rejectedOutcome(cursor, "check_failed", "check failed: %s (exit %d)\n%s", currentStep.Checks[failedIdx], exitCode, outputTail), nil
		}
	}

	rotated := rotateDecision(ev, currentStep, cursor.CurrentIndex, len(steps))
	completed = append(completed, model.CompletedStep{
		StepID:       stepID,
		EvidenceKeys: ev.FindingKeys,
		Summary:      ev.Summary,
		SessionID:    ev.SessionID,
		CompletedAt:  e.clock.Now().UTC().Format(time.RFC3339Nano),
		Rotated:      rotated,
	})
	completedJSON, err := json.Marshal(completed)
	if err != nil {
		return Outcome{}, err
	}

	ok, err := e.cursorRepo.Advance(instanceID, nodeID, cursor.Revision, cursor.CurrentIndex, string(completedJSON))
	if err != nil {
		return Outcome{}, err
	}
	if !ok {
		return e.advanceCASMiss(instanceID, nodeID, stepID, revision, ev, steps, evResult.Flags)
	}

	newIndex := cursor.CurrentIndex + 1
	outcome := e.nextOutcome(steps, newIndex, cursor.Revision+1)
	outcome.Flags = evResult.Flags
	applyRotateUpgrade(&outcome, rotated)
	return outcome, nil
}

// rotateDecision is the single ShouldRotate call site for a completed step,
// consumed both to decide the Outcome (applyRotateUpgrade) and to stamp the
// durable model.CompletedStep.Rotated flag — the outcome and the stored flag
// can never disagree.
func rotateDecision(ev Evidence, step model.StepDefinition, completedIndex, stepCount int) bool {
	return ShouldRotate(RotateInput{
		ContextTokens:   ev.ContextTokens,
		ThresholdTokens: ev.RotateThreshold,
		RotationAllowed: step.RotationAllowed,
		FinalStep:       completedIndex == stepCount-1,
	})
}

// applyRotateUpgrade upgrades an OutcomeNext to OutcomeRotate when rotated is
// true. Shared by the main success path and advanceCASMiss's replay path so
// both agree on whether a given advance triggered rotation.
func applyRotateUpgrade(outcome *Outcome, rotated bool) {
	if outcome.Kind != OutcomeNext {
		return
	}
	if rotated {
		outcome.Kind = OutcomeRotate
	}
}

// advanceCASMiss handles a lost race on repo.Advance: re-reads the row and,
// if the miss is explained by this exact call having already landed
// (revision advanced by one, last completed entry is this step_id), returns
// the replay outcome (with the same rotate upgrade the winning caller would
// have received); otherwise it's a genuine concurrent-mutation rejection.
// Never mutates the row.
func (e *Engine) advanceCASMiss(instanceID, nodeID, stepID string, revision int, ev Evidence, steps []model.StepDefinition, flags []string) (Outcome, error) {
	fresh, err := e.cursorRepo.Get(instanceID, nodeID)
	if err != nil {
		return Outcome{}, ErrNoCursor
	}
	freshCompleted, err := decodeCompleted(fresh.Completed)
	if err == nil && isReplay(fresh.Revision, revision, stepID, freshCompleted) {
		outcome := e.replayOutcome(steps, fresh)
		outcome.Flags = flags
		if completedIndex := fresh.CurrentIndex - 1; completedIndex >= 0 && completedIndex < len(steps) {
			applyRotateUpgrade(&outcome, rotateDecision(ev, steps[completedIndex], completedIndex, len(steps)))
		}
		return outcome, nil
	}
	return rejectedOutcome(fresh, "stale_revision", "cursor advanced concurrently to revision %d", fresh.Revision), nil
}

// isReplay reports whether an Advance guard miss is the same call arriving
// twice: the caller's revision is exactly one behind the cursor's, and the
// most recently completed step is the one being replayed.
func isReplay(cursorRevision, callerRevision int, stepID string, completed []model.CompletedStep) bool {
	if callerRevision != cursorRevision-1 || len(completed) == 0 {
		return false
	}
	return completed[len(completed)-1].StepID == stepID
}

func guardMissReason(callerRevision, cursorRevision int) string {
	if callerRevision == cursorRevision {
		return "step_mismatch"
	}
	return "stale_revision"
}

func rejectedOutcome(cursor *model.AgentStepCursor, reason, format string, args ...interface{}) Outcome {
	return Outcome{
		Kind:         OutcomeRejected,
		Revision:     cursor.Revision,
		CurrentIndex: cursor.CurrentIndex,
		Rejection:    &Rejection{Reason: reason, Message: fmt.Sprintf(format, args...)},
	}
}

// nextOutcome computes the outcome for landing on index at revision:
// OutcomeDone past the end, else OutcomeNext with that step.
func (e *Engine) nextOutcome(steps []model.StepDefinition, index, revision int) Outcome {
	if index >= len(steps) {
		return Outcome{Kind: OutcomeDone, Revision: revision, CurrentIndex: index}
	}
	next := steps[index]
	return Outcome{Kind: OutcomeNext, NextStep: &next, Revision: revision, CurrentIndex: index}
}

// replayOutcome rebuilds the outcome the original successful call issued,
// recomputed from the cursor's current (already-advanced) state.
func (e *Engine) replayOutcome(steps []model.StepDefinition, cursor *model.AgentStepCursor) Outcome {
	outcome := e.nextOutcome(steps, cursor.CurrentIndex, cursor.Revision)
	outcome.Replayed = true
	return outcome
}

// replayWithSignals rebuilds a guard-miss replay outcome and reattaches the
// two signals the winning caller's outcome carried but a bare replayOutcome
// drops: the rotate upgrade (read from the durable CompletedStep.Rotated
// stamp — authoritative, unlike a recompute off the replay call's evidence)
// and the non-fatal path-resolution Flags (recomputed for the just-completed
// step). This mirrors advanceCASMiss so both replay paths return an identical
// outcome, honoring the "a replay returns the original outcome again" invariant.
func (e *Engine) replayWithSignals(instanceID, nodeID string, steps []model.StepDefinition, cursor *model.AgentStepCursor, completed []model.CompletedStep, ev Evidence) Outcome {
	outcome := e.replayOutcome(steps, cursor)
	if len(completed) > 0 {
		applyRotateUpgrade(&outcome, completed[len(completed)-1].Rotated)
	}
	if idx := cursor.CurrentIndex - 1; idx >= 0 && idx < len(steps) {
		ec := EvidenceContext{InstanceID: instanceID, NodeID: nodeID, SessionID: ev.SessionID, RepoRoot: e.resolveWorktreeRoot(instanceID)}
		if evResult, err := e.ValidateEvidence(steps[idx], ec); err == nil {
			outcome.Flags = evResult.Flags
		}
	}
	return outcome
}

func decodeCompleted(raw string) ([]model.CompletedStep, error) {
	var completed []model.CompletedStep
	if raw == "" {
		return completed, nil
	}
	if err := json.Unmarshal([]byte(raw), &completed); err != nil {
		return nil, err
	}
	return completed, nil
}
