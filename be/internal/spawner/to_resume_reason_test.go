package spawner

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

func TestFetchPreviousDataAndReason_WithDataAndReason(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	findings := map[string]interface{}{"to_resume": "saved progress"}
	createContinuedSessionWithReason(t, env, sessionID, findings, "low_context")

	data, reason := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet", "test-phase", "")
	if data != "saved progress" {
		t.Errorf("data = %q, want %q", data, "saved progress")
	}
	if reason != "low_context" {
		t.Errorf("reason = %q, want %q", reason, "low_context")
	}
}

func TestFetchPreviousDataAndReason_NoDataWithReason(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	findings := map[string]interface{}{"other_key": "value"}
	createContinuedSessionWithReason(t, env, sessionID, findings, "instant_stall")

	data, reason := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet", "test-phase", "")
	if data != "" {
		t.Errorf("data = %q, want empty", data)
	}
	if reason != "instant_stall" {
		t.Errorf("reason = %q, want %q", reason, "instant_stall")
	}
}

func TestFetchPreviousDataAndReason_NoContinuedSessionReturnsEmptyReason(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	data, reason := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet", "test-phase", "")
	if data != "" {
		t.Errorf("data = %q, want empty", data)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

func TestFetchPreviousDataAndReason_NullReason(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	findings := map[string]interface{}{"to_resume": "progress data"}
	env.createContinuedSession(t, sessionID, findings)

	data, reason := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet", "test-phase", "")
	if data != "progress data" {
		t.Errorf("data = %q, want %q", data, "progress data")
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty for NULL result_reason", reason)
	}
}

func TestFetchPreviousDataAndReason_AllStallReasons(t *testing.T) {
	reasons := []string{
		"stall_restart_start_stall",
		"stall_restart_running_stall",
		"instant_stall",
		"fail_restart",
		"timeout_restart",
	}
	for _, r := range reasons {
		t.Run(r, func(t *testing.T) {
			env := setupToResumeTestEnv(t)
			defer env.cleanup()

			sessionID := uuid.New().String()
			createContinuedSessionWithReason(t, env, sessionID,
				map[string]interface{}{}, r)

			_, reason := env.spawner.fetchPreviousDataAndReason(
				env.projectID, env.ticketID, env.workflowID,
				"test-agent", "claude:sonnet", "test-phase", "")
			if reason != r {
				t.Errorf("reason = %q, want %q", reason, r)
			}
		})
	}
}

func createContinuedSessionWithReason(t *testing.T, env *toResumeTestEnv, sessionID string, findings map[string]interface{}, resultReason string) {
	t.Helper()
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("failed to marshal findings: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "claude:sonnet", Valid: true},
		Status:             model.AgentSessionContinued,
		Result:             sql.NullString{String: "continue", Valid: true},
		ResultReason:       sql.NullString{String: resultReason, Valid: resultReason != ""},
		Findings:           sql.NullString{String: string(findingsJSON), Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
		EndedAt:            sql.NullString{String: now, Valid: true},
	}
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create continued session: %v", err)
	}
}
