package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/service"

	"github.com/google/uuid"
)

// TestHandleCompletion_StepwiseExplicitFail_GuardNeverRuns is case 3: an
// explicit fail already recorded means result != "pass", so the guard's
// condition never even evaluates, and no steps_incomplete finding appears.
func TestHandleCompletion_StepwiseExplicitFail_GuardNeverRuns(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	createStepwiseAgentDefInTestEnv(t, env, "test-agent", threeSteps())
	insertStepCursor(t, env, "test-agent", threeSteps(), 0, nil)

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.UpdateResult(env.sessionID, "fail", "explicit"); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}

	proc := stepwiseCompletionProc(env, "test-agent", []string{"false"})
	proc.cmd = runTrue(t)

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: env.workflowID, AgentType: "test-agent",
	})

	s, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if !s.ResultReason.Valid || s.ResultReason.String != "explicit" {
		t.Errorf("result_reason = %v, want explicit", s.ResultReason)
	}
	if p := getStepsIncompleteFinding(t, env, env.sessionID); p != nil {
		t.Error("steps_incomplete finding should not exist when explicit fail was pre-set")
	}
}

// TestHandleCompletion_FullMode_GuardNoOpDespiteStrayCursor is case 4: a
// full-mode def (no prompt_mode/steps) means the guard degrades silently
// even when a stray cursor row exists for the same (wfiID, nodeID).
func TestHandleCompletion_FullMode_GuardNoOpDespiteStrayCursor(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	createFullModeAgentDefInTestEnv(t, env, "test-agent")
	insertStepCursor(t, env, "test-agent", threeSteps(), 0, nil)

	proc := stepwiseCompletionProc(env, "test-agent", nil)
	proc.cmd = runTrue(t)

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: env.workflowID, AgentType: "test-agent",
	})

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	s, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if !s.Result.Valid || s.Result.String != "pass" {
		t.Errorf("result = %v, want pass (full-mode def, guard must be a no-op)", s.Result)
	}
	if p := getStepsIncompleteFinding(t, env, env.sessionID); p != nil {
		t.Error("unexpected steps_incomplete finding for a full-mode def")
	}
}

// TestHandleCompletion_NoCursorRow_GuardDegradesSilently is case 5: a
// stepwise def with no cursor row (snapshot failed/never ran) must degrade
// to false rather than inventing a failure.
func TestHandleCompletion_NoCursorRow_GuardDegradesSilently(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	createStepwiseAgentDefInTestEnv(t, env, "test-agent", threeSteps())
	// No cursor row inserted.

	proc := stepwiseCompletionProc(env, "test-agent", nil)
	proc.cmd = runTrue(t)

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: env.workflowID, AgentType: "test-agent",
	})

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	s, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if !s.Result.Valid || s.Result.String != "pass" {
		t.Errorf("result = %v, want pass (no cursor row, guard must degrade silently)", s.Result)
	}
	if p := getStepsIncompleteFinding(t, env, env.sessionID); p != nil {
		t.Error("unexpected steps_incomplete finding with no cursor row")
	}
}

// TestCopyFindingsForContinuation_CarriesStepsIncomplete is case 6: a
// steps_incomplete finding planted on the old session must carry to the new
// session via copyFindingsForContinuation (clone of the validation_failure
// carryover test).
func TestCopyFindingsForContinuation_CarriesStepsIncomplete(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	oldID := uuid.New().String()
	newID := uuid.New().String()
	env.createNamedSession(t, oldID, "claude:sonnet-5")
	env.createNamedSession(t, newID, "claude:sonnet-5")

	payload := json.RawMessage(`{"step_id":"step-one","step_index":1,"total":3,"completed_step_ids":[],"pending_titles":["Title One","Title Two","Title Three"]}`)
	findingRepo := repo.NewFindingRepo(env.database, clock.Real())
	denorm := repo.Denorm{
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
		AgentType:          "test-agent",
		ModelID:            "claude:sonnet-5",
	}
	if err := findingRepo.Upsert("session", oldID, service.FindingKeyStepsIncomplete, payload, denorm,
		repo.Actor{Source: "system", ID: "stepwise"}); err != nil {
		t.Fatalf("Upsert old finding: %v", err)
	}

	env.spawner.copyFindingsForContinuation(context.Background(), oldID, newID)

	m, err := findingRepo.GetOwn("session", newID)
	if err != nil {
		t.Fatalf("GetOwn new session: %v", err)
	}
	raw, ok := m[service.FindingKeyStepsIncomplete]
	if !ok {
		t.Fatal("expected steps_incomplete on new session after carryover, got none")
	}
	var p map[string]interface{}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal carried finding: %v", err)
	}
	if p["step_id"] != "step-one" {
		t.Errorf("carried finding step_id = %v, want step-one", p["step_id"])
	}
}
