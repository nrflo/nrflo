package spawner

// Golden full-body prompt snapshots for the stepwise prompt-assembly seam
// (template_stepwise.go). Unlike template_stepwise_test.go's substring
// assertions, these compare the ENTIRE rendered prompt against a literal
// expected string reconstructed by hand from the stepwise-guidance
// injectable seed (migrations/000203_stepwise_guidance_injectable.up.sql)
// and appendStepwiseBlock's documented layout — any accidental whitespace/
// ordering/field drift in either fails the test.

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/repo"

	"github.com/google/uuid"
)

// stepwiseGuidanceBody mirrors the readonly `stepwise-guidance` injectable's
// template column verbatim (migration 000203) with vars substituted in.
func stepwiseGuidanceBody(stepIndex, stepTotal, stepTitle, stepID, stepRevision string) string {
	return "## Stepwise Mode\n\n" +
		"You are working through a server-owned sequence of steps for this task: step " + stepIndex + " of " + stepTotal + " — \"" + stepTitle + "\" (step_id=" + stepID + ", revision=" + stepRevision + ").\n\n" +
		"- You cannot see step " + stepIndex + "'s successor until this step is accepted — do not attempt or pre-answer future steps.\n" +
		"- The server owns the cursor. You advance only by calling `complete_step` with `{step_id, revision, summary, evidence: {finding_keys}}`.\n" +
		"- Record every required finding with `findings_add` BEFORE calling `complete_step` — the call validates against what is already recorded, not what you say you did.\n" +
		"- A rejected `complete_step` call lists exactly what is missing or invalid. Fix and resubmit — never guess at what might satisfy it.\n" +
		"- Use the exact `step_id` and `revision` shown above; a stale revision is rejected.\n"
}

// TestAppendStepwiseBlock_GoldenInitialRender is a full-body snapshot of a
// fresh cursor's rendered prompt (revision 1, current_index 0, nothing
// completed) — the exact bytes a stepwise agent's first turn sees.
func TestAppendStepwiseBlock_GoldenInitialRender(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SW-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	expected := "Main prompt body" +
		"\n\n" + stepwiseGuidanceBody("1", "3", "Title One", "step-one", "1") +
		"\n\n## Steps (step 1 of 3)\n" +
		"1. [current] Title One\n" +
		"2. [locked] Title Two\n" +
		"3. [locked] Title Three\n" +
		"\n### Current step: Title One\n" +
		"step_id=step-one revision=1\n\n" +
		"Instruction body one."

	if result != expected {
		t.Errorf("golden initial render mismatch:\n--- got ---\n%s\n--- want ---\n%s", result, expected)
	}
}

// TestAppendStepwiseBlock_GoldenRelaunchRender is the same snapshot after a
// real CAS-guarded Advance (step-one completed, revision 1->2, current_index
// 0->1) — the exact bytes a relaunched agent's next turn sees.
func TestAppendStepwiseBlock_GoldenRelaunchRender(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SW-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	cursorRepo := repo.NewAgentStepCursorRepo(env.pool, clock.Real())
	completedJSON := `[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`
	ok, advErr := cursorRepo.Advance(wfiID, "analyzer", 1, 0, completedJSON)
	if advErr != nil {
		t.Fatalf("Advance: %v", advErr)
	}
	if !ok {
		t.Fatal("Advance() = false, want true (CAS should succeed on a fresh cursor)")
	}

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	expected := "Main prompt body" +
		"\n\n" + stepwiseGuidanceBody("2", "3", "Title Two", "step-two", "2") +
		"\n\n## Steps (step 2 of 3)\n" +
		"1. [completed] Title One\n" +
		"2. [current] Title Two\n" +
		"3. [locked] Title Three\n" +
		"\n### Current step: Title Two\n" +
		"step_id=step-two revision=2\n\n" +
		"Instruction body two."

	if result != expected {
		t.Errorf("golden relaunch render mismatch:\n--- got ---\n%s\n--- want ---\n%s", result, expected)
	}
}
