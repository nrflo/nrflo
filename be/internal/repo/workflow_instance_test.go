package repo

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"be/internal/db"
	"be/internal/model"
)

func TestUpdateStatusToProjectCompleted(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Create project first
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, categories, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"test-workflow", "test-project", "Test Workflow", "project", `["full"]`, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool)

	// Create a workflow instance
	phaseOrder, _ := json.Marshal([]string{"phase1"})
	phases, _ := json.Marshal(map[string]model.PhaseStatus{"phase1": {Status: "pending"}})
	findings, _ := json.Marshal(map[string]interface{}{})

	wi := &model.WorkflowInstance{
		ID:         "test-wfi",
		ProjectID:  "test-project",
		TicketID:   "",
		WorkflowID: "test-workflow",
		ScopeType:  "project",
		Status:     model.WorkflowInstanceActive,
		Category:   sql.NullString{String: "full", Valid: true},
		PhaseOrder: string(phaseOrder),
		Phases:     string(phases),
		Findings:   string(findings),
	}

	if err := repo.Create(wi); err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Update status to project_completed
	err = repo.UpdateStatus("test-wfi", model.WorkflowInstanceProjectCompleted)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Read it back
	wi2, err := repo.Get("test-wfi")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify status was updated
	if wi2.Status != model.WorkflowInstanceProjectCompleted {
		t.Fatalf("expected status 'project_completed', got %v", wi2.Status)
	}
}

func TestListByProjectScopeIncludesAllStatuses(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "test-project"

	// Create project first
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		projectID, "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool)

	// Each instance needs its own workflow due to UNIQUE constraint on (project_id, workflow_id, scope_type).
	instances := []struct {
		id         string
		workflowID string
		status     model.WorkflowInstanceStatus
	}{
		{"wfi-active", "wf-active", model.WorkflowInstanceActive},
		{"wfi-completed", "wf-completed", model.WorkflowInstanceCompleted},
		{"wfi-failed", "wf-failed", model.WorkflowInstanceFailed},
		{"wfi-proj-completed", "wf-proj-completed", model.WorkflowInstanceProjectCompleted},
	}

	phaseOrder, _ := json.Marshal([]string{"phase1"})
	phases, _ := json.Marshal(map[string]model.PhaseStatus{"phase1": {Status: "completed", Result: "pass"}})
	findings, _ := json.Marshal(map[string]interface{}{})
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})

	for _, inst := range instances {
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, categories, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			inst.workflowID, projectID, "Test Workflow", "project", `["full"]`, string(phasesJSON))
		if err != nil {
			t.Fatalf("failed to create workflow %s: %v", inst.workflowID, err)
		}

		wi := &model.WorkflowInstance{
			ID:         inst.id,
			ProjectID:  projectID,
			TicketID:   "",
			WorkflowID: inst.workflowID,
			ScopeType:  "project",
			Status:     inst.status,
			Category:   sql.NullString{String: "full", Valid: true},
			PhaseOrder: string(phaseOrder),
			Phases:     string(phases),
			Findings:   string(findings),
		}
		if err := repo.Create(wi); err != nil {
			t.Fatalf("failed to create workflow instance %s: %v", inst.id, err)
		}
	}

	// Call ListByProjectScope - should return ALL project-scoped instances including project_completed
	results, err := repo.ListByProjectScope(projectID)
	if err != nil {
		t.Fatalf("ListByProjectScope failed: %v", err)
	}

	// All project-scoped instances are returned including project_completed
	if len(results) != 4 {
		t.Fatalf("expected 4 instances, got %d", len(results))
	}

	foundIDs := make(map[string]bool)
	for _, wi := range results {
		foundIDs[wi.ID] = true
	}

	for _, id := range []string{"wfi-active", "wfi-completed", "wfi-failed", "wfi-proj-completed"} {
		if !foundIDs[id] {
			t.Fatalf("expected instance %s to be in results", id)
		}
	}
}
