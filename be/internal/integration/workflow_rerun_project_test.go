package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/types"
)

// TestRerunCompletedProjectWorkflow tests that re-running a project_completed workflow
// creates a NEW instance (not reset the old one), since multiple concurrent instances are allowed.
func TestRerunCompletedProjectWorkflow(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
		{"agent": "impl", "layer": 1},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "rerun-test",
		Description: "Test rerun workflow",
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Initialize project workflow
	firstInstance, err := env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "rerun-test",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// Simulate completion: set status to project_completed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	wfiRepo.UpdateStatus(firstInstance.ID, model.WorkflowInstanceProjectCompleted)
	wfiRepo.UpdateFindings(firstInstance.ID, `{"some_key": "some_value"}`)

	// Verify it's completed
	firstInstance, _ = wfiRepo.Get(firstInstance.ID)
	if firstInstance.Status != model.WorkflowInstanceProjectCompleted {
		t.Fatalf("expected status project_completed, got %v", firstInstance.Status)
	}

	// Verify ListByProjectScope returns it
	instances, err := wfiRepo.ListByProjectScope(env.ProjectID)
	if err != nil {
		t.Fatalf("ListByProjectScope failed: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance from ListByProjectScope, got %d", len(instances))
	}

	// Create orchestrator to re-run the workflow
	orch := orchestrator.New(env.Pool.Path, env.Hub, clock.Real())

	// Start the workflow again — should create a NEW instance
	ctx := context.Background()
	result, err := orch.Start(ctx, orchestrator.RunRequest{
		ProjectID:    env.ProjectID,
		WorkflowName: "rerun-test",
		ScopeType:    "project",
	})

	if err != nil {
		t.Fatalf("failed to start orchestrator: %v", err)
	}
	if result.Status != "started" {
		t.Fatalf("expected status 'started', got %v", result.Status)
	}

	// New instance should have a different ID
	if result.InstanceID == firstInstance.ID {
		t.Fatalf("expected new instance ID, got same as first: %s", result.InstanceID)
	}

	// Stop right away to avoid spawning actual agents
	orch.Stop(result.InstanceID)
	waitForCondition(t, time.Second, func() bool {
		return !orch.IsInstanceRunning(result.InstanceID)
	})

	// Verify the NEW instance was created with fresh state
	newInstance, err := wfiRepo.Get(result.InstanceID)
	if err != nil {
		t.Fatalf("failed to get new workflow instance: %v", err)
	}

	// New instance should have retry_count = 0 (fresh instance)
	if newInstance.RetryCount != 0 {
		t.Fatalf("expected retry_count 0 for new instance, got %d", newInstance.RetryCount)
	}

	// Old instance should still be project_completed
	oldInstance, _ := wfiRepo.Get(firstInstance.ID)
	if oldInstance.Status != model.WorkflowInstanceProjectCompleted {
		t.Fatalf("expected old instance to remain project_completed, got %v", oldInstance.Status)
	}

	// Verify both instances exist
	instances, _ = wfiRepo.ListByProjectScope(env.ProjectID)
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances after rerun, got %d", len(instances))
	}
}

// TestConcurrentProjectWorkflowsAllowed tests that multiple concurrent instances of
// the same project workflow can be started.
func TestConcurrentProjectWorkflowsAllowed(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "concurrent-test",
		Description: "Test concurrent workflows",
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Create orchestrator and start first workflow
	orch := orchestrator.New(env.Pool.Path, env.Hub, clock.Real())
	ctx := context.Background()

	result1, err := orch.Start(ctx, orchestrator.RunRequest{
		ProjectID:    env.ProjectID,
		WorkflowName: "concurrent-test",
		ScopeType:    "project",
	})
	if err != nil {
		t.Fatalf("failed to start first orchestration: %v", err)
	}

	// Start second instance — should succeed (no longer blocked)
	result2, err := orch.Start(ctx, orchestrator.RunRequest{
		ProjectID:    env.ProjectID,
		WorkflowName: "concurrent-test",
		ScopeType:    "project",
	})
	if err != nil {
		t.Fatalf("expected second start to succeed, got error: %v", err)
	}

	// Different instance IDs
	if result1.InstanceID == result2.InstanceID {
		t.Fatalf("expected different instance IDs, got same: %s", result1.InstanceID)
	}

	// Stop both
	orch.Stop(result1.InstanceID)
	orch.Stop(result2.InstanceID)
	waitForCondition(t, time.Second, func() bool {
		return !orch.IsInstanceRunning(result1.InstanceID) && !orch.IsInstanceRunning(result2.InstanceID)
	})

	// Verify both instances exist
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	instances, _ := wfiRepo.ListByProjectScope(env.ProjectID)
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
}

