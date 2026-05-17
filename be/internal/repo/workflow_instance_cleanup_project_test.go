package repo

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func TestCleanupKeepLatestForProject_Basic(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	findings, _ := json.Marshal(map[string]interface{}{})

	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"proj-a", "Project A", "/tmp/a"); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"wf-a", "proj-a", "Workflow A", "ticket"); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		updatedAt := now.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		id := fmt.Sprintf("wfi-a-%d", i)
		if _, err := pool.Exec(
			`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, "proj-a", fmt.Sprintf("TKT-%d", i), "wf-a",
			model.WorkflowInstanceCompleted, "ticket", string(findings),
			updatedAt, updatedAt,
		); err != nil {
			t.Fatalf("seed instance %d: %v", i, err)
		}
	}

	deleted, err := repo.CleanupKeepLatestForProject("proj-a", 3)
	if err != nil {
		t.Fatalf("CleanupKeepLatestForProject() error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	for _, id := range []string{"wfi-a-0", "wfi-a-1"} {
		var exists bool
		if err := pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, id).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", id, err)
		}
		if exists {
			t.Errorf("%s should have been deleted", id)
		}
	}

	for _, id := range []string{"wfi-a-2", "wfi-a-3", "wfi-a-4"} {
		var exists bool
		if err := pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, id).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", id, err)
		}
		if !exists {
			t.Errorf("%s should remain", id)
		}
	}
}

func TestCleanupKeepLatestForProject_CrossProjectIsolation(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	findings, _ := json.Marshal(map[string]interface{}{})

	for _, proj := range []string{"proj-x", "proj-y"} {
		if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
			proj, proj, "/tmp/"+proj); err != nil {
			t.Fatalf("seed project %s: %v", proj, err)
		}
		if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
			"wf-"+proj, proj, "WF", "ticket"); err != nil {
			t.Fatalf("seed workflow for %s: %v", proj, err)
		}
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())
	now := time.Now().UTC()

	for _, proj := range []string{"proj-x", "proj-y"} {
		for i := 0; i < 5; i++ {
			updatedAt := now.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
			id := fmt.Sprintf("wfi-%s-%d", proj, i)
			if _, err := pool.Exec(
				`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, proj, fmt.Sprintf("TKT-%d", i), "wf-"+proj,
				model.WorkflowInstanceCompleted, "ticket", string(findings),
				updatedAt, updatedAt,
			); err != nil {
				t.Fatalf("seed %s instance %d: %v", proj, i, err)
			}
		}
	}

	deleted, err := repo.CleanupKeepLatestForProject("proj-x", 3)
	if err != nil {
		t.Fatalf("CleanupKeepLatestForProject(proj-x,3): %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted from proj-x = %d, want 2", deleted)
	}

	var yCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances WHERE project_id = ?`, "proj-y").Scan(&yCount); err != nil {
		t.Fatalf("count proj-y: %v", err)
	}
	if yCount != 5 {
		t.Errorf("proj-y count = %d, want 5 (no cross-project deletion)", yCount)
	}
}

func TestCleanupKeepLatestForProject_PreservesActive(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	findings, _ := json.Marshal(map[string]interface{}{})
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"proj-act", "Active Project", "/tmp/act"); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"wf-act", "proj-act", "WF", "ticket"); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())
	now := time.Now().UTC()

	statuses := []model.WorkflowInstanceStatus{
		model.WorkflowInstanceCompleted,
		model.WorkflowInstanceCompleted,
		model.WorkflowInstanceActive,
		model.WorkflowInstanceActive,
		model.WorkflowInstanceActive,
	}
	for i, status := range statuses {
		updatedAt := now.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		id := fmt.Sprintf("wfi-act-%d", i)
		if _, err := pool.Exec(
			`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, "proj-act", fmt.Sprintf("TKT-%d", i), "wf-act",
			status, "ticket", string(findings), updatedAt, updatedAt,
		); err != nil {
			t.Fatalf("seed instance %d: %v", i, err)
		}
	}

	deleted, err := repo.CleanupKeepLatestForProject("proj-act", 0)
	if err != nil {
		t.Fatalf("CleanupKeepLatestForProject(): %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2 (completed only)", deleted)
	}

	var activeCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances WHERE project_id = ? AND status = ?`,
		"proj-act", model.WorkflowInstanceActive).Scan(&activeCount); err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 3 {
		t.Errorf("active remaining = %d, want 3", activeCount)
	}
}

func TestCleanupKeepLatestForProject_EmptyProject(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	repo := NewWorkflowInstanceRepo(pool, clock.Real())

	deleted, err := repo.CleanupKeepLatestForProject("proj-nonexistent", 10)
	if err != nil {
		t.Fatalf("CleanupKeepLatestForProject(nonexistent): %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted from nonexistent project = %d, want 0", deleted)
	}
}
