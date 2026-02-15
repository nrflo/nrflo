package spawner

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// mustMarshal marshals data to JSON or fails the test
func mustMarshal(t *testing.T, data interface{}) string {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return string(b)
}

// createSessionWithID creates a test session with a specific ID
func (env *testEnv) createSessionWithID(t *testing.T, sessionID, modelID string) string {
	t.Helper()

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		AgentType:          "test-agent",
		Status:             model.AgentSessionRunning,
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	return sessionID
}

// TestFinalizePhase_CallbackDetection tests that finalizePhase returns CallbackError
// when an agent completes with CALLBACK status.
func TestFinalizePhase_CallbackDetection(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	// Set session result to callback with callback_level and callback_instructions findings
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	findings := map[string]interface{}{
		"callback_level":        1,
		"callback_instructions": "Fix the implementation bug",
	}
	findingsJSON := mustMarshal(t, findings)
	sessionRepo.UpdateFindings(env.sessionID, findingsJSON)
	sessionRepo.UpdateResult(env.sessionID, "callback", "explicit")

	// Create completed process info with CALLBACK finalStatus
	cmd := exec.Command("true")
	cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now(),
		finalStatus:        "CALLBACK",
		elapsed:            5 * time.Second,
	}

	// Call finalizePhase
	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase")

	// Verify CallbackError is returned
	var cbErr *CallbackError
	if !errors.As(err, &cbErr) {
		t.Fatalf("expected CallbackError, got %T: %v", err, err)
	}

	if cbErr.Level != 1 {
		t.Errorf("expected callback level=1, got %d", cbErr.Level)
	}
	if cbErr.Instructions != "Fix the implementation bug" {
		t.Errorf("expected instructions='Fix the implementation bug', got '%s'", cbErr.Instructions)
	}
	if cbErr.AgentType != "test-agent" {
		t.Errorf("expected agent_type='test-agent', got '%s'", cbErr.AgentType)
	}
}

// TestFinalizePhase_CallbackLevelZero tests callback to level 0 (valid edge case)
func TestFinalizePhase_CallbackLevelZero(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	findings := map[string]interface{}{
		"callback_level":        0,
		"callback_instructions": "Restart from beginning",
	}
	findingsJSON := mustMarshal(t, findings)
	sessionRepo.UpdateFindings(env.sessionID, findingsJSON)
	sessionRepo.UpdateResult(env.sessionID, "callback", "explicit")

	cmd := exec.Command("true")
	cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent",
		modelID:            "claude:opus",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "CALLBACK",
		elapsed:            3 * time.Second,
	}

	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase")

	var cbErr *CallbackError
	if !errors.As(err, &cbErr) {
		t.Fatalf("expected CallbackError, got %T: %v", err, err)
	}

	if cbErr.Level != 0 {
		t.Errorf("expected callback level=0, got %d", cbErr.Level)
	}
}

// TestFinalizePhase_CallbackWithMissingInstructions tests callback with missing callback_instructions
func TestFinalizePhase_CallbackWithMissingInstructions(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	findings := map[string]interface{}{
		"callback_level": 2,
		// No callback_instructions
	}
	findingsJSON := mustMarshal(t, findings)
	sessionRepo.UpdateFindings(env.sessionID, findingsJSON)
	sessionRepo.UpdateResult(env.sessionID, "callback", "explicit")

	cmd := exec.Command("true")
	cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent",
		modelID:            "claude:haiku",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "CALLBACK",
		elapsed:            2 * time.Second,
	}

	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase")

	var cbErr *CallbackError
	if !errors.As(err, &cbErr) {
		t.Fatalf("expected CallbackError, got %T: %v", err, err)
	}

	// Should return empty string for missing instructions
	if cbErr.Instructions != "" {
		t.Errorf("expected empty instructions, got '%s'", cbErr.Instructions)
	}
}

