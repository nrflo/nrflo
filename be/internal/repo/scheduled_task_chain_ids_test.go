package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestScheduledTaskRepo_Create_Get_WorkflowChainIDs verifies that WorkflowChainIDs
// round-trips through Create → Get correctly.
func TestScheduledTaskRepo_Create_Get_WorkflowChainIDs(t *testing.T) {
	t.Parallel()
	r, projectID := setupScheduledTaskDB(t)

	task := &model.ScheduledTask{
		ID:               "chain-task-cg",
		ProjectID:        projectID,
		Name:             "chain-task",
		CronExpression:   "0 * * * *",
		Workflows:        []string{"feature"},
		WorkflowChainIDs: []string{"chain-1", "chain-2"},
		Enabled:          true,
	}
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("chain-task-cg")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(got.WorkflowChainIDs) != 2 {
		t.Fatalf("WorkflowChainIDs len = %d, want 2", len(got.WorkflowChainIDs))
	}
	if got.WorkflowChainIDs[0] != "chain-1" || got.WorkflowChainIDs[1] != "chain-2" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-1 chain-2]", got.WorkflowChainIDs)
	}
}

// TestScheduledTaskRepo_Create_NilChainIDs_DefaultsEmpty verifies that a nil
// WorkflowChainIDs becomes an empty (non-nil) slice after round-trip.
func TestScheduledTaskRepo_Create_NilChainIDs_DefaultsEmpty(t *testing.T) {
	t.Parallel()
	r, projectID := setupScheduledTaskDB(t)

	task := &model.ScheduledTask{
		ID:               "nil-chain-task",
		ProjectID:        projectID,
		Name:             "nil chains",
		CronExpression:   "0 * * * *",
		Workflows:        []string{"feature"},
		WorkflowChainIDs: nil,
		Enabled:          true,
	}
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("nil-chain-task")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WorkflowChainIDs == nil {
		t.Error("WorkflowChainIDs = nil, want empty slice")
	}
	if len(got.WorkflowChainIDs) != 0 {
		t.Errorf("WorkflowChainIDs len = %d, want 0", len(got.WorkflowChainIDs))
	}
}

// TestScheduledTaskRepo_Update_WorkflowChainIDs verifies that Update persists
// WorkflowChainIDs changes including clearing them.
func TestScheduledTaskRepo_Update_WorkflowChainIDs(t *testing.T) {
	t.Parallel()
	r, projectID := setupScheduledTaskDB(t)

	task := makeTask("upd-chain-task", projectID, true, []string{"feature"})
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Initially no chain IDs — verify default is empty.
	got, err := r.Get("upd-chain-task")
	if err != nil {
		t.Fatalf("Get initial: %v", err)
	}
	if len(got.WorkflowChainIDs) != 0 {
		t.Errorf("initial WorkflowChainIDs = %v, want empty", got.WorkflowChainIDs)
	}

	// Update to add chain IDs.
	task.WorkflowChainIDs = []string{"chain-a", "chain-b", "chain-c"}
	if err := r.Update(task); err != nil {
		t.Fatalf("Update (add chains): %v", err)
	}
	got, err = r.Get("upd-chain-task")
	if err != nil {
		t.Fatalf("Get after add: %v", err)
	}
	if len(got.WorkflowChainIDs) != 3 {
		t.Fatalf("WorkflowChainIDs len = %d, want 3", len(got.WorkflowChainIDs))
	}
	if got.WorkflowChainIDs[0] != "chain-a" {
		t.Errorf("WorkflowChainIDs[0] = %q, want chain-a", got.WorkflowChainIDs[0])
	}

	// Update to clear chain IDs.
	task.WorkflowChainIDs = []string{}
	if err := r.Update(task); err != nil {
		t.Fatalf("Update (clear chains): %v", err)
	}
	got2, err := r.Get("upd-chain-task")
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if len(got2.WorkflowChainIDs) != 0 {
		t.Errorf("WorkflowChainIDs after clear = %v, want empty", got2.WorkflowChainIDs)
	}
}

// TestScheduledTaskRepo_ListEnabled_IncludesChainIDs verifies that ListEnabled
// returns WorkflowChainIDs correctly.
func TestScheduledTaskRepo_ListEnabled_IncludesChainIDs(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-chain-le', 'T', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	r := NewScheduledTaskRepo(database, clock.Real())

	task := &model.ScheduledTask{
		ID:               "enabled-chain-task",
		ProjectID:        "proj-chain-le",
		Name:             "chain task",
		CronExpression:   "0 0 * * *",
		Workflows:        []string{"feature"},
		WorkflowChainIDs: []string{"chain-x", "chain-y"},
		Enabled:          true,
	}
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	enabled, err := r.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("ListEnabled count = %d, want 1", len(enabled))
	}
	if len(enabled[0].WorkflowChainIDs) != 2 {
		t.Fatalf("ListEnabled[0].WorkflowChainIDs len = %d, want 2", len(enabled[0].WorkflowChainIDs))
	}
	if enabled[0].WorkflowChainIDs[0] != "chain-x" || enabled[0].WorkflowChainIDs[1] != "chain-y" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-x chain-y]", enabled[0].WorkflowChainIDs)
	}
}

// TestScheduledTaskRepo_Create_ChainIDsOnlyNoWorkflows verifies that a task can
// be created with chain IDs and no workflows at the repo level.
func TestScheduledTaskRepo_Create_ChainIDsOnlyNoWorkflows(t *testing.T) {
	t.Parallel()
	r, projectID := setupScheduledTaskDB(t)

	task := &model.ScheduledTask{
		ID:               "chains-only-task",
		ProjectID:        projectID,
		Name:             "chains only",
		CronExpression:   "*/5 * * * *",
		Workflows:        []string{},
		WorkflowChainIDs: []string{"chain-only-1"},
		Enabled:          true,
	}
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("chains-only-task")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Workflows) != 0 {
		t.Errorf("Workflows = %v, want empty", got.Workflows)
	}
	if len(got.WorkflowChainIDs) != 1 || got.WorkflowChainIDs[0] != "chain-only-1" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-only-1]", got.WorkflowChainIDs)
	}
}
