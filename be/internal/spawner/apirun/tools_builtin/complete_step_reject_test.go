package tools_builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"be/internal/service"
	"be/internal/spawner/apirun"
)

// TestCompleteStep_MissingEvidence_NamesKeyAggregatedAndDoesNotAdvance
// verifies a rejection message names the missing key exactly once
// (RejectionMessage aggregates, never dribbles one problem per call) and the
// cursor + rejection counter reflect exactly one recorded rejection.
func TestCompleteStep_MissingEvidence_NamesKeyAggregatedAndDoesNotAdvance(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	// No "summary" finding seeded.

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Fatalf("isErr = false, want true (rejected); output=%q", out)
	}
	if !strings.Contains(out, "summary") {
		t.Errorf("rejection message = %q, want it to name the missing key %q", out, "summary")
	}
	if strings.Count(out, "missing required findings") != 1 {
		t.Errorf("rejection message = %q, want exactly one aggregated missing-findings clause, not one per problem", out)
	}

	revision, currentIndex, _, rejections := env.readCursor(t)
	if revision != 1 || currentIndex != 0 {
		t.Errorf("cursor mutated by a rejected call: revision=%d current_index=%d", revision, currentIndex)
	}
	var counts map[string]int
	if uerr := json.Unmarshal([]byte(rejections), &counts); uerr != nil {
		t.Fatalf("unmarshal rejections %q: %v", rejections, uerr)
	}
	if counts["s1"] != 1 {
		t.Errorf("rejections[s1] = %d, want 1", counts["s1"])
	}
}

// TestCompleteStep_InvalidEvidenceSchema_NamesKeyAndIncrementsCounter
// verifies a present-but-schema-failing key is named in the message and
// still counts toward the evidence cap.
func TestCompleteStep_InvalidEvidenceSchema_NamesKeyAndIncrementsCounter(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "") // present, but fails nonempty_text

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Fatalf("isErr = false, want true (rejected); output=%q", out)
	}
	if !strings.Contains(out, "summary") {
		t.Errorf("rejection message = %q, want it to name the invalid key %q", out, "summary")
	}

	_, _, _, rejections := env.readCursor(t)
	var counts map[string]int
	if uerr := json.Unmarshal([]byte(rejections), &counts); uerr != nil {
		t.Fatalf("unmarshal rejections %q: %v", rejections, uerr)
	}
	if counts["s1"] != 1 {
		t.Errorf("rejections[s1] = %d, want 1", counts["s1"])
	}
}

// TestCompleteStep_GuardMiss_RestatesCurrentStepWithoutCountingTowardCap
// covers both stale_revision and step_mismatch: the underlying stepengine
// rejection message (embedded verbatim, see stepengine/advance.go's
// rejectedOutcome) restates the actual current step_id/revision, and neither
// guard-miss counts toward the evidence cap.
//
// NOTE (see be_production_bugs): complete_step_result.go's renderRejected
// appends a redundant "(current step_id=%q revision=%d)" suffix built from
// the AGENT-SUBMITTED step_id, not the cursor's actual current step. For
// step_mismatch this echoes the caller's own wrong step_id back as if it
// were "current" — this test only asserts on the correct embedded message,
// not that misleading suffix.
func TestCompleteStep_GuardMiss_RestatesCurrentStepWithoutCountingTowardCap(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"stale_revision", `{"step_id":"s1","revision":99}`},
		{"step_mismatch", `{"step_id":"wrong-step","revision":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newBuiltinTestEnv(t)
			env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)

			out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(tc.input))
			if err != nil {
				t.Fatalf("Invoke err: %v", err)
			}
			if !isErr {
				t.Fatalf("isErr = false, want true (rejected); output=%q", out)
			}
			if !strings.Contains(out, `step "s1"`) || !strings.Contains(out, "revision 1") {
				t.Errorf("guard-miss message = %q, want it to restate the actual current step (s1) and revision (1)", out)
			}

			revision, currentIndex, _, rejections := env.readCursor(t)
			if revision != 1 || currentIndex != 0 {
				t.Errorf("cursor mutated by a guard-miss rejection: revision=%d current_index=%d", revision, currentIndex)
			}
			var counts map[string]int
			if uerr := json.Unmarshal([]byte(rejections), &counts); uerr != nil {
				t.Fatalf("unmarshal rejections %q: %v", rejections, uerr)
			}
			if counts["s1"] != 0 {
				t.Errorf("rejections[s1] = %d, want 0 (guard misses never count toward the evidence cap)", counts["s1"])
			}
		})
	}
}

// TestCompleteStep_RejectionCap_ForceFailsSessionAtCap sets the project cap
// to 2 and drives two missing-evidence rejections; the second must force-fail
// the session (result=fail, result_reason=step_evidence_exhausted), return the
// FAIL TerminalSignal via errors.As, and broadcast agent.completed.
func TestCompleteStep_RejectionCap_ForceFailsSessionAtCap(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	if err := env.pool.SetProjectConfig(testProjectID, service.StepRejectionCapKey, "2"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	// First rejection: attempt 1 of 2, session still running.
	out1, isErr1, err1 := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1}`))
	if err1 != nil {
		t.Fatalf("first Invoke err: %v", err1)
	}
	if !isErr1 {
		t.Fatalf("first call isErr = false, want true; output=%q", out1)
	}
	if !strings.Contains(out1, "attempt 1 of 2") {
		t.Errorf("first rejection message = %q, want it to report attempt 1 of 2", out1)
	}
	if env.readSessionResult(t) != "" {
		t.Errorf("session result after attempt 1 = %q, want empty (not yet failed)", env.readSessionResult(t))
	}

	// Second rejection hits the cap: force-fail. failSession returns a
	// TerminalSignal error (not an isError result) — mirrors agent_fail's
	// own contract (agent.go's failSession).
	out2, isErr2, err2 := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1}`))
	if isErr2 {
		t.Errorf("second call isErr = true, want false (TerminalSignal path, not an isError result); output=%q", out2)
	}
	if out2 != "" {
		t.Errorf("second call output = %q, want empty (TerminalSignal carries no output)", out2)
	}
	if err2 == nil {
		t.Fatal("second call error = nil, want a FAIL TerminalSignal")
	}
	var term apirun.TerminalSignal
	if !errors.As(err2, &term) {
		t.Fatalf("second call error = %v, want errors.As to unwrap a TerminalSignal", err2)
	}
	if term.Status != "FAIL" {
		t.Errorf("TerminalSignal.Status = %q, want FAIL", term.Status)
	}

	if got := env.readSessionResult(t); got != "fail" {
		t.Errorf("session result = %q, want fail", got)
	}
	if got := env.readSessionResultReason(t); got != service.ResultReasonStepEvidenceExhausted {
		t.Errorf("session result_reason = %q, want %q", got, service.ResultReasonStepEvidenceExhausted)
	}
	if len(env.hub.events) == 0 {
		t.Error("no ws events broadcast on force-fail")
	}
}
