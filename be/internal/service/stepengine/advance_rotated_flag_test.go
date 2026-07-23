package stepengine

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// rotatingEvidence returns Evidence configured to cross the rotate threshold.
func rotatingEvidence() Evidence {
	ev := evidenceOK()
	ev.ContextTokens = 300000
	ev.RotateThreshold = 250000
	return ev
}

// TestAdvance_CompletedStepRotatedStampedTrueWhenRotating verifies the
// persisted CompletedStep entry carries Rotated=true when ShouldRotate fires
// — the same decision that upgraded the Outcome.
func TestAdvance_CompletedStepRotatedStampedTrueWhenRotating(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	out, err := e.Advance(context.Background(), wfi, node, "s1", 1, rotatingEvidence())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeRotate {
		t.Fatalf("Kind = %v, want OutcomeRotate", out.Kind)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Completed) != 1 || !state.Completed[0].Rotated {
		t.Errorf("Completed[0].Rotated = %v, want true", state.Completed)
	}
}

// TestAdvance_CompletedStepRotatedStampedFalseWhenNotRotating verifies a
// plain accepted advance (no threshold crossed) stamps Rotated=false.
func TestAdvance_CompletedStepRotatedStampedFalseWhenNotRotating(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	out, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeNext {
		t.Fatalf("Kind = %v, want OutcomeNext", out.Kind)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Completed) != 1 || state.Completed[0].Rotated {
		t.Errorf("Completed[0].Rotated = %v, want false", state.Completed)
	}
}

// TestAdvance_CompletedStepRotatedFalseOnFinalStepEvenOverThreshold verifies
// the final-step negative: over threshold, but the completed step is the
// last one, so Rotated must stay false and the outcome is Done.
func TestAdvance_CompletedStepRotatedFalseOnFinalStepEvenOverThreshold(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	if _, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK()); err != nil {
		t.Fatalf("advance step 1: %v", err)
	}

	out, err := e.Advance(context.Background(), wfi, node, "s2", 2, rotatingEvidence())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeDone {
		t.Fatalf("Kind = %v, want OutcomeDone", out.Kind)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Completed) != 2 || state.Completed[1].Rotated {
		t.Errorf("Completed[1].Rotated = %v, want false (final step never rotates)", state.Completed)
	}
}

// TestAdvance_CompletedStepRotatedFalseWhenRotationNotAllowed verifies the
// rotation_allowed=false negative: over threshold but the snapshot step
// forbids rotation.
func TestAdvance_CompletedStepRotatedFalseWhenRotationNotAllowed(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-advnorot", "wfi-advnorot", "", "")
	e := New(pool, clock.NewTest(time.Now()), nil)
	stepsJSON := `[
		{"step_id":"s1","title":"Step 1","instruction":"do 1","required_findings":[{"key":"summary","schema":"nonempty_text"}],"rotation_allowed":false},
		{"step_id":"s2","title":"Step 2","instruction":"do 2"}
	]`
	if _, err := e.Snapshot("wfi-advnorot", "node-a", stepwiseDef("def-advnorot", stepsJSON)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool, "sess-advnorot", "proj-advnorot", "wfi-advnorot", "node-a")
	seedFinding(t, pool, "wfi-advnorot", "sess-advnorot", "summary", "did step 1")

	ev := Evidence{SessionID: "sess-advnorot", Summary: "done", FindingKeys: []string{"summary"}, ContextTokens: 300000, RotateThreshold: 250000}
	out, err := e.Advance(context.Background(), "wfi-advnorot", "node-a", "s1", 1, ev)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeNext {
		t.Fatalf("Kind = %v, want OutcomeNext (rotation_allowed=false)", out.Kind)
	}

	state, err := e.State("wfi-advnorot", "node-a")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Completed) != 1 || state.Completed[0].Rotated {
		t.Errorf("Completed[0].Rotated = %v, want false (rotation_allowed=false)", state.Completed)
	}
}

// TestAdvance_TopLevelReplayReappliesRotateUpgrade verifies the common
// guard-miss replay path (advance.go's isReplay branch, distinct from
// advanceCASMiss) reapplies the rotate upgrade off the durable Rotated stamp:
// a first call rotates, then replaying the exact (step_id, revision) returns
// OutcomeRotate again with Replayed=true — never a bare Next that would fail
// to re-tell the agent to stop.
func TestAdvance_TopLevelReplayReappliesRotateUpgrade(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	ev := rotatingEvidence()
	first, err := e.Advance(context.Background(), wfi, node, "s1", 1, ev)
	if err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	if first.Kind != OutcomeRotate {
		t.Fatalf("first Kind = %v, want OutcomeRotate", first.Kind)
	}

	// Replay the exact original call: revision 1 is now one behind the
	// advanced cursor (revision 2), so this takes the top-level isReplay
	// branch — which must reproduce the rotate outcome, not a bare Next.
	replay, err := e.Advance(context.Background(), wfi, node, "s1", 1, ev)
	if err != nil {
		t.Fatalf("replay Advance: %v", err)
	}
	if !replay.Replayed {
		t.Error("replay.Replayed = false, want true")
	}
	if replay.Kind != OutcomeRotate {
		t.Errorf("replay Kind = %v, want OutcomeRotate (top-level replay must reapply the rotate upgrade)", replay.Kind)
	}
}

// TestAdvance_CASMissReplayAgreesWithWinningCallerOnRotated verifies
// advanceCASMiss's replay branch recomputes the same Rotated upgrade the
// winning caller would have received: force a CAS miss by advancing the
// cursor out from under a stale in-memory read, then replay the exact
// original call and check the outcome still reports Rotate.
func TestAdvance_CASMissReplayAgreesWithWinningCallerOnRotated(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	ev := rotatingEvidence()

	// First call wins the race and commits revision 1->2, current_index 0->1,
	// with the completed[0] entry carrying Rotated=true (winning path).
	first, err := e.Advance(context.Background(), wfi, node, "s1", 1, ev)
	if err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	if first.Kind != OutcomeRotate {
		t.Fatalf("first Kind = %v, want OutcomeRotate", first.Kind)
	}

	// Exercise advanceCASMiss directly with the same (stepID, revision) the
	// winning caller used: it re-reads the now-advanced cursor, recognizes
	// this as a replay of the call that already landed, and must recompute
	// the same rotate upgrade the winning caller received.
	fresh, err := e.cursorRepo.Get(wfi, node)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	steps, err := decodeSteps([]byte(fresh.StepsSnapshot))
	if err != nil {
		t.Fatalf("decodeSteps: %v", err)
	}
	replayOutcome, err := e.advanceCASMiss(wfi, node, "s1", 1, ev, steps, nil)
	if err != nil {
		t.Fatalf("advanceCASMiss: %v", err)
	}
	if !replayOutcome.Replayed {
		t.Error("advanceCASMiss replay outcome: Replayed = false, want true")
	}
	if replayOutcome.Kind != OutcomeRotate {
		t.Errorf("advanceCASMiss replay Kind = %v, want OutcomeRotate (must agree with the winning caller)", replayOutcome.Kind)
	}
}
