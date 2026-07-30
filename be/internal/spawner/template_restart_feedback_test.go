package spawner

// Tests for the truthful restart-feedback prepend (template_restart_feedback.go):
// a fail_restart relaunch only renders the validation-failure block when the
// PREVIOUS session itself wrote a validation_failure finding under the
// genuine actor (validationFindingActorID); a timeout_restart always
// renders. Both replace the low-context block for that relaunch rather than
// stacking with it (template.go's prepend seam).

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// createContinuedSessionReturningID creates a continued agent_sessions row
// with no findings and returns its ID, so callers can attach a
// validation_failure finding under a specific actor via
// writeValidationFailureFindingRaw.
func createContinuedSessionReturningID(t *testing.T, env *spawnerTestEnv, ticketID, wfiID, agentType, modelID, phase, resultReason string) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessionID := uuid.New().String()
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.project,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              phase,
		NodeID:             phase,
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionContinued,
		Result:             sql.NullString{String: "continue", Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
		EndedAt:            sql.NullString{String: now, Valid: true},
		ResultReason:       sql.NullString{String: resultReason, Valid: true},
	}
	sessionRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create continued session: %v", err)
	}
	return sessionID
}

// writeValidationFailureFindingRaw upserts a validation_failure finding on
// sessionID under actorID — actorID distinguishes a genuine validation write
// ("validation", the actual validationFindingActorID) from a stale
// carried-forward copy ("continuation").
func writeValidationFailureFindingRaw(t *testing.T, env *spawnerTestEnv, sessionID, wfiID, agentType, modelID, actorID, command string, exitCode int, outputTail string) {
	t.Helper()
	payload, err := json.Marshal(map[string]interface{}{
		"command":       command,
		"command_index": 0,
		"exit_code":     exitCode,
		"output_tail":   outputTail,
	})
	if err != nil {
		t.Fatalf("marshal validation_failure payload: %v", err)
	}
	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	denorm := repo.Denorm{ProjectID: env.project, WorkflowInstanceID: wfiID, AgentType: agentType, ModelID: modelID}
	actor := repo.Actor{Source: "system", ID: actorID}
	if err := findingRepo.Upsert("session", sessionID, findingKeyValidationFailure, json.RawMessage(payload), denorm, actor); err != nil {
		t.Fatalf("upsert validation_failure finding: %v", err)
	}
}

// Case 1 + 2: a genuine validation_failure finding (actor=validation) on a
// fail_restart relaunch renders the failed command/exit code/tail, and never
// the low-context wording/header.
func TestLoadTemplate_FailRestart_GenuineFinding_RendersValidationBlockOnly(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "FR-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	sessionID := createContinuedSessionReturningID(t, env, ticketID, wfiID, "analyzer", "claude:sonnet-5", "test-phase", reasonFailRestart)
	writeValidationFailureFindingRaw(t, env, sessionID, wfiID, "analyzer", "claude:sonnet-5", validationFindingActorID,
		"make test", 1, "boom: assertion failed\nFAIL\texit status 1")

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "## Your Previous Run Failed Validation") {
		t.Fatalf("expected validation-failure header, got: %s", result)
	}
	if !strings.Contains(result, "make test") {
		t.Error("expected the failed command in the prompt")
	}
	if !strings.Contains(result, "Exit code: 1") {
		t.Error("expected the exit code in the prompt")
	}
	if !strings.Contains(result, "FAIL\texit status 1") {
		t.Error("expected the output-tail substring in the prompt")
	}

	// Requirement 5: never the low-context wording/header alongside a
	// genuine validation-failure block.
	if strings.Contains(result, "interrupted at low context") {
		t.Error("must not render low-context wording for a genuine fail_restart")
	}
	if strings.Contains(result, "## Continuation From Saved State") {
		t.Error("must not render the low-context header for a genuine fail_restart")
	}
}

// Case 3: a validation_failure finding carried forward under the
// "continuation" actor (copyFindingsForContinuation's stamp) is not genuine
// — no validation-failure block renders, and the untouched low-context path
// still falls back normally when there is real previous data (to_resume).
func TestLoadTemplate_FailRestart_StaleCarriedFinding_FallsBackToLowContext(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "FR-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	sessionID := createContinuedSessionReturningID(t, env, ticketID, wfiID, "analyzer", "claude:sonnet-5", "test-phase", reasonFailRestart)
	writeValidationFailureFindingRaw(t, env, sessionID, wfiID, "analyzer", "claude:sonnet-5", "continuation",
		"make test", 1, "some stale tail")

	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	denorm := repo.Denorm{ProjectID: env.project, WorkflowInstanceID: wfiID, AgentType: "analyzer", ModelID: "claude:sonnet-5"}
	toResume, err := json.Marshal("saved progress data")
	if err != nil {
		t.Fatalf("marshal to_resume: %v", err)
	}
	if err := findingRepo.Upsert("session", sessionID, "to_resume", json.RawMessage(toResume), denorm, repo.Actor{Source: "agent"}); err != nil {
		t.Fatalf("upsert to_resume: %v", err)
	}

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if strings.Contains(result, "## Your Previous Run Failed Validation") {
		t.Errorf("stale carried-forward finding (actor=continuation) must not render the validation-failure block, got: %s", result)
	}
	if !strings.Contains(result, "## Continuation From Saved State") {
		t.Error("expected the untouched low-context header as the fallback")
	}
	if !strings.Contains(result, "saved progress data") {
		t.Error("expected the low-context PREVIOUS_DATA content")
	}
}

// Case 4: reason=low_context + to_resume is the untouched path — regression
// guard that the restart-feedback change left it alone.
func TestLoadTemplate_LowContext_Unaffected(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "LC-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	createContinuedSessionInEnv(t, env, ticketID, wfiID,
		"analyzer", "claude:sonnet-5", "test-phase", "low_context",
		map[string]interface{}{"to_resume": "saved progress data"})

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "## Continuation From Saved State") {
		t.Error("expected the low-context header unchanged")
	}
	if strings.Contains(result, "## Your Previous Run Failed Validation") || strings.Contains(result, "## Your Previous Run Timed Out") {
		t.Error("low_context reason must never render a restart-feedback block")
	}
}

// Case 5: timeout_restart always renders the timeout wording, never a
// validation section, even with no previous data at all.
func TestLoadTemplate_TimeoutRestart_RendersTimeoutBlock(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "TR-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	createContinuedSessionInEnv(t, env, ticketID, wfiID,
		"analyzer", "claude:sonnet-5", "test-phase", reasonTimeoutRestart,
		map[string]interface{}{})

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "## Your Previous Run Timed Out") {
		t.Fatalf("expected the timeout-restart header, got: %s", result)
	}
	if strings.Contains(result, "## Your Previous Run Failed Validation") {
		t.Error("timeout_restart must never render a validation section")
	}
	if strings.Contains(result, "## Continuation From Saved State") {
		t.Error("timeout_restart must never render the low-context header")
	}
}

// Cases 6 and 7 (oversized-tail capping, stepwise PREVIOUS_DATA folding) are
// in template_restart_feedback_extra_test.go — split to stay under the
// 300-line source file limit.
