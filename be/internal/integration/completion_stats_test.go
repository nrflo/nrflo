package integration

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestCompletedWorkflowStats is an end-to-end test verifying all three
// completion stats: completed_at, total_duration_sec, and total_tokens_used.
// These correspond to the ticket acceptance criteria:
//   1. Date/Time of completion
//   2. Total time spent on workflow
//   3. Total context consumption (200K token window * (100 - context_left)/100)
func TestCompletedWorkflowStats(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CS-1", "Completion stats")
	env.InitWorkflow(t, "CS-1")

	wfiID := env.GetWorkflowInstanceID(t, "CS-1", "test")

	// Insert completed agent sessions with various context_left values:
	//   context_left=60 → 200000*(100-60)/100 = 80000 tokens used
	//   context_left=25 → 200000*(100-25)/100 = 150000 tokens used
	//   Total expected: 230000
	insertSessionWithContextLeft(t, env, "cs-sess-1", "CS-1", wfiID,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 60)
	insertSessionWithContextLeft(t, env, "cs-sess-2", "CS-1", wfiID,
		"builder", "implementor", "claude:opus", "completed", "pass", 25)

	// Complete both phases
	env.StartPhase(t, "CS-1", "analyzer")
	env.CompletePhase(t, "CS-1", "analyzer", "pass")
	env.StartPhase(t, "CS-1", "builder")
	env.CompletePhase(t, "CS-1", "builder", "pass")

	// Mark the workflow instance as completed (simulating orchestrator behavior)
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	if err := wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("failed to set workflow completed: %v", err)
	}

	// Get status
	status, err := getWorkflowStatus(t, env, "CS-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	// 1. Verify status field
	if status["status"] != "completed" {
		t.Fatalf("expected status 'completed', got %v", status["status"])
	}

	// 2. Verify completed_at is present and non-empty
	completedAt, ok := status["completed_at"].(string)
	if !ok || completedAt == "" {
		t.Fatalf("expected non-empty completed_at string, got %v", status["completed_at"])
	}

	// 3. Verify total_duration_sec is present and >= 0
	durationSec, ok := status["total_duration_sec"].(float64)
	if !ok {
		t.Fatalf("expected total_duration_sec to be float64, got %T (%v)", status["total_duration_sec"], status["total_duration_sec"])
	}
	if durationSec < 0 {
		t.Fatalf("expected non-negative duration, got %v", durationSec)
	}

	// 4. Verify total_tokens_used = 80000 + 150000 = 230000
	tokensUsed, ok := status["total_tokens_used"].(float64)
	if !ok {
		t.Fatalf("expected total_tokens_used to be float64, got %T (%v)", status["total_tokens_used"], status["total_tokens_used"])
	}
	if int(tokensUsed) != 230000 {
		t.Fatalf("expected total_tokens_used 230000, got %v", tokensUsed)
	}
}

// TestCompletedWorkflowStatsAbsentWhenActive verifies that completion stats
// (completed_at, total_duration_sec, total_tokens_used) are absent when the
// workflow status is "active" (not completed).
func TestCompletedWorkflowStatsAbsentWhenActive(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CS-2", "Active workflow - no completion stats")
	env.InitWorkflow(t, "CS-2")

	status, err := getWorkflowStatus(t, env, "CS-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	if status["status"] != "active" {
		t.Fatalf("expected status 'active', got %v", status["status"])
	}

	for _, field := range []string{"completed_at", "total_duration_sec", "total_tokens_used"} {
		if _, exists := status[field]; exists {
			t.Fatalf("expected %s to be absent for active workflow, but got %v", field, status[field])
		}
	}
}

// TestCompletedWorkflowTokensBoundary verifies token calculation edge cases:
//   - context_left=0 (fully consumed) → 200000 tokens
//   - context_left=100 (no consumption) → 0 tokens
//   - context_left=NULL (not reported) → excluded from total
func TestCompletedWorkflowTokensBoundary(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CS-3", "Token boundary values")
	env.InitWorkflow(t, "CS-3")

	wfiID := env.GetWorkflowInstanceID(t, "CS-3", "test")

	// context_left=0 → 200000*(100-0)/100 = 200000 tokens
	insertSessionWithContextLeft(t, env, "cs3-sess-1", "CS-3", wfiID,
		"analyzer", "agent-fully-consumed", "claude:sonnet", "completed", "pass", 0)
	// context_left=100 → 200000*(100-100)/100 = 0 tokens
	insertSessionWithContextLeft(t, env, "cs3-sess-2", "CS-3", wfiID,
		"builder", "agent-no-consumption", "claude:opus", "completed", "pass", 100)
	// NULL context_left (using insertCompletedSession which sets context_left=NULL)
	insertCompletedSession(t, env, "cs3-sess-3", "CS-3", wfiID,
		"analyzer", "agent-null-context", "claude:haiku", "completed", "pass")

	// Mark workflow completed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	if err := wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("failed to set workflow completed: %v", err)
	}

	status, err := getWorkflowStatus(t, env, "CS-3", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	// Total should be 200000 (from context_left=0) + 0 (from context_left=100)
	// The NULL context_left agent should not contribute to the total
	tokensUsed, ok := status["total_tokens_used"].(float64)
	if !ok {
		t.Fatalf("expected total_tokens_used to be float64, got %T (%v)", status["total_tokens_used"], status["total_tokens_used"])
	}
	if int(tokensUsed) != 200000 {
		t.Fatalf("expected total_tokens_used 200000, got %v", tokensUsed)
	}
}

// TestCompletedWorkflowDuration verifies that total_duration_sec reflects the
// time between created_at and updated_at of the workflow instance.
func TestCompletedWorkflowDuration(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CS-4", "Duration test")
	env.InitWorkflow(t, "CS-4")

	wfiID := env.GetWorkflowInstanceID(t, "CS-4", "test")

	// Set created_at to a known time in the past
	past := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	_, err := env.Pool.Exec(`UPDATE workflow_instances SET created_at = ? WHERE id = ?`, past, wfiID)
	if err != nil {
		t.Fatalf("failed to update created_at: %v", err)
	}

	// Mark completed (UpdateStatus sets updated_at to now)
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	if err := wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("failed to set workflow completed: %v", err)
	}

	status, err := getWorkflowStatus(t, env, "CS-4", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	durationSec, ok := status["total_duration_sec"].(float64)
	if !ok {
		t.Fatalf("expected total_duration_sec to be float64, got %T", status["total_duration_sec"])
	}

	// Should be approximately 300 seconds (5 minutes), with some tolerance
	if durationSec < 290 || durationSec > 310 {
		t.Fatalf("expected total_duration_sec ~300, got %v", durationSec)
	}
}

// TestCompletedWorkflowNoAgents verifies that a completed workflow with no
// agent sessions returns total_tokens_used = 0.
func TestCompletedWorkflowNoAgents(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CS-5", "No agents completed")
	env.InitWorkflow(t, "CS-5")

	wfiID := env.GetWorkflowInstanceID(t, "CS-5", "test")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	if err := wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("failed to set workflow completed: %v", err)
	}

	status, err := getWorkflowStatus(t, env, "CS-5", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	tokensUsed, ok := status["total_tokens_used"].(float64)
	if !ok {
		t.Fatalf("expected total_tokens_used to be float64, got %T", status["total_tokens_used"])
	}
	if int(tokensUsed) != 0 {
		t.Fatalf("expected total_tokens_used 0, got %v", tokensUsed)
	}
}
