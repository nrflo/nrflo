package spawner

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"

	"github.com/google/uuid"
)

// validationProc returns a processInfo with validation commands set, attached to env.
func validationProc(env *testEnv, cmds []string) *processInfo {
	return &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet",
		agentType:          "test-agent",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-5 * time.Second),
		validationCommands: cmds,
		env:                []string{"NRFLO_PROJECT=" + env.projectID},
	}
}

// getValidationFinding returns the parsed validation_failure payload for a session, or nil.
func getValidationFinding(t *testing.T, env *testEnv, sessionID string) map[string]interface{} {
	t.Helper()
	findingRepo := repo.NewFindingRepo(env.database, clock.Real())
	m, err := findingRepo.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("GetOwn(%q): %v", sessionID, err)
	}
	raw, ok := m["validation_failure"]
	if !ok {
		return nil
	}
	var p map[string]interface{}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal validation_failure: %v", err)
	}
	return p
}

// TestWriteValidationFailureFinding_PersistsToDB verifies the finding is stored with
// the correct key, scope, and payload fields.
func TestWriteValidationFailureFinding_PersistsToDB(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	proc := validationProc(env, []string{"false", "echo ignored"})
	env.spawner.writeValidationFailureFinding(proc, 0, 1, "some output here")

	p := getValidationFinding(t, env, env.sessionID)
	if p == nil {
		t.Fatal("expected validation_failure finding, got none")
	}
	if p["command"] != "false" {
		t.Errorf("command = %v, want \"false\"", p["command"])
	}
	if p["command_index"] != float64(0) {
		t.Errorf("command_index = %v, want 0", p["command_index"])
	}
	if p["exit_code"] != float64(1) {
		t.Errorf("exit_code = %v, want 1", p["exit_code"])
	}
	if p["output_tail"] != "some output here" {
		t.Errorf("output_tail = %v, want \"some output here\"", p["output_tail"])
	}
}

// TestHandleCompletion_ValidationCommandsPass_ResultPreserved verifies that when all
// validation commands pass the agent result remains "pass".
func TestHandleCompletion_ValidationCommandsPass_ResultPreserved(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("pre-run true: %v", err)
	}
	proc := validationProc(env, []string{"true", "true"})
	proc.cmd = cmd

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	s, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if !s.Result.Valid || s.Result.String != "pass" {
		t.Errorf("result = %v, want pass", s.Result)
	}
	if !s.ResultReason.Valid || s.ResultReason.String != "implicit" {
		t.Errorf("result_reason = %v, want implicit", s.ResultReason)
	}
	if proc.finalStatus != "PASS" {
		t.Errorf("finalStatus = %q, want PASS", proc.finalStatus)
	}
	if p := getValidationFinding(t, env, env.sessionID); p != nil {
		t.Error("unexpected validation_failure finding when commands all pass")
	}
}

// TestHandleCompletion_ValidationCommandsFail_FlipsToFail verifies that a failing
// validation command overrides pass → fail with reason=validation_failure.
func TestHandleCompletion_ValidationCommandsFail_FlipsToFail(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("pre-run true: %v", err)
	}
	proc := validationProc(env, []string{"false"})
	proc.cmd = cmd

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
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
		t.Errorf("result_reason = %v, want validation_failure", s.ResultReason)
	}
	if proc.finalStatus != "FAIL" {
		t.Errorf("finalStatus = %q, want FAIL", proc.finalStatus)
	}
	// Finding must be persisted.
	if p := getValidationFinding(t, env, env.sessionID); p == nil {
		t.Error("expected validation_failure finding in DB, got none")
	}
}

// TestHandleCompletion_ExplicitFail_SkipsValidation verifies that when the agent
// already has an explicit fail result, validation commands are NOT executed.
func TestHandleCompletion_ExplicitFail_SkipsValidation(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.UpdateResult(env.sessionID, "fail", "explicit"); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("pre-run true: %v", err)
	}
	proc := validationProc(env, []string{"false"}) // would flip if run
	proc.cmd = cmd

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	s, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if !s.ResultReason.Valid || s.ResultReason.String != "explicit" {
		t.Errorf("result_reason = %v, want explicit (not validation_failure)", s.ResultReason)
	}
	if p := getValidationFinding(t, env, env.sessionID); p != nil {
		t.Error("validation_failure finding should not exist when explicit fail was pre-set")
	}
}

// TestCopyFindingsForContinuation_CarriesValidationFailure verifies that a
// validation_failure finding on the old session is carried to the new session.
func TestCopyFindingsForContinuation_CarriesValidationFailure(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	oldID := uuid.New().String()
	newID := uuid.New().String()

	env.createNamedSession(t, oldID, "claude:sonnet")
	env.createNamedSession(t, newID, "claude:sonnet")

	// Plant a validation_failure finding on the old session.
	payload := json.RawMessage(`{"command":"false","command_index":0,"exit_code":1,"output_tail":"oops"}`)
	findingRepo := repo.NewFindingRepo(env.database, clock.Real())
	denorm := repo.Denorm{
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
		AgentType:          "test-agent",
		ModelID:            "claude:sonnet",
	}
	if err := findingRepo.Upsert("session", oldID, "validation_failure", payload, denorm,
		repo.Actor{Source: "system", ID: "validation"}); err != nil {
		t.Fatalf("Upsert old finding: %v", err)
	}

	env.spawner.copyFindingsForContinuation(context.Background(), oldID, newID)

	// validation_failure must appear on new session.
	findingRepo2 := repo.NewFindingRepo(env.database, clock.Real())
	m, err := findingRepo2.GetOwn("session", newID)
	if err != nil {
		t.Fatalf("GetOwn new session: %v", err)
	}
	raw, ok := m["validation_failure"]
	if !ok {
		t.Fatal("expected validation_failure on new session after carryover, got none")
	}
	var p map[string]interface{}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal carried finding: %v", err)
	}
	if p["command"] != "false" {
		t.Errorf("carried finding command = %v, want false", p["command"])
	}
	if p["exit_code"] != float64(1) {
		t.Errorf("carried finding exit_code = %v, want 1", p["exit_code"])
	}
}
