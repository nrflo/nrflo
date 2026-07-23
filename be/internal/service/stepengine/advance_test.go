package stepengine

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// twoStepJSON is a stepwise steps JSON with two steps, each requiring a
// "summary" nonempty_text finding; both steps have rotation_allowed=true.
const twoStepJSON = `[
	{"step_id":"s1","title":"Step 1","instruction":"do 1","required_findings":[{"key":"summary","schema":"nonempty_text"}],"rotation_allowed":true},
	{"step_id":"s2","title":"Step 2","instruction":"do 2","required_findings":[{"key":"summary","schema":"nonempty_text"}],"rotation_allowed":true}
]`

func seedAdvanceFixture(t *testing.T, checks CheckRunner) (*Engine, string, string, *clock.TestClock) {
	t.Helper()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-adv", "wfi-adv", "", "")
	tc := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	e := New(pool, tc, checks)
	def := stepwiseDef("def-adv", twoStepJSON)
	if _, err := e.Snapshot("wfi-adv", "node-a", def); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool, "sess-adv", "proj-adv", "wfi-adv", "node-a")
	return e, "wfi-adv", "node-a", tc
}

func evidenceOK() Evidence {
	return Evidence{SessionID: "sess-adv", Summary: "done step", FindingKeys: []string{"summary"}}
}

func TestAdvance_HappyPathReturnsNextWithCompletedEntry(t *testing.T) {
	t.Parallel()
	e, wfi, node, tc := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	out, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeNext {
		t.Fatalf("Kind = %v, want OutcomeNext", out.Kind)
	}
	if out.NextStep == nil || out.NextStep.StepID != "s2" {
		t.Fatalf("NextStep = %+v, want s2", out.NextStep)
	}
	if out.Revision != 2 {
		t.Errorf("Revision = %d, want 2", out.Revision)
	}
	if out.CurrentIndex != 1 {
		t.Errorf("CurrentIndex = %d, want 1", out.CurrentIndex)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Completed) != 1 {
		t.Fatalf("Completed = %+v, want 1 entry", state.Completed)
	}
	c := state.Completed[0]
	if c.StepID != "s1" {
		t.Errorf("Completed[0].StepID = %q, want s1", c.StepID)
	}
	if c.Summary != "done step" {
		t.Errorf("Completed[0].Summary = %q, want %q", c.Summary, "done step")
	}
	if c.SessionID != "sess-adv" {
		t.Errorf("Completed[0].SessionID = %q, want sess-adv", c.SessionID)
	}
	if len(c.EvidenceKeys) != 1 || c.EvidenceKeys[0] != "summary" {
		t.Errorf("Completed[0].EvidenceKeys = %v, want [summary]", c.EvidenceKeys)
	}
	if c.CompletedAt != tc.Now().UTC().Format(time.RFC3339Nano) {
		t.Errorf("Completed[0].CompletedAt = %q, want %q", c.CompletedAt, tc.Now().UTC().Format(time.RFC3339Nano))
	}
}

func TestAdvance_LastStepReturnsDone(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	if _, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK()); err != nil {
		t.Fatalf("advance step 1: %v", err)
	}

	out, err := e.Advance(context.Background(), wfi, node, "s2", 2, evidenceOK())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeDone {
		t.Fatalf("Kind = %v, want OutcomeDone", out.Kind)
	}
}

// TestAdvance_ReplayReturnsSameOutcomeWithoutDoubleAdvancing verifies
// replaying the exact (step_id, revision) after a successful advance
// returns the SAME next step with Replayed=true and does not re-mutate.
func TestAdvance_ReplayReturnsSameOutcomeWithoutDoubleAdvancing(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	first, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK())
	if err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	if first.Kind != OutcomeNext {
		t.Fatalf("first Kind = %v, want OutcomeNext", first.Kind)
	}

	replay, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK())
	if err != nil {
		t.Fatalf("replay Advance: %v", err)
	}
	if !replay.Replayed {
		t.Error("replay.Replayed = false, want true")
	}
	if replay.Kind != OutcomeNext || replay.NextStep == nil || replay.NextStep.StepID != "s2" {
		t.Errorf("replay outcome = %+v, want same Next(s2) as the original call", replay)
	}
	if replay.Revision != first.Revision || replay.CurrentIndex != first.CurrentIndex {
		t.Errorf("replay revision/index = %d/%d, want same as original %d/%d", replay.Revision, replay.CurrentIndex, first.Revision, first.CurrentIndex)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Completed) != 1 {
		t.Errorf("Completed = %+v, want still exactly 1 entry (no double-advance)", state.Completed)
	}
}

func TestAdvance_OverThresholdRotationAllowedNonFinalYieldsRotate(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	ev := evidenceOK()
	ev.ContextTokens = 300000
	ev.RotateThreshold = 250000

	out, err := e.Advance(context.Background(), wfi, node, "s1", 1, ev)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeRotate {
		t.Fatalf("Kind = %v, want OutcomeRotate", out.Kind)
	}

	// The completion must already be persisted even though the outcome was
	// upgraded to Rotate.
	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Completed) != 1 || state.Completed[0].StepID != "s1" {
		t.Errorf("Completed = %+v, want persisted s1 entry despite Rotate", state.Completed)
	}
	if state.CurrentIndex != 1 {
		t.Errorf("CurrentIndex = %d, want 1 (advance still happened)", state.CurrentIndex)
	}
}

func TestAdvance_FinalStepNeverRotatesEvenOverThreshold(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	// Advance to the final step first.
	if _, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK()); err != nil {
		t.Fatalf("advance step 1: %v", err)
	}

	ev := evidenceOK()
	ev.ContextTokens = 300000
	ev.RotateThreshold = 250000

	out, err := e.Advance(context.Background(), wfi, node, "s2", 2, ev)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeDone {
		t.Fatalf("Kind = %v, want OutcomeDone (final step never rotates)", out.Kind)
	}
}

func TestAdvance_PastEndReturnsDoneWithoutMutating(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	if _, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK()); err != nil {
		t.Fatalf("advance step 1: %v", err)
	}
	if _, err := e.Advance(context.Background(), wfi, node, "s2", 2, evidenceOK()); err != nil {
		t.Fatalf("advance step 2: %v", err)
	}

	out, err := e.Advance(context.Background(), wfi, node, "s2", 3, evidenceOK())
	if err != nil {
		t.Fatalf("Advance past end: %v", err)
	}
	if out.Kind != OutcomeDone {
		t.Errorf("Kind = %v, want OutcomeDone", out.Kind)
	}
}

func TestAdvance_NoCursorReturnsErrNoCursor(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	e := New(pool, clock.NewTest(time.Now()), nil)

	_, err := e.Advance(context.Background(), "wfi-nope", "node-nope", "s1", 1, Evidence{})
	if err == nil {
		t.Fatal("Advance(no cursor) error = nil, want ErrNoCursor")
	}
}