// TestCompletedTicketWorkflowUnaffected tests that re-running a completed ticket-scoped
// workflow creates a new instance and leaves the old one unchanged.
func TestCompletedTicketWorkflowUnaffected(t *testing.T) {
	env := NewTestEnv(t)

	// Create ticket
	env.CreateTicket(t, "TICKET-1", "Test ticket")

	// Initialize ticket workflow (uses default "test" workflow from TestEnv)
	env.InitWorkflow(t, "TICKET-1")

	// Get the workflow instance
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	wi, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "TICKET-1", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	firstInstanceID := wi.ID

	// Set it to completed
	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceCompleted)
	wfiRepo.UpdateFindings(wi.ID, `{"ticket_finding": "ticket_value"}`)

	// Create orchestrator and try to start again
	orch := orchestrator.New(env.Pool.Path, env.Hub, clock.Real())
	ctx := context.Background()

	result, err := orch.Start(ctx, orchestrator.RunRequest{
		ProjectID:    env.ProjectID,
		TicketID:     "TICKET-1",
		WorkflowName: "test",
		ScopeType:    "ticket",
	})
	if err != nil {
		t.Fatalf("failed to start orchestrator for ticket workflow: %v", err)
	}

	// New instance should have a different ID
	if result.InstanceID == firstInstanceID {
		t.Fatalf("expected new instance ID, got same as first: %s", result.InstanceID)
	}

	// Stop immediately
	orch.Stop(result.InstanceID)
	waitForCondition(t, time.Second, func() bool {
		return !orch.IsInstanceRunning(result.InstanceID)
	})

	// Old instance should remain completed with its findings intact
	oldInstance, err := wfiRepo.Get(firstInstanceID)
	if err != nil {
		t.Fatalf("failed to get old workflow instance: %v", err)
	}
	if oldInstance.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected old instance to remain completed, got %v", oldInstance.Status)
	}
	if oldInstance.Findings != `{"ticket_finding": "ticket_value"}` {
		t.Fatalf("expected old instance findings preserved, got %s", oldInstance.Findings)
	}

	// New instance should exist with retry_count = 0 (fresh instance)
	newInstance, err := wfiRepo.Get(result.InstanceID)
	if err != nil {
		t.Fatalf("failed to get new workflow instance: %v", err)
	}
	if newInstance.RetryCount != 0 {
		t.Fatalf("expected retry_count 0 for new instance, got %d", newInstance.RetryCount)
	}

	// Both instances should exist
	instances, _ := wfiRepo.ListByTicket(env.ProjectID, "TICKET-1")
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances after rerun, got %d", len(instances))
	}
}

// TestMultipleProjectWorkflowsListed tests that when multiple project workflows exist
// with different statuses, all are returned by ListByProjectScope (including project_completed).
func TestMultipleProjectWorkflowsListed(t *testing.T) {
	env := NewTestEnv(t)

	// Create three different project workflows
	workflows := []struct {
		id     string
		status model.WorkflowInstanceStatus
	}{
		{"wf-active", model.WorkflowInstanceActive},
		{"wf-failed", model.WorkflowInstanceFailed},
		{"wf-proj-completed", model.WorkflowInstanceProjectCompleted},
	}

	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())

	for _, wf := range workflows {
		// Create workflow definition
		_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
			ID:          wf.id,
			Description: "Test workflow",
			Phases:      phasesJSON,
			ScopeType:   "project",
		})
		if err != nil {
			t.Fatalf("failed to create workflow def %s: %v", wf.id, err)
		}

		// Initialize workflow — capture returned instance directly
		wi, err := env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
			Workflow: wf.id,
		})
		if err != nil {
			t.Fatalf("failed to init project workflow %s: %v", wf.id, err)
		}

		// Update status
		wfiRepo.UpdateStatus(wi.ID, wf.status)
	}

	// List all project workflows
	instances, err := wfiRepo.ListByProjectScope(env.ProjectID)
	if err != nil {
		t.Fatalf("ListByProjectScope failed: %v", err)
	}

	// Should return all 3 workflows including project_completed
	if len(instances) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(instances))
	}

	// Verify all expected workflows are present
	found := make(map[string]model.WorkflowInstanceStatus)
	for _, wi := range instances {
		found[wi.WorkflowID] = wi.Status
	}

	for _, wf := range workflows {
		status, exists := found[wf.id]
		if !exists {
			t.Fatalf("workflow %s not found in results", wf.id)
		}
		if status != wf.status {
			t.Fatalf("workflow %s: expected status %v, got %v", wf.id, wf.status, status)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
