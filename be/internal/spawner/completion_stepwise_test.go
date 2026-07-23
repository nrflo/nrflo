package spawner

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// createStepwiseAgentDefInTestEnv inserts a prompt_mode='stepwise' agent def
// under env's project/workflow ("feature"), keyed by agentType so
// loadAgentDefinition(agentType, projectID, workflowName) resolves it.
func createStepwiseAgentDefInTestEnv(t *testing.T, env *testEnv, agentType string, steps []model.StepDefinition) {
	t.Helper()
	b, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal steps: %v", err)
	}
	stepsJSON := string(b)

	adRepo := repo.NewAgentDefinitionRepo(env.database, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:         agentType,
		ProjectID:  env.projectID,
		WorkflowID: env.workflowID,
		Model:      "sonnet-5",
		Timeout:    3600,
		Prompt:     "Main prompt body",
		PromptMode: service.PromptModeStepwise,
		Steps:      &stepsJSON,
	}); err != nil {
		t.Fatalf("createStepwiseAgentDefInTestEnv: %v", err)
	}
}

// createFullModeAgentDefInTestEnv inserts a plain (non-stepwise) agent def.
func createFullModeAgentDefInTestEnv(t *testing.T, env *testEnv, agentType string) {
	t.Helper()
	adRepo := repo.NewAgentDefinitionRepo(env.database, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:         agentType,
		ProjectID:  env.projectID,
		WorkflowID: env.workflowID,
		Model:      "sonnet-5",
		Timeout:    3600,
		Prompt:     "Main prompt body",
	}); err != nil {
		t.Fatalf("createFullModeAgentDefInTestEnv: %v", err)
	}
}

// insertStepCursor writes an agent_step_cursors row directly (no engine),
// so guard tests can pin exact current_index/completed state.
func insertStepCursor(t *testing.T, env *testEnv, nodeID string, steps []model.StepDefinition, currentIndex int, completed []model.CompletedStep) {
	t.Helper()
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal steps: %v", err)
	}
	completedJSON, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal completed: %v", err)
	}
	cursorRepo := repo.NewAgentStepCursorRepo(env.database, clock.Real())
	if err := cursorRepo.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: env.wfiID,
		NodeID:             nodeID,
		StepsSnapshot:      string(stepsJSON),
		Revision:           1,
		CurrentIndex:       currentIndex,
		Completed:          string(completedJSON),
		Rejections:         "{}",
	}); err != nil {
		t.Fatalf("insertStepCursor: %v", err)
	}
}

// stepwiseCompletionProc builds a processInfo for handleCompletion tests,
// with nodeID set (the completion guard's cursor lookup requires it — see
// validationProc, which deliberately leaves nodeID empty for the
// non-stepwise tests it serves).
func stepwiseCompletionProc(env *testEnv, agentType string, cmds []string) *processInfo {
	return &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet-5",
		agentType:          agentType,
		nodeID:             agentType,
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-5 * time.Second),
		validationCommands: cmds,
		env:                []string{"NRFLO_PROJECT=" + env.projectID},
	}
}

func runTrue(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run true: %v", err)
	}
	return cmd
}

// getStepsIncompleteFinding returns the parsed steps_incomplete payload, or nil.
func getStepsIncompleteFinding(t *testing.T, env *testEnv, sessionID string) map[string]interface{} {
	t.Helper()
	findingRepo := repo.NewFindingRepo(env.database, clock.Real())
	m, err := findingRepo.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("GetOwn(%q): %v", sessionID, err)
	}
	raw, ok := m[service.FindingKeyStepsIncomplete]
	if !ok {
		return nil
	}
	var p map[string]interface{}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal steps_incomplete: %v", err)
	}
	return p
}

// TestHandleCompletion_StepwiseIncomplete_ForceFailsWithFinding is case 1:
// implicit pass with the cursor short of its last step must flip to
// fail/steps_incomplete and write the finding.
func TestHandleCompletion_StepwiseIncomplete_ForceFailsWithFinding(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	createStepwiseAgentDefInTestEnv(t, env, "test-agent", threeSteps())
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
	if !s.Result.Valid || s.Result.String != "fail" {
		t.Errorf("result = %v, want fail", s.Result)
	}
	if !s.ResultReason.Valid || s.ResultReason.String != service.ResultReasonStepsIncomplete {
		t.Errorf("result_reason = %v, want %q", s.ResultReason, service.ResultReasonStepsIncomplete)
	}
	if proc.finalStatus != "FAIL" {
		t.Errorf("finalStatus = %q, want FAIL", proc.finalStatus)
	}

	p := getStepsIncompleteFinding(t, env, env.sessionID)
	if p == nil {
		t.Fatal("expected steps_incomplete finding, got none")
	}
	if p["step_id"] != "step-one" {
		t.Errorf("step_id = %v, want step-one", p["step_id"])
	}
	if p["step_index"] != float64(1) {
		t.Errorf("step_index = %v, want 1", p["step_index"])
	}
	if p["total"] != float64(3) {
		t.Errorf("total = %v, want 3", p["total"])
	}
}

// TestHandleCompletion_StepwiseComplete_ValidationStillRuns is case 2: cursor
// past the last step means the guard is a no-op, but whole-agent
// validation_commands still runs (proving the guard did not swallow it).
func TestHandleCompletion_StepwiseComplete_ValidationStillRuns(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	createStepwiseAgentDefInTestEnv(t, env, "test-agent", threeSteps())
	insertStepCursor(t, env, "test-agent", threeSteps(), 3, nil)

	proc := stepwiseCompletionProc(env, "test-agent", []string{"false"})
	proc.cmd = runTrue(t)

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: env.workflowID, AgentType: "test-agent",
	})

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	s, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if !s.Result.Valid || s.Result.String != "fail" {
		t.Errorf("result = %v, want fail", s.Result)
	}
	if !s.ResultReason.Valid || s.ResultReason.String != "validation_failure" {
		t.Errorf("result_reason = %v, want validation_failure (guard must not have already flipped it)", s.ResultReason)
	}
	if p := getStepsIncompleteFinding(t, env, env.sessionID); p != nil {
		t.Error("unexpected steps_incomplete finding when cursor is past the last step")
	}
}
