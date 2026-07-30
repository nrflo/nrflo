package spawner

// Cases 6 and 7 of the restart-feedback prepend coverage — split out of
// template_restart_feedback_test.go to stay under the 300-line source file
// limit. Shared helpers (createContinuedSessionReturningID,
// writeValidationFailureFindingRaw) live there.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/repo"

	"github.com/google/uuid"
)

// Case 6: an oversized output_tail is capped to at most 8KB and the kept
// bytes are the TAIL of the input, not the head.
func TestLoadTemplate_FailRestart_OversizedTail_KeepsTailNotHead(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "FR-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	sessionID := createContinuedSessionReturningID(t, env, ticketID, wfiID, "analyzer", "claude:sonnet-5", "test-phase", reasonFailRestart)

	const headMarker = "HEADSTARTMARKER"
	const tailMarker = "TAILENDMARKER1234567890"
	bigOutput := headMarker + strings.Repeat("Q", 20000) + tailMarker
	writeValidationFailureFindingRaw(t, env, sessionID, wfiID, "analyzer", "claude:sonnet-5", validationFindingActorID,
		"go test ./...", 1, bigOutput)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if strings.Contains(result, headMarker) {
		t.Error("expected the head of the oversized output_tail to be cut")
	}
	if !strings.Contains(result, tailMarker) {
		t.Error("expected the tail of the oversized output_tail to survive")
	}

	const outputHeader = "Output (tail):\n```\n"
	idx := strings.Index(result, outputHeader)
	if idx == -1 {
		t.Fatalf("expected the output-tail block markers, got: %s", result)
	}
	rest := result[idx+len(outputHeader):]
	end := strings.Index(rest, "\n```")
	if end == -1 {
		t.Fatalf("expected a closing fence for the output-tail block, got: %s", rest)
	}
	tailSection := rest[:end]
	if len(tailSection) > restartFeedbackTailSize+64 {
		t.Errorf("rendered tail section = %d bytes, want <= %d (plus truncation marker)", len(tailSection), restartFeedbackTailSize+64)
	}
	if !strings.HasSuffix(tailSection, tailMarker) {
		t.Errorf("expected the rendered tail section to end with the tail marker, got suffix: %q", lastN(tailSection, 60))
	}
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// Case 7: a stepwise def's fail-restart validation block folds the
// step-cursor resume body (${PREVIOUS_DATA}) inside itself, not under the
// (absent) low-context header.
func TestLoadTemplate_StepwiseFailRestart_PreviousDataInsideValidationBlock(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createStepwiseAgentDef(t, env, "analyzer", twoStepResumeSteps())

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")
	if _, err := env.pool.Exec(
		`UPDATE agent_step_cursors SET current_index = 1, completed = ? WHERE workflow_instance_id = ? AND node_id = ?`,
		`[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`, wfiID, "analyzer"); err != nil {
		t.Fatalf("advance cursor: %v", err)
	}

	sessionID := createContinuedSessionReturningID(t, env, ticketID, wfiID, "analyzer", "claude:sonnet-5", "analyzer", reasonFailRestart)
	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	denorm := repo.Denorm{ProjectID: env.project, WorkflowInstanceID: wfiID, AgentType: "analyzer", ModelID: "claude:sonnet-5"}
	agentActor := repo.Actor{Source: "agent"}
	for key, val := range map[string]interface{}{
		"summary": "did the first thing",
		"changes": []map[string]string{{"path": "pkg/real.go", "change": "added"}},
	} {
		b, err := json.Marshal(val)
		if err != nil {
			t.Fatalf("marshal %q: %v", key, err)
		}
		if err := findingRepo.Upsert("session", sessionID, key, json.RawMessage(b), denorm, agentActor); err != nil {
			t.Fatalf("upsert %q: %v", key, err)
		}
	}
	writeValidationFailureFindingRaw(t, env, sessionID, wfiID, "analyzer", "claude:sonnet-5", validationFindingActorID,
		"go test ./...", 1, "FAIL step2")

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	valIdx := strings.Index(result, "## Your Previous Run Failed Validation")
	if valIdx == -1 {
		t.Fatalf("expected the validation-failure header, got: %s", result)
	}
	if strings.Contains(result, "## Continuation From Saved State") {
		t.Error("low-context header must not render alongside the validation-failure block")
	}
	stepsIdx := strings.Index(result, "## Completed Steps (verified)")
	if stepsIdx == -1 {
		t.Fatalf("expected the step-cursor evidence block present, got: %s", result)
	}
	if !strings.Contains(result, "did the first thing") {
		t.Error("expected the step-cursor PREVIOUS_DATA content inside the block")
	}
	if stepsIdx < valIdx {
		t.Errorf("step-cursor evidence (idx=%d) must be inside the validation-failure block, not before its header (idx=%d)", stepsIdx, valIdx)
	}
}
