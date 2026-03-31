package integration

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestTicketWorkflowRerunPreservesAgentSessions verifies that re-running a completed
// ticket workflow creates a new instance while the old instance retains its agent sessions.
func TestTicketWorkflowRerunPreservesAgentSessions(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "TRPAS-1", "Session preservation test")

	// Init first workflow instance
	wi1, err := env.WorkflowSvc.Init(env.ProjectID, "TRPAS-1", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}

	// Attach an agent session to the first instance and complete it
	env.InsertAgentSession(t, "sess-trpas-1", "TRPAS-1", wi1.ID, "analyzer", "analyzer", "")
	env.CompleteAgentSession(t, "sess-trpas-1", "pass")

	// Mark first instance as completed with recognizable findings
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	wfiRepo.UpdateStatus(wi1.ID, model.WorkflowInstanceCompleted)
	wfiRepo.UpdateFindings(wi1.ID, `{"old_key":"old_val"}`)

	// Advance clock so the second instance has a newer created_at timestamp
	env.Clock.Advance(1 * time.Second)

	// Init second workflow instance (simulates a re-run)
	wi2, err := env.WorkflowSvc.Init(env.ProjectID, "TRPAS-1", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}

	if wi1.ID == wi2.ID {
		t.Fatalf("expected distinct instance IDs; both are %s", wi1.ID)
	}

	// Old instance must remain completed with unchanged findings
	oldInst, err := wfiRepo.Get(wi1.ID)
	if err != nil {
		t.Fatalf("get old instance: %v", err)
	}
	if oldInst.Status != model.WorkflowInstanceCompleted {
		t.Errorf("old instance status = %s, want completed", oldInst.Status)
	}
	if oldInst.Findings != `{"old_key":"old_val"}` {
		t.Errorf("old instance findings = %q, want original value", oldInst.Findings)
	}

	// Old agent session must still belong to the first instance
	var sessWFI string
	if err := env.Pool.QueryRow(
		`SELECT workflow_instance_id FROM agent_sessions WHERE id = ?`, "sess-trpas-1",
	).Scan(&sessWFI); err != nil {
		t.Fatalf("old session not found: %v", err)
	}
	if sessWFI != wi1.ID {
		t.Errorf("old session workflow_instance_id = %q, want %q", sessWFI, wi1.ID)
	}

	// New instance must be fresh
	newInst, err := wfiRepo.Get(wi2.ID)
	if err != nil {
		t.Fatalf("get new instance: %v", err)
	}
	if newInst.Status != model.WorkflowInstanceActive {
		t.Errorf("new instance status = %s, want active", newInst.Status)
	}
	if newInst.RetryCount != 0 {
		t.Errorf("new instance retry_count = %d, want 0", newInst.RetryCount)
	}

	// Both instances must be visible in ListByTicket
	instances, err := wfiRepo.ListByTicket(env.ProjectID, "TRPAS-1")
	if err != nil {
		t.Fatalf("ListByTicket: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
}

// TestGetByTicketAndWorkflowReturnsLatest verifies that after multiple Init() calls,
// GetByTicketAndWorkflow returns the most recently created instance (ORDER BY created_at DESC).
func TestGetByTicketAndWorkflowReturnsLatest(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "TRPAS-2", "Latest instance test")

	wi1, err := env.WorkflowSvc.Init(env.ProjectID, "TRPAS-2", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}

	// Advance clock so the second instance is strictly newer
	env.Clock.Advance(2 * time.Second)

	wi2, err := env.WorkflowSvc.Init(env.ProjectID, "TRPAS-2", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, env.Clock)
	latest, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "TRPAS-2", "test")
	if err != nil {
		t.Fatalf("GetByTicketAndWorkflow: %v", err)
	}

	if latest.ID != wi2.ID {
		t.Errorf("GetByTicketAndWorkflow returned %q, want latest %q", latest.ID, wi2.ID)
	}
	if latest.ID == wi1.ID {
		t.Error("GetByTicketAndWorkflow returned first instance, expected latest")
	}
}

// TestTicketWorkflowMultiInstanceGetStatusByInstance verifies that GetStatusByInstance
// returns the correct state (instance_id, workflow) for each of the multiple instances.
func TestTicketWorkflowMultiInstanceGetStatusByInstance(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "TRPAS-3", "GetStatusByInstance test")

	wi1, err := env.WorkflowSvc.Init(env.ProjectID, "TRPAS-3", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}
	env.Clock.Advance(1 * time.Second)

	wi2, err := env.WorkflowSvc.Init(env.ProjectID, "TRPAS-3", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}

	for _, wi := range []*struct {
		id         string
		workflowID string
	}{
		{wi1.ID, wi1.WorkflowID},
		{wi2.ID, wi2.WorkflowID},
	} {
		state, err := env.WorkflowSvc.GetStatusByInstance(&model.WorkflowInstance{
			ID:         wi.id,
			ProjectID:  env.ProjectID,
			WorkflowID: wi.workflowID,
			ScopeType:  "ticket",
			Status:     model.WorkflowInstanceActive,
			Findings:   "{}",
		})
		if err != nil {
			t.Fatalf("GetStatusByInstance(%s): %v", wi.id, err)
		}
		if state["instance_id"] != wi.id {
			t.Errorf("instance_id = %v, want %q", state["instance_id"], wi.id)
		}
		if state["workflow"] != wi.workflowID {
			t.Errorf("workflow = %v, want %q", state["workflow"], wi.workflowID)
		}
	}

	// Both instances must appear in ListWorkflowInstances
	instances, err := env.WorkflowSvc.ListWorkflowInstances(env.ProjectID, "TRPAS-3")
	if err != nil {
		t.Fatalf("ListWorkflowInstances: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}

	// IDs must be distinct
	if instances[0].ID == instances[1].ID {
		t.Error("expected distinct instance IDs in ListWorkflowInstances")
	}
}
