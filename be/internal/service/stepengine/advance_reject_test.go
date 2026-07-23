package stepengine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// fakeCheckRunner is a canned CheckRunner (Rule 4 forbids real CLI
// execution) — it returns whatever failedIdx/exitCode/outputTail/err the
// test configures, and records every call it received.
type fakeCheckRunner struct {
	failedIdx  int
	exitCode   int
	outputTail string
	err        error
	calls      [][]string
}

func (f *fakeCheckRunner) RunChecks(_ context.Context, cmds []string) (int, int, string, error) {
	f.calls = append(f.calls, cmds)
	return f.failedIdx, f.exitCode, f.outputTail, f.err
}

func TestAdvance_StaleRevisionRejectsWithoutMutating(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	out, err := e.Advance(context.Background(), wfi, node, "s1", 99, evidenceOK())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeRejected {
		t.Fatalf("Kind = %v, want OutcomeRejected", out.Kind)
	}
	if out.Rejection == nil || out.Rejection.Reason != "stale_revision" {
		t.Errorf("Rejection = %+v, want reason stale_revision", out.Rejection)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Revision != 1 || state.CurrentIndex != 0 || len(state.Completed) != 0 {
		t.Errorf("cursor mutated by stale-revision Advance: revision=%d index=%d completed=%v", state.Revision, state.CurrentIndex, state.Completed)
	}
}

func TestAdvance_MismatchedStepIDRejectsWithoutMutating(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	pool := e.pool
	seedFinding(t, pool, wfi, "sess-adv", "summary", "did step 1")

	out, err := e.Advance(context.Background(), wfi, node, "s2", 1, evidenceOK())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeRejected {
		t.Fatalf("Kind = %v, want OutcomeRejected", out.Kind)
	}
	if out.Rejection == nil || out.Rejection.Reason != "step_mismatch" {
		t.Errorf("Rejection = %+v, want reason step_mismatch", out.Rejection)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Revision != 1 || state.CurrentIndex != 0 {
		t.Errorf("cursor mutated by mismatched step_id Advance: revision=%d index=%d", state.Revision, state.CurrentIndex)
	}
}

func TestAdvance_MissingEvidenceRejectsWithoutMutating(t *testing.T) {
	t.Parallel()
	e, wfi, node, _ := seedAdvanceFixture(t, nil)
	// No finding seeded — "summary" required_finding is missing.

	out, err := e.Advance(context.Background(), wfi, node, "s1", 1, evidenceOK())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeRejected {
		t.Fatalf("Kind = %v, want OutcomeRejected", out.Kind)
	}
	if out.Rejection == nil || out.Rejection.Reason != "missing_evidence" {
		t.Errorf("Rejection = %+v, want reason missing_evidence", out.Rejection)
	}

	state, err := e.State(wfi, node)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Revision != 1 || state.CurrentIndex != 0 || len(state.Completed) != 0 {
		t.Errorf("cursor mutated by missing-evidence Advance: revision=%d index=%d completed=%v", state.Revision, state.CurrentIndex, state.Completed)
	}
}

func TestAdvance_CheckFailureRejectsWithoutMutating(t *testing.T) {
	t.Parallel()
	checker := &fakeCheckRunner{failedIdx: -1}
	pool2 := newTestPool(t)
	seedProjectAndWorkflow(t, pool2, "proj-advchk", "wfi-advchk", "", "")
	tc := clock.NewTest(time.Now())
	e := New(pool2, tc, checker)
	checksJSON := `[{"step_id":"s1","title":"t","instruction":"i","checks":["make test"]},{"step_id":"s2","title":"t2","instruction":"i2"}]`
	if _, err := e.Snapshot("wfi-advchk", "node-a", stepwiseDef("def-chk", checksJSON)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool2, "sess-advchk", "proj-advchk", "wfi-advchk", "node-a")

	checker.failedIdx = 0
	checker.exitCode = 1
	checker.outputTail = "FAIL: something broke"

	out, err := e.Advance(context.Background(), "wfi-advchk", "node-a", "s1", 1, Evidence{SessionID: "sess-advchk"})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeRejected {
		t.Fatalf("Kind = %v, want OutcomeRejected", out.Kind)
	}
	if out.Rejection == nil || out.Rejection.Reason != "check_failed" {
		t.Errorf("Rejection = %+v, want reason check_failed", out.Rejection)
	}
	if len(checker.calls) != 1 {
		t.Fatalf("CheckRunner called %d times, want 1", len(checker.calls))
	}

	state, err := e.State("wfi-advchk", "node-a")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Revision != 1 || state.CurrentIndex != 0 {
		t.Errorf("cursor mutated by check-failure Advance: revision=%d index=%d", state.Revision, state.CurrentIndex)
	}
}

func TestAdvance_NilCheckRunnerSkipsChecksEntirely(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-advnil", "wfi-advnil", "", "")
	e := New(pool, clock.NewTest(time.Now()), nil)
	checksJSON := `[{"step_id":"s1","title":"t","instruction":"i","checks":["make test"]},{"step_id":"s2","title":"t2","instruction":"i2"}]`
	if _, err := e.Snapshot("wfi-advnil", "node-a", stepwiseDef("def-nil", checksJSON)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool, "sess-advnil", "proj-advnil", "wfi-advnil", "node-a")

	out, err := e.Advance(context.Background(), "wfi-advnil", "node-a", "s1", 1, Evidence{SessionID: "sess-advnil"})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeNext {
		t.Fatalf("Kind = %v, want OutcomeNext (nil CheckRunner must skip checks, not block)", out.Kind)
	}
}

func TestDecodeCompleted_EmptyStringYieldsEmptySlice(t *testing.T) {
	t.Parallel()
	got, err := decodeCompleted("")
	if err != nil {
		t.Fatalf("decodeCompleted(\"\"): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("decodeCompleted(\"\") = %v, want empty", got)
	}
}

func TestDecodeCompleted_RoundTrips(t *testing.T) {
	t.Parallel()
	entries := []model.CompletedStep{{StepID: "s1", CompletedAt: "2026-01-01T00:00:00Z"}}
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := decodeCompleted(string(b))
	if err != nil {
		t.Fatalf("decodeCompleted: %v", err)
	}
	if len(got) != 1 || got[0].StepID != "s1" {
		t.Errorf("decodeCompleted round trip = %+v, want [s1]", got)
	}
}
