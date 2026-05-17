package repo

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func TestCleanupKeepLatestForProject_CaseInsensitiveProjectID(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	findings, _ := json.Marshal(map[string]interface{}{})
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"Proj-Case", "Case Project", "/tmp/case"); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"wf-case", "Proj-Case", "WF", "ticket"); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())
	now := time.Now().UTC()

	for i := 0; i < 3; i++ {
		updatedAt := now.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		id := fmt.Sprintf("wfi-case-%d", i)
		if _, err := pool.Exec(
			`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, "Proj-Case", fmt.Sprintf("TKT-%d", i), "wf-case",
			model.WorkflowInstanceCompleted, "ticket", string(findings), updatedAt, updatedAt,
		); err != nil {
			t.Fatalf("seed instance %d: %v", i, err)
		}
	}

	// Pass lowercase — LOWER() in the query should match "Proj-Case".
	deleted, err := repo.CleanupKeepLatestForProject("proj-case", 1)
	if err != nil {
		t.Fatalf("CleanupKeepLatestForProject(lowercase): %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
}

func TestCleanupKeepLatestForProject_LargeScale(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	findings, _ := json.Marshal(map[string]interface{}{})
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"proj-big", "Big Project", "/tmp/big"); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"wf-big", "proj-big", "WF", "ticket"); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())
	base := time.Now().UTC()

	for i := 0; i < 1500; i++ {
		updatedAt := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		id := fmt.Sprintf("wfi-big-%04d", i)
		if _, err := pool.Exec(
			`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, "proj-big", fmt.Sprintf("TKT-%d", i), "wf-big",
			model.WorkflowInstanceCompleted, "ticket", string(findings), updatedAt, updatedAt,
		); err != nil {
			t.Fatalf("seed instance %d: %v", i, err)
		}
	}

	// keep >= total → 0 deletions (simulates cleanup disabled).
	deleted, err := repo.CleanupKeepLatestForProject("proj-big", 2000)
	if err != nil {
		t.Fatalf("CleanupKeepLatestForProject(keep=2000): %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted with keep=2000 = %d, want 0", deleted)
	}

	// keep=1000 → exactly 500 deletions (oldest 500 gone, newest 1000 kept).
	deleted, err = repo.CleanupKeepLatestForProject("proj-big", 1000)
	if err != nil {
		t.Fatalf("CleanupKeepLatestForProject(keep=1000): %v", err)
	}
	if deleted != 500 {
		t.Errorf("deleted = %d, want 500", deleted)
	}

	var oldestExists bool
	if err := pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, "wfi-big-0000").Scan(&oldestExists); err != nil {
		t.Fatalf("check oldest: %v", err)
	}
	if oldestExists {
		t.Error("wfi-big-0000 (oldest) should have been deleted")
	}

	var newestExists bool
	if err := pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, "wfi-big-1499").Scan(&newestExists); err != nil {
		t.Fatalf("check newest: %v", err)
	}
	if !newestExists {
		t.Error("wfi-big-1499 (newest) should remain")
	}
}
