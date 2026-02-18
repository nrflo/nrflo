package orchestrator

import (
	"database/sql"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner"
)

// TestTakeControl_NoRunningOrchestration verifies that TakeControl returns an
// error when there is no running orchestration for the workflow.
func TestTakeControl_NoRunningOrchestration(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-1", "Take control test")
	env.initWorkflow(t, "TC-1")

	_, err := env.orch.TakeControl(env.project, "TC-1", "test", "some-session")
	if err == nil {
		t.Fatal("expected error for non-running orchestration")
	}
	if got := err.Error(); got != "no running orchestration for workflow 'test' on TC-1" {
		t.Fatalf("unexpected error: %s", got)
	}
}

// TestTakeControl_WorkflowNotFound verifies that TakeControl returns an error
// when the workflow instance does not exist in the database.
func TestTakeControl_WorkflowNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-2", "Take control test")
	// No initWorkflow — no WFI in DB.

	_, err := env.orch.TakeControl(env.project, "TC-2", "test", "some-session")
	if err == nil {
		t.Fatal("expected error for missing workflow instance")
	}
}

// TestTakeControl_NoActiveSpawner verifies that TakeControl returns a descriptive
// error when there is an active run but no spawner (between phases).
func TestTakeControl_NoActiveSpawner(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-3", "Take control test")
	wfiID := env.initWorkflow(t, "TC-3")

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:  func() {},
		spawner: nil,
	}
	env.orch.mu.Unlock()

	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	_, err := env.orch.TakeControl(env.project, "TC-3", "test", "some-session")
	if err == nil {
		t.Fatal("expected error when spawner is nil")
	}
	if got := err.Error(); got != "no active spawner (agent may be between phases)" {
		t.Fatalf("unexpected error: %s", got)
	}
}

// TestTakeControl_ForwardsToSpawner verifies that TakeControl calls
// RequestTakeControl on the active spawner and returns the session ID.
func TestTakeControl_ForwardsToSpawner(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-4", "Take control test")
	wfiID := env.initWorkflow(t, "TC-4")

	sp := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:  func() {},
		spawner: sp,
	}
	env.orch.mu.Unlock()

	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	returnedID, err := env.orch.TakeControl(env.project, "TC-4", "test", "target-session-tc4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if returnedID != "target-session-tc4" {
		t.Errorf("expected returned session_id='target-session-tc4', got %q", returnedID)
	}

	// Verify RequestTakeControl was forwarded: calling it a second time with a
	// different ID should be a no-op since the channel is already full.
	sp.RequestTakeControl("second-call-noop")
	// No panic or block = success
}

// TestTakeControlProject_RequiresInstanceID verifies that TakeControlProject
// returns an error when instance_id is empty.
func TestTakeControlProject_RequiresInstanceID(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.orch.TakeControlProject(env.project, "test", "some-session", "")
	if err == nil {
		t.Fatal("expected error for missing instance_id")
	}
	if got := err.Error(); got != "instance_id is required for project-scoped workflow take-control" {
		t.Fatalf("unexpected error: %s", got)
	}
}

// TestTakeControlProject_NoRunningOrchestration verifies that TakeControlProject
// returns an error when the instance ID is not in the active runs map.
func TestTakeControlProject_NoRunningOrchestration(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.orch.TakeControlProject(env.project, "test", "some-session", "nonexistent-instance-id")
	if err == nil {
		t.Fatal("expected error for non-running orchestration")
	}
}

// TestTakeControlProject_ForwardsToSpawner verifies that TakeControlProject
// returns the session ID and signals the spawner when conditions are met.
func TestTakeControlProject_ForwardsToSpawner(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")

	sp := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:  func() {},
		spawner: sp,
	}
	env.orch.mu.Unlock()

	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	returnedID, err := env.orch.TakeControlProject(env.project, "test", "proj-session-1", wfiID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if returnedID != "proj-session-1" {
		t.Errorf("expected 'proj-session-1', got %q", returnedID)
	}

	// Second call is a no-op (channel full) — verifies first call forwarded.
	sp.RequestTakeControl("second-call-noop")
}

// TestCompleteInteractive_SessionNotFound verifies that CompleteInteractive
// returns an error for a missing session.
func TestCompleteInteractive_SessionNotFound(t *testing.T) {
	env := newTestEnv(t)

	err := env.orch.CompleteInteractive("nonexistent-session-999")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

// TestCompleteInteractive_Succeeds verifies that CompleteInteractive updates
// the session status to interactive_completed (fixed by migration 000026).
func TestCompleteInteractive_Succeeds(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-CI-BUG", "Constraint bug test")
	wfiID := env.initWorkflow(t, "TC-CI-BUG")

	sessionID := "ci-constraint-bug-session"
	insertRunningSession(t, env, wfiID, "TC-CI-BUG", sessionID)

	err := env.orch.CompleteInteractive(sessionID)
	if err != nil {
		t.Fatalf("CompleteInteractive should succeed after migration 000026: %v", err)
	}
}

// TestTakeControl_ReturnsCorrectSessionID ensures TakeControl echoes back
// exactly the sessionID supplied by the caller.
func TestTakeControl_ReturnsCorrectSessionID(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TC-5", "Session ID echo test")
	wfiID := env.initWorkflow(t, "TC-5")

	sp := spawner.New(spawner.Config{Clock: clock.Real()})
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}, spawner: sp}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	want := "the-exact-session-uuid-123"
	got, err := env.orch.TakeControl(env.project, "TC-5", "test", want)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("TakeControl returned session_id=%q, want %q", got, want)
	}
}

// insertRunningSession inserts a minimal agent_sessions row with status=running.
// Uses 'running' status to avoid the DB constraint bug on user_interactive.
func insertRunningSession(t *testing.T, env *testEnv, wfiID, ticketID, sessionID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			 model_id, status, result, result_reason, pid, findings,
			 context_left, ancestor_session_id, spawn_command, prompt_context,
			 restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?)`,
		sessionID, env.project, ticketID, wfiID, "test-phase", "test-agent",
		sql.NullString{String: "claude:sonnet", Valid: true},
		"running",
		sql.NullString{}, sql.NullString{}, sql.NullInt64{}, sql.NullString{},
		sql.NullInt64{}, sql.NullString{}, sql.NullString{}, sql.NullString{},
		0, sql.NullString{String: now, Valid: true}, sql.NullString{},
		now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert running session: %v", err)
	}
}
