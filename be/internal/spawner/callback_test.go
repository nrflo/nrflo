package spawner

import (
	"os/exec"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
)

// TestHandleCompletion_ExitZeroWithExplicitCallback tests that explicit callback
// is detected during the grace period
func TestHandleCompletion_ExitZeroWithExplicitCallback(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	// Simulate explicit callback by setting result before handleCompletion
	sessionRepo := repo.NewAgentSessionRepo(env.database)
	if err := sessionRepo.UpdateResult(env.sessionID, "callback", "explicit"); err != nil {
		t.Fatalf("failed to update result: %v", err)
	}

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run test command: %v", err)
	}

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-5 * time.Second),
	}

	env.spawner.handleCompletion(proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	// Verify result in database
	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if !updatedSession.Result.Valid || updatedSession.Result.String != "callback" {
		t.Errorf("expected result='callback', got %v", updatedSession.Result)
	}

	if !updatedSession.ResultReason.Valid || updatedSession.ResultReason.String != "explicit" {
		t.Errorf("expected result_reason='explicit', got %v", updatedSession.ResultReason)
	}

	if updatedSession.Status != model.AgentSessionCallback {
		t.Errorf("expected status='callback', got %v", updatedSession.Status)
	}

	if proc.finalStatus != "CALLBACK" {
		t.Errorf("expected finalStatus='CALLBACK', got %v", proc.finalStatus)
	}
}

// TestHandleCompletion_CallbackStatusMapping tests that callback result
// is correctly mapped to callback status in registerAgentStopWithReason
func TestHandleCompletion_CallbackStatusMapping(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	// Simulate explicit callback
	sessionRepo := repo.NewAgentSessionRepo(env.database)
	if err := sessionRepo.UpdateResult(env.sessionID, "callback", "explicit"); err != nil {
		t.Fatalf("failed to update result: %v", err)
	}

	// Call registerAgentStopWithReason directly to test status mapping
	env.spawner.registerAgentStopWithReason(
		env.projectID,
		env.ticketID,
		env.workflowID,
		env.sessionID,
		"test-agent-id",
		"callback",
		"explicit",
		"claude:opus",
	)

	// Verify status was correctly mapped to callback
	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if updatedSession.Status != model.AgentSessionCallback {
		t.Errorf("expected status='callback', got %v", updatedSession.Status)
	}

	if !updatedSession.Result.Valid || updatedSession.Result.String != "callback" {
		t.Errorf("expected result='callback', got %v", updatedSession.Result)
	}

	if !updatedSession.ResultReason.Valid || updatedSession.ResultReason.String != "explicit" {
		t.Errorf("expected result_reason='explicit', got %v", updatedSession.ResultReason)
	}

	// Verify ended_at was set
	if !updatedSession.EndedAt.Valid {
		t.Errorf("expected ended_at to be set")
	}
}

// TestHandleCompletion_CallbackWithOtherResults verifies callback doesn't
// interfere with other result types
func TestHandleCompletion_CallbackWithOtherResults(t *testing.T) {
	testCases := []struct {
		name           string
		result         string
		resultReason   string
		expectedStatus model.AgentSessionStatus
		expectedFinal  string
	}{
		{"Pass", "pass", "explicit", model.AgentSessionCompleted, "PASS"},
		{"Fail", "fail", "explicit", model.AgentSessionFailed, "FAIL"},
		{"Continue", "continue", "explicit", model.AgentSessionContinued, "CONTINUE"},
		{"Callback", "callback", "explicit", model.AgentSessionCallback, "CALLBACK"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.cleanup()

			env.createSession(t, "claude:sonnet")

			sessionRepo := repo.NewAgentSessionRepo(env.database)
			if err := sessionRepo.UpdateResult(env.sessionID, tc.result, tc.resultReason); err != nil {
				t.Fatalf("failed to update result: %v", err)
			}

			cmd := exec.Command("true")
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to run test command: %v", err)
			}

			proc := &processInfo{
				cmd:                cmd,
				sessionID:          env.sessionID,
				agentID:            "test-agent-id",
				modelID:            "claude:sonnet",
				workflowInstanceID: env.wfiID,
				projectID:          env.projectID,
				ticketID:           env.ticketID,
				workflowName:       env.workflowID,
				startTime:          time.Now().Add(-5 * time.Second),
			}

			env.spawner.handleCompletion(proc, SpawnRequest{
				ProjectID:    env.projectID,
				TicketID:     env.ticketID,
				WorkflowName: env.workflowID,
				AgentType:    "test-agent",
			})

			// Verify status mapping
			updatedSession, err := sessionRepo.Get(env.sessionID)
			if err != nil {
				t.Fatalf("failed to get updated session: %v", err)
			}

			if updatedSession.Status != tc.expectedStatus {
				t.Errorf("expected status='%v', got '%v'", tc.expectedStatus, updatedSession.Status)
			}

			if proc.finalStatus != tc.expectedFinal {
				t.Errorf("expected finalStatus='%v', got '%v'", tc.expectedFinal, proc.finalStatus)
			}
		})
	}
}

// TestHandleCompletion_CallbackGetAgentResult tests that getAgentResult
// correctly returns "callback" when the session has callback result
func TestHandleCompletion_CallbackGetAgentResult(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")

	// Set result to callback
	sessionRepo := repo.NewAgentSessionRepo(env.database)
	if err := sessionRepo.UpdateResult(env.sessionID, "callback", "explicit"); err != nil {
		t.Fatalf("failed to update result: %v", err)
	}

	proc := &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:haiku",
		workflowInstanceID: env.wfiID,
	}

	// Get result via spawner's getAgentResult
	result := env.spawner.getAgentResult(proc)

	if result != "callback" {
		t.Errorf("expected getAgentResult to return 'callback', got '%v'", result)
	}
}
