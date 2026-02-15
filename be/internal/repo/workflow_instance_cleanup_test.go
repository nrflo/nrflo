package repo

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func TestWorkflowInstanceCleanupKeepLatest(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Create project
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflows
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	for i := 1; i <= 5; i++ {
		wfID := "test-workflow-" + string(rune(i+'0'))
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			wfID, "test-project", "Test Workflow", "ticket", string(phasesJSON))
		if err != nil {
			t.Fatalf("failed to create workflow %s: %v", wfID, err)
		}
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())

	phaseOrder, _ := json.Marshal([]string{"phase1"})
	phases, _ := json.Marshal(map[string]model.PhaseStatus{"phase1": {Status: "completed", Result: "pass"}})
	findings, _ := json.Marshal(map[string]interface{}{})

	// Insert 5 instances: 3 completed, 2 active
	// The 3 completed instances should be ordered by updated_at with varying timestamps
	instances := []struct {
		id         string
		workflowID string
		status     model.WorkflowInstanceStatus
		ticketID   string
	}{
		{"wfi-completed-1", "test-workflow-1", model.WorkflowInstanceCompleted, "TKT-1"},
		{"wfi-completed-2", "test-workflow-2", model.WorkflowInstanceCompleted, "TKT-2"},
		{"wfi-completed-3", "test-workflow-3", model.WorkflowInstanceCompleted, "TKT-3"},
		{"wfi-active-1", "test-workflow-4", model.WorkflowInstanceActive, "TKT-4"},
		{"wfi-active-2", "test-workflow-5", model.WorkflowInstanceActive, "TKT-5"},
	}

	now := time.Now().UTC()
	for i, inst := range instances {
		// Create instances with different updated_at timestamps (older first)
		updatedAt := now.Add(time.Duration(-5+i) * time.Minute).Format(time.RFC3339Nano)

		_, err = pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, phase_order, phases, findings, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			inst.id, "test-project", inst.ticketID, inst.workflowID, inst.status, "ticket", string(phaseOrder), string(phases), string(findings), updatedAt, updatedAt)
		if err != nil {
			t.Fatalf("failed to create workflow instance %s: %v", inst.id, err)
		}
	}

	// Call CleanupKeepLatest(2) - keep only 2 latest completed + all active
	deleted, err := repo.CleanupKeepLatest(2)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	// Should delete 1 completed instance (oldest one)
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify remaining instances
	var count int
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count instances: %v", err)
	}
	// Should have 4 remaining: 2 completed + 2 active
	if count != 4 {
		t.Errorf("expected 4 remaining instances, got %d", count)
	}

	// Verify oldest completed instance was deleted
	var exists bool
	err = pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, "wfi-completed-1").Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check existence: %v", err)
	}
	if exists {
		t.Errorf("expected wfi-completed-1 to be deleted")
	}

	// Verify latest 2 completed instances remain
	for _, id := range []string{"wfi-completed-2", "wfi-completed-3"} {
		err = pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check existence: %v", err)
		}
		if !exists {
			t.Errorf("expected %s to remain", id)
		}
	}

	// Verify all active instances remain
	for _, id := range []string{"wfi-active-1", "wfi-active-2"} {
		err = pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check existence: %v", err)
		}
		if !exists {
			t.Errorf("expected %s to remain (active)", id)
		}
	}
}