// TestFinalizePhase_PassTakesPrecedenceOverCallback tests that when both PASS and CALLBACK
// are present, PASS count >= 1 means success (callback detection happens later)
func TestFinalizePhase_PassTakesPrecedenceOverCallback(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Create two sessions: one PASS, one CALLBACK
	sess1 := env.createSessionWithID(t, "sess-pass", "claude:sonnet")
	sess2 := env.createSessionWithID(t, "sess-callback", "claude:opus")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())

	// Set first session to PASS
	sessionRepo.UpdateResult(sess1, "pass", "explicit")

	// Set second session to CALLBACK with findings
	findings := map[string]interface{}{
		"callback_level":        1,
		"callback_instructions": "Callback from multi-agent layer",
	}
	findingsJSON := mustMarshal(t, findings)
	sessionRepo.UpdateFindings(sess2, findingsJSON)
	sessionRepo.UpdateResult(sess2, "callback", "explicit")

	cmd1 := exec.Command("true")
	cmd1.Run()
	cmd2 := exec.Command("true")
	cmd2.Run()

	proc1 := &processInfo{
		cmd:                cmd1,
		sessionID:          sess1,
		agentID:            "agent-pass",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "PASS",
		elapsed:            4 * time.Second,
	}

	proc2 := &processInfo{
		cmd:                cmd2,
		sessionID:          sess2,
		agentID:            "agent-callback",
		modelID:            "claude:opus",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "CALLBACK",
		elapsed:            3 * time.Second,
	}

	// Call finalizePhase with both procs
	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc1, proc2}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "multi-agent-phase",
	}, "multi-phase")

	// Should return CallbackError (callback is checked BEFORE pass count in finalizePhase)
	var cbErr *CallbackError
	if !errors.As(err, &cbErr) {
		t.Fatalf("expected CallbackError when callback is present, got %T: %v", err, err)
	}

	if cbErr.Level != 1 {
		t.Errorf("expected callback level=1, got %d", cbErr.Level)
	}
}

// TestFinalizePhase_NoCallback_Pass tests normal pass flow (no callback)
func TestFinalizePhase_NoCallback_Pass(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sessionRepo.UpdateResult(env.sessionID, "pass", "explicit")

	cmd := exec.Command("true")
	cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "PASS",
		elapsed:            5 * time.Second,
	}

	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase")

	// Should return nil (success)
	if err != nil {
		t.Errorf("expected no error for PASS, got: %v", err)
	}
}

// TestFinalizePhase_NoCallback_Fail tests normal fail flow (no callback)
func TestFinalizePhase_NoCallback_Fail(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sessionRepo.UpdateResult(env.sessionID, "fail", "explicit")

	cmd := exec.Command("false")
	cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent",
		modelID:            "claude:opus",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "FAIL",
		elapsed:            2 * time.Second,
	}

	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase")

	// Should return error (phase failed)
	if err == nil {
		t.Error("expected error for FAIL, got nil")
	}

	// Should NOT be a CallbackError
	var cbErr *CallbackError
	if errors.As(err, &cbErr) {
		t.Error("expected regular error for FAIL, got CallbackError")
	}
}

// TestFinalizePhase_AllSkipped tests all-skipped flow (no callback)
func TestFinalizePhase_AllSkipped(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sessionRepo.UpdateResult(env.sessionID, "skip", "explicit")

	cmd := exec.Command("true")
	cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent",
		modelID:            "claude:haiku",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "SKIPPED",
		elapsed:            1 * time.Second,
	}

	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase")

	// Should return nil (success - all skipped counts as success)
	if err != nil {
		t.Errorf("expected no error for SKIPPED, got: %v", err)
	}
}

// TestFinalizePhase_CallbackLevelFloat tests callback_level as float64 (JSON unmarshal behavior)
func TestFinalizePhase_CallbackLevelFloat(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	// JSON numbers unmarshal to float64
	findings := map[string]interface{}{
		"callback_level":        float64(3),
		"callback_instructions": "Float level test",
	}
	findingsJSON := mustMarshal(t, findings)
	sessionRepo.UpdateFindings(env.sessionID, findingsJSON)
	sessionRepo.UpdateResult(env.sessionID, "callback", "explicit")

	cmd := exec.Command("true")
	cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		finalStatus:        "CALLBACK",
		elapsed:            2 * time.Second,
	}

	err := env.spawner.finalizePhase(context.Background(), []*processInfo{proc}, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, "test-phase")

	var cbErr *CallbackError
	if !errors.As(err, &cbErr) {
		t.Fatalf("expected CallbackError, got %T: %v", err, err)
	}

	if cbErr.Level != 3 {
		t.Errorf("expected callback level=3 (from float64), got %d", cbErr.Level)
	}
}
