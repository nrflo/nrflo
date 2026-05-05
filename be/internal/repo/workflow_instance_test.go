package repo

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func TestUpdateStatusToProjectCompleted(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	var err error
	// Create project first
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflow definition
	_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"test-workflow", "test-project", "Test Workflow", "project")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())

	// Create a workflow instance
	findings, _ := json.Marshal(map[string]interface{}{})

	wi := &model.WorkflowInstance{
		ID:         "test-wfi",
		ProjectID:  "test-project",
		TicketID:   "",
		WorkflowID: "test-workflow",
		ScopeType:  "project",
		Status:     model.WorkflowInstanceActive,
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

func TestCreate_ScheduledTaskID_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("with_scheduled_task_id", func(t *testing.T) {
		t.Parallel()
		pool := newTestPool(t)

		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"proj-sched", "Sched Project", t.TempDir(), now, now)
		if err != nil {
			t.Fatalf("seed project: %v", err)
		}
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"wf-sched", "proj-sched", "Sched Workflow", "project", now, now)
		if err != nil {
			t.Fatalf("seed workflow: %v", err)
		}
		_, err = pool.Exec(`INSERT INTO scheduled_tasks (id, project_id, name, description, cron_expression, workflows, enabled, created_at, updated_at)
			VALUES (?, ?, ?, '', '* * * * *', '[]', 1, ?, ?)`,
			"task-abc", "proj-sched", "task-abc", now, now)
		if err != nil {
			t.Fatalf("seed scheduled_task: %v", err)
		}

		repo := NewWorkflowInstanceRepo(pool, clock.Real())
		wi := &model.WorkflowInstance{
			ID:              "wfi-sched-1",
			ProjectID:       "proj-sched",
			WorkflowID:      "wf-sched",
			ScopeType:       "project",
			Status:          model.WorkflowInstanceActive,
			Findings:        "{}",
			ScheduledTaskID: "task-abc",
		}
		if err := repo.Create(wi); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := repo.Get("wfi-sched-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ScheduledTaskID != "task-abc" {
			t.Errorf("ScheduledTaskID = %q, want %q", got.ScheduledTaskID, "task-abc")
		}
	})

	t.Run("empty_scheduled_task_id_is_null", func(t *testing.T) {
		t.Parallel()
		pool := newTestPool(t)

		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"proj-nosched", "No Sched", t.TempDir(), now, now)
		if err != nil {
			t.Fatalf("seed project: %v", err)
		}
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"wf-nosched", "proj-nosched", "No Sched Workflow", "project", now, now)
		if err != nil {
			t.Fatalf("seed workflow: %v", err)
		}

		repo := NewWorkflowInstanceRepo(pool, clock.Real())
		wi := &model.WorkflowInstance{
			ID:         "wfi-nosched-1",
			ProjectID:  "proj-nosched",
			WorkflowID: "wf-nosched",
			ScopeType:  "project",
			Status:     model.WorkflowInstanceActive,
			Findings:   "{}",
			// ScheduledTaskID intentionally empty
		}
		if err := repo.Create(wi); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Verify NULL via raw query
		var isNull bool
		row := pool.QueryRow(`SELECT scheduled_task_id IS NULL FROM workflow_instances WHERE id = ?`, "wfi-nosched-1")
		if scanErr := row.Scan(&isNull); scanErr != nil {
			t.Fatalf("raw NULL check: %v", scanErr)
		}
		if !isNull {
			t.Error("scheduled_task_id should be NULL when ScheduledTaskID is empty, but is not")
		}

		// Verify repo.Get returns empty string
		got, err := repo.Get("wfi-nosched-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ScheduledTaskID != "" {
			t.Errorf("ScheduledTaskID = %q, want empty string", got.ScheduledTaskID)
		}
	})
}

func TestListByProjectScopeIncludesAllStatuses(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	var err error
	projectID := "test-project"

	// Create project first
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		projectID, "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	repo := NewWorkflowInstanceRepo(pool, clock.Real())

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

	findings, _ := json.Marshal(map[string]interface{}{})

	for _, inst := range instances {
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
			inst.workflowID, projectID, "Test Workflow", "project")
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
