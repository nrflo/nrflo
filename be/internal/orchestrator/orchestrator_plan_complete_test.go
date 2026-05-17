package orchestrator

import (
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// TestHandlePlanModePostStep_MarksSessionInteractiveCompleted verifies that
// handlePlanModePostStep sets the planner session to interactive_completed/pass
// with ended_at populated.
func TestHandlePlanModePostStep_MarksSessionInteractiveCompleted(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PIC-1", "planner interactive completed test")
	wfiID := env.initWorkflow(t, "TKT-PIC-1")

	sessionID := "planner-session-pic-1"
	projectRoot := "/test/project/pic"
	planContent := "# Plan\n\nStep 1: Analyze\nStep 2: Implement"

	now := clock.Real().Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, result_reason, pid, context_left, ancestor_session_id,
			spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-project', '', ?, 'planning', 'planner', 'user_interactive',
			NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		sessionID, wfiID, now, now, now)
	if err != nil {
		t.Fatalf("insert planner session: %v", err)
	}

	setupPlanModeHome(t, sessionID, projectRoot, planContent)

	if err := handlePlanModePostStep(sessionID, projectRoot, env.pool, wfiID, clock.Real()); err != nil {
		t.Fatalf("handlePlanModePostStep() error: %v", err)
	}

	session, err := repo.NewAgentSessionRepo(env.pool, clock.Real()).Get(sessionID)
	if err != nil {
		t.Fatalf("get session after handlePlanModePostStep: %v", err)
	}
	if string(session.Status) != "interactive_completed" {
		t.Errorf("session status = %q, want 'interactive_completed'", session.Status)
	}
	if !session.Result.Valid || session.Result.String != "pass" {
		t.Errorf("session result = %v, want 'pass'", session.Result)
	}
	if !session.EndedAt.Valid || session.EndedAt.String == "" {
		t.Error("session ended_at must be set after interactive_completed")
	}
}

// TestHandlePlanModePostStep_MissingSession_ReturnsError verifies that
// UpdateStatusToInteractiveCompleted errors when the session ID does not exist.
func TestHandlePlanModePostStep_MissingSession_ReturnsError(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PIC-2", "missing session test")
	wfiID := env.initWorkflow(t, "TKT-PIC-2")

	sessionID := "nonexistent-session-pic-2"
	projectRoot := "/test/project/pic2"
	planContent := "# Plan\n\nSome plan content"

	setupPlanModeHome(t, sessionID, projectRoot, planContent)

	err := handlePlanModePostStep(sessionID, projectRoot, env.pool, wfiID, clock.Real())
	if err == nil {
		t.Fatal("handlePlanModePostStep() should return error when session does not exist")
	}
}
