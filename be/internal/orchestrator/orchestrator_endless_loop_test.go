package orchestrator

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// pollUntil polls check every 5ms until true or timeout; returns whether condition met.
func pollUntil(timeout time.Duration, check func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return check()
}

// seedEndlessLoopInstance promotes the workflow to project scope, creates a
// project-scoped workflow_instance, and sets the given endless-loop flags /
// project_completed status. Returns the instance ID.
func (e *testEnv) seedEndlessLoopInstance(t *testing.T, workflowID string, endlessLoop, stopFlag bool) string {
	t.Helper()
	wfiID := e.initProjectWorkflow(t, workflowID)

	if _, err := e.pool.Exec(
		`UPDATE workflow_instances SET endless_loop = ?, stop_endless_loop_after_iteration = ?,
			status = ? WHERE id = ?`,
		endlessLoop, stopFlag, model.WorkflowInstanceProjectCompleted, wfiID,
	); err != nil {
		t.Fatalf("failed to update endless_loop flags: %v", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(e.pool, clock.Real())
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("failed to read back wfi: %v", err)
	}
	if wi.EndlessLoop != endlessLoop || wi.StopEndlessLoopAfterIteration != stopFlag {
		t.Fatalf("seed mismatch: endless=%v stop=%v want %v/%v",
			wi.EndlessLoop, wi.StopEndlessLoopAfterIteration, endlessLoop, stopFlag)
	}
	return wfiID
}

// countProjectInstances returns the number of workflow_instances for the
// test project and given workflow id (any status).
func (e *testEnv) countProjectInstances(t *testing.T, workflowID string) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(
		`SELECT COUNT(*) FROM workflow_instances
			WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		e.project, workflowID,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count instances: %v", err)
	}
	return n
}

// TestStart_ProjectScope_EndlessLoop_PersistsFlag verifies RunRequest.EndlessLoop
// is propagated through InitProjectWorkflow and persisted on the workflow_instances row.
func TestStart_ProjectScope_EndlessLoop_PersistsFlag(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.pool.Exec(
		`UPDATE workflows SET scope_type = 'project' WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER('test')`,
		env.project,
	); err != nil {
		t.Fatalf("promote workflow: %v", err)
	}

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
		EndlessLoop:  true,
	})
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if result == nil || result.InstanceID == "" {
		t.Fatal("Start() returned empty InstanceID")
	}

	// Stop immediately so the runLoop goroutine exits cleanly before cleanup.
	env.orch.Stop(result.InstanceID)

	wi := env.getWorkflowInstance(t, result.InstanceID)
	if !wi.EndlessLoop {
		t.Errorf("wfi.EndlessLoop = false, want true")
	}
	if wi.ScopeType != "project" {
		t.Errorf("wfi.ScopeType = %q, want 'project'", wi.ScopeType)
	}
}

// TestStart_TicketScope_EndlessLoopNotPersisted verifies the orchestrator does not
// persist EndlessLoop on a ticket-scoped run (flag is ignored for ticket scope;
// handler-level validation also rejects this combination for defense-in-depth).
func TestStart_TicketScope_EndlessLoopNotPersisted(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "EL-TICKET", "ticket endless loop")

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "EL-TICKET",
		WorkflowName: "test",
		ScopeType:    "ticket",
		EndlessLoop:  true,
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	env.orch.Stop(result.InstanceID)

	wi := env.getWorkflowInstance(t, result.InstanceID)
	if wi.EndlessLoop {
		t.Errorf("ticket-scoped wfi.EndlessLoop = true, want false (flag must be ignored for ticket scope)")
	}
}

// TestMaybeRestartEndlessLoop_StopFlagSet_SkipsRestart verifies that when
// stop_endless_loop_after_iteration=true, no new workflow instance is spawned.
func TestMaybeRestartEndlessLoop_StopFlagSet_SkipsRestart(t *testing.T) {
	env := newTestEnv(t)

	wfiID := env.seedEndlessLoopInstance(t, "test", true, true)
	initial := env.countProjectInstances(t, "test")

	env.orch.maybeRestartEndlessLoop(wfiID, RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
		EndlessLoop:  true,
	})

	// Ensure no new instance appears within a short window.
	if pollUntil(200*time.Millisecond, func() bool {
		return env.countProjectInstances(t, "test") != initial
	}) {
		t.Fatalf("new instance was created despite stop flag; got %d (was %d)",
			env.countProjectInstances(t, "test"), initial)
	}
}

// TestMaybeRestartEndlessLoop_ClearFlag_SpawnsNewInstance verifies that when
// the stop flag is false, maybeRestartEndlessLoop kicks off a new Start() that
// creates a fresh workflow_instances row carrying endless_loop=1.
func TestMaybeRestartEndlessLoop_ClearFlag_SpawnsNewInstance(t *testing.T) {
	env := newTestEnv(t)

	wfiID := env.seedEndlessLoopInstance(t, "test", true, false)
	if env.countProjectInstances(t, "test") != 1 {
		t.Fatalf("setup: expected 1 instance, got %d", env.countProjectInstances(t, "test"))
	}

	env.orch.maybeRestartEndlessLoop(wfiID, RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
		EndlessLoop:  true,
	})

	if !pollUntil(5*time.Second, func() bool {
		return env.countProjectInstances(t, "test") >= 2
	}) {
		t.Fatalf("timeout waiting for second workflow_instances row")
	}

	// Find the non-wfiID instance and verify fields.
	var secondID string
	err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances
			WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER('test') AND id != ?`,
		env.project, wfiID,
	).Scan(&secondID)
	if err != nil {
		t.Fatalf("querying second instance id: %v", err)
	}

	wi := env.getWorkflowInstance(t, secondID)
	if !wi.EndlessLoop {
		t.Errorf("second instance endless_loop = false, want true")
	}
	if wi.ScopeType != "project" {
		t.Errorf("second instance scope_type = %q, want 'project'", wi.ScopeType)
	}
	if wi.TicketID != "" {
		t.Errorf("second instance ticket_id = %q, want empty", wi.TicketID)
	}

	// Stop the second run so cleanup doesn't hang waiting on it.
	env.orch.Stop(secondID)
}

// TestRunLoop_CtxCancelled_SkipsEndlessLoopRestart verifies that when the run
// is stopped mid-iteration (ctx cancelled), runLoop does not spawn a restart
// even with EndlessLoop=true. The `ctx.Err() == nil` guard in runLoop handles this.
func TestRunLoop_CtxCancelled_SkipsEndlessLoopRestart(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.pool.Exec(
		`UPDATE workflows SET scope_type = 'project' WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER('test')`,
		env.project,
	); err != nil {
		t.Fatalf("promote workflow: %v", err)
	}

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
		EndlessLoop:  true,
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Cancel the run immediately; runLoop should take the failure/cancel path.
	env.orch.Stop(result.InstanceID)
	if !pollUntil(5*time.Second, func() bool {
		return !env.orch.IsInstanceRunning(result.InstanceID)
	}) {
		t.Fatal("timeout waiting for first instance to stop")
	}

	// Ensure no second instance appears.
	if pollUntil(200*time.Millisecond, func() bool {
		return env.countProjectInstances(t, "test") > 1
	}) {
		t.Fatalf("expected exactly 1 instance after cancelled run, got %d",
			env.countProjectInstances(t, "test"))
	}
}
