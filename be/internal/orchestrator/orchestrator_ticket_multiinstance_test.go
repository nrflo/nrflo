package orchestrator

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
)

// TestIsRunning_MultipleInstances_OnlyOneActive verifies that IsRunning returns true
// when at least one of multiple ticket+workflow instances is in the runs map.
func TestIsRunning_MultipleInstances_OnlyOneActive(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TM-1", "Multi-instance IsRunning")

	// Create first instance (will NOT be in runs)
	env.initWorkflow(t, "TM-1")

	// Create second instance via service (to get its ID)
	wfSvc := service.NewWorkflowService(env.pool, clock.Real())
	wi2, err := wfSvc.Init(env.project, "TM-1", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init second workflow: %v", err)
	}

	// Add only the second instance to runs
	env.orch.mu.Lock()
	env.orch.runs[wi2.ID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wi2.ID)
		env.orch.mu.Unlock()
	}()

	// IsRunning must return true because the second instance is in runs
	if !env.orch.IsRunning(env.project, "TM-1", "test") {
		t.Fatal("IsRunning should return true when second instance is in runs")
	}
}

// TestIsRunning_MultipleInstances_NoneActive verifies that IsRunning returns false
// when multiple ticket+workflow instances exist but none are in the runs map.
func TestIsRunning_MultipleInstances_NoneActive(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TM-2", "Multi-instance IsRunning none")

	env.initWorkflow(t, "TM-2")

	wfSvc := service.NewWorkflowService(env.pool, clock.Real())
	_, err := wfSvc.Init(env.project, "TM-2", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init second workflow: %v", err)
	}

	// No instances in runs — should report not running
	if env.orch.IsRunning(env.project, "TM-2", "test") {
		t.Fatal("IsRunning should return false when no instances are in runs")
	}
}

// TestStopByTicket_WithExplicitInstanceID verifies that passing a specific instanceID
// cancels only that instance, leaving the other intact.
func TestStopByTicket_WithExplicitInstanceID(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TM-3", "Stop with instance ID")

	wfiID1 := env.initWorkflow(t, "TM-3")

	wfSvc := service.NewWorkflowService(env.pool, clock.Real())
	wi2, err := wfSvc.Init(env.project, "TM-3", &types.WorkflowInitRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to init second workflow: %v", err)
	}

	cancelled1 := false
	cancelled2 := false
	env.orch.mu.Lock()
	env.orch.runs[wfiID1] = &runState{cancel: func() { cancelled1 = true }}
	env.orch.runs[wi2.ID] = &runState{cancel: func() { cancelled2 = true }}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID1)
		delete(env.orch.runs, wi2.ID)
		env.orch.mu.Unlock()
	}()

	// Stop only wfiID1 using its explicit instance ID
	err = env.orch.StopByTicket(env.project, "TM-3", "test", wfiID1)
	if err != nil {
		t.Fatalf("StopByTicket with instanceID: %v", err)
	}

	if !cancelled1 {
		t.Error("expected instance 1 to be cancelled")
	}
	if cancelled2 {
		t.Error("expected instance 2 NOT to be cancelled")
	}
}

// TestConcurrentTicketWorkflowPrevented verifies that starting a second orchestration
// for the same ticket+workflow fails with an "already running" error when one
// instance is already active in the runs map.
func TestConcurrentTicketWorkflowPrevented(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TM-4", "Concurrent run prevention")

	wfiID := env.initWorkflow(t, "TM-4")

	// Simulate the first instance already in progress
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	// Attempting to start another run for the same ticket+workflow must fail
	_, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "TM-4",
		WorkflowName: "test",
		ScopeType:    "ticket",
	})
	if err == nil {
		t.Fatal("expected error for concurrent ticket workflow run")
	}
	want := "workflow 'test' is already running on TM-4"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