func TestWorkflowInstanceCleanupKeepLatest_ZeroKeep(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Create project
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflows
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	for i := 1; i <= 3; i++ {
		wfID := "test-workflow-" + string(rune(i+'0'))
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			wfID, "test-project", "Test Workflow", "ticket", string(phasesJSON))
		if err != nil {
			t.Fatalf("failed to create workflow %s: %v", wfID, err)
		}
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())

	phaseOrder, _ := json.Marshal([]string{"phase1"})
	phases, _ := json.Marshal(map[string]model.PhaseStatus{"phase1": {Status: "completed"}})
	findings, _ := json.Marshal(map[string]interface{}{})

	// Insert 3 completed instances
	for i := 1; i <= 3; i++ {
		wfiID := "wfi-" + string(rune(i+'0'))
		wfID := "test-workflow-" + string(rune(i+'0'))
		ticketID := "TKT-" + string(rune(i+'0'))

		wi := &model.WorkflowInstance{
			ID:         wfiID,
			ProjectID:  "test-project",
			TicketID:   ticketID,
			WorkflowID: wfID,
			ScopeType:  "ticket",
			Status:     model.WorkflowInstanceCompleted,
			PhaseOrder: string(phaseOrder),
			Phases:     string(phases),
			Findings:   string(findings),
		}
		if err := repo.Create(wi); err != nil {
			t.Fatalf("failed to create instance: %v", err)
		}
	}

	// CleanupKeepLatest(0) should delete all non-active instances
	deleted, err := repo.CleanupKeepLatest(0)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", deleted)
	}

	var count int
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count instances: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 remaining instances, got %d", count)
	}
}

func TestWorkflowInstanceCleanupKeepLatest_KeepExceedsTotal(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Create project
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"test-workflow", "test-project", "Test Workflow", "ticket", string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())

	phaseOrder, _ := json.Marshal([]string{"phase1"})
	phases, _ := json.Marshal(map[string]model.PhaseStatus{"phase1": {Status: "completed"}})
	findings, _ := json.Marshal(map[string]interface{}{})

	// Insert only 2 completed instances
	for i := 1; i <= 2; i++ {
		wfiID := "wfi-" + string(rune(i+'0'))
		ticketID := "TKT-" + string(rune(i+'0'))

		wi := &model.WorkflowInstance{
			ID:         wfiID,
			ProjectID:  "test-project",
			TicketID:   ticketID,
			WorkflowID: "test-workflow",
			ScopeType:  "ticket",
			Status:     model.WorkflowInstanceCompleted,
			PhaseOrder: string(phaseOrder),
			Phases:     string(phases),
			Findings:   string(findings),
		}
		if err := repo.Create(wi); err != nil {
			t.Fatalf("failed to create instance: %v", err)
		}
		// Sleep to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// CleanupKeepLatest(100) should delete nothing (keep > total)
	deleted, err := repo.CleanupKeepLatest(100)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}

	var count int
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count instances: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 remaining instances, got %d", count)
	}
}

func TestWorkflowInstanceCleanupKeepLatest_EmptyTable(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewWorkflowInstanceRepo(pool, clock.Real())

	// Call cleanup on empty table
	deleted, err := repo.CleanupKeepLatest(10)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted from empty table, got %d", deleted)
	}
}

func TestWorkflowInstanceCleanupKeepLatest_OnlyActiveInstances(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Create project
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflows
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	for i := 1; i <= 3; i++ {
		wfID := "test-workflow-" + string(rune(i+'0'))
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			wfID, "test-project", "Test Workflow", "ticket", string(phasesJSON))
		if err != nil {
			t.Fatalf("failed to create workflow %s: %v", wfID, err)
		}
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())

	phaseOrder, _ := json.Marshal([]string{"phase1"})
	phases, _ := json.Marshal(map[string]model.PhaseStatus{"phase1": {Status: "in_progress"}})
	findings, _ := json.Marshal(map[string]interface{}{})

	// Insert only active instances
	for i := 1; i <= 3; i++ {
		wfiID := "wfi-active-" + string(rune(i+'0'))
		wfID := "test-workflow-" + string(rune(i+'0'))
		ticketID := "TKT-" + string(rune(i+'0'))

		wi := &model.WorkflowInstance{
			ID:         wfiID,
			ProjectID:  "test-project",
			TicketID:   ticketID,
			WorkflowID: wfID,
			ScopeType:  "ticket",
			Status:     model.WorkflowInstanceActive,
			PhaseOrder: string(phaseOrder),
			Phases:     string(phases),
			Findings:   string(findings),
		}
		if err := repo.Create(wi); err != nil {
			t.Fatalf("failed to create instance: %v", err)
		}
	}

	// Cleanup should not delete any active instances
	deleted, err := repo.CleanupKeepLatest(1)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted (all active), got %d", deleted)
	}

	var count int
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count instances: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 remaining instances, got %d", count)
	}
}
