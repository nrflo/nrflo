package repo

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupScheduledTaskDB(t *testing.T) (*ScheduledTaskRepo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return NewScheduledTaskRepo(database, clock.Real()), "proj-1"
}

func makeTask(id, projectID string, enabled bool, workflows []string) *model.ScheduledTask {
	return &model.ScheduledTask{
		ID:             id,
		ProjectID:      projectID,
		Name:           "daily-run",
		Description:    "runs every day",
		CronExpression: "0 0 * * *",
		Workflows:      workflows,
		Enabled:        enabled,
	}
}

func TestScheduledTaskRepo_Create_Get(t *testing.T) {
	r, projectID := setupScheduledTaskDB(t)

	task := makeTask("task-1", projectID, true, []string{"a", "b"})
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("task-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != "task-1" {
		t.Errorf("ID = %q, want task-1", got.ID)
	}
	if got.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.Name != task.Name {
		t.Errorf("Name = %q, want %q", got.Name, task.Name)
	}
	if got.Description != task.Description {
		t.Errorf("Description = %q, want %q", got.Description, task.Description)
	}
	if got.CronExpression != task.CronExpression {
		t.Errorf("CronExpression = %q, want %q", got.CronExpression, task.CronExpression)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if len(got.Workflows) != 2 {
		t.Fatalf("Workflows len = %d, want 2", len(got.Workflows))
	}
	if got.Workflows[0] != "a" || got.Workflows[1] != "b" {
		t.Errorf("Workflows = %v, want [a b]", got.Workflows)
	}
	if got.LastTriggeredAt != nil {
		t.Errorf("LastTriggeredAt = %v, want nil", got.LastTriggeredAt)
	}
	if got.NextRunAt != nil {
		t.Errorf("NextRunAt = %v, want nil", got.NextRunAt)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt is zero")
	}
}

func TestScheduledTaskRepo_Create_Get_CaseInsensitive(t *testing.T) {
	r, projectID := setupScheduledTaskDB(t)

	task := makeTask("TASK-UPPER", projectID, true, []string{"w1"})
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// ID stored as lowercase; lookup is case-insensitive
	got, err := r.Get("task-upper")
	if err != nil {
		t.Fatalf("Get lower: %v", err)
	}
	if got.ID != "task-upper" {
		t.Errorf("ID = %q, want task-upper", got.ID)
	}

	got2, err := r.Get("TASK-UPPER")
	if err != nil {
		t.Fatalf("Get upper: %v", err)
	}
	if got2.ID != "task-upper" {
		t.Errorf("ID = %q, want task-upper", got2.ID)
	}
}

func TestScheduledTaskRepo_Get_NotFound(t *testing.T) {
	r, _ := setupScheduledTaskDB(t)

	_, err := r.Get("no-such-task")
	if err == nil {
		t.Fatalf("Get missing task: expected error, got nil")
	}
}

func TestScheduledTaskRepo_List_FiltersByProjectID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	for _, id := range []string{"proj-a", "proj-b"} {
		_, err = database.Exec(
			`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, id)
		if err != nil {
			t.Fatalf("insert project %s: %v", id, err)
		}
	}

	r := NewScheduledTaskRepo(database, clock.Real())

	for i, pid := range []string{"proj-a", "proj-a", "proj-b"} {
		task := makeTask("t-"+pid+"-"+string(rune('1'+i)), pid, true, []string{"w"})
		if err := r.Create(task); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	listA, err := r.List("proj-a")
	if err != nil {
		t.Fatalf("List proj-a: %v", err)
	}
	if len(listA) != 2 {
		t.Errorf("List proj-a count = %d, want 2", len(listA))
	}

	listB, err := r.List("proj-b")
	if err != nil {
		t.Fatalf("List proj-b: %v", err)
	}
	if len(listB) != 1 {
		t.Errorf("List proj-b count = %d, want 1", len(listB))
	}

	listC, err := r.List("proj-none")
	if err != nil {
		t.Fatalf("List proj-none: %v", err)
	}
	if len(listC) != 0 {
		t.Errorf("List proj-none count = %d, want 0", len(listC))
	}
}

func TestScheduledTaskRepo_Update_MutatesFields(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	r := NewScheduledTaskRepo(database, clk)
	task := makeTask("task-upd", "p1", false, []string{"old"})
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}
	originalUpdatedAt := task.UpdatedAt

	clk.Advance(time.Second)

	task.Name = "updated-name"
	task.Description = "new description"
	task.CronExpression = "*/5 * * * *"
	task.Workflows = []string{"x", "y", "z"}
	task.Enabled = true
	if err := r.Update(task); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.Get("task-upd")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("Name = %q, want updated-name", got.Name)
	}
	if got.Description != "new description" {
		t.Errorf("Description = %q, want new description", got.Description)
	}
	if got.CronExpression != "*/5 * * * *" {
		t.Errorf("CronExpression = %q, want */5 * * * *", got.CronExpression)
	}
	if len(got.Workflows) != 3 || got.Workflows[0] != "x" {
		t.Errorf("Workflows = %v, want [x y z]", got.Workflows)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("UpdatedAt %v not after original %v", got.UpdatedAt, originalUpdatedAt)
	}
}

func TestScheduledTaskRepo_Update_NotFound(t *testing.T) {
	r, _ := setupScheduledTaskDB(t)

	task := makeTask("no-such", "proj-1", true, []string{"w"})
	err := r.Update(task)
	if err == nil {
		t.Fatalf("Update non-existent: expected error, got nil")
	}
}

func TestScheduledTaskRepo_Delete(t *testing.T) {
	r, projectID := setupScheduledTaskDB(t)

	task := makeTask("task-del", projectID, true, []string{"w"})
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.Delete("task-del"); err != nil {
		t.Fatalf("Delete first call: %v", err)
	}

	err := r.Delete("task-del")
	if err == nil {
		t.Fatalf("Delete second call: expected error, got nil")
	}
}

func TestScheduledTaskRepo_ListEnabled_CrossProject(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	for _, id := range []string{"proj-x", "proj-y"} {
		_, err = database.Exec(
			`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, id)
		if err != nil {
			t.Fatalf("insert project %s: %v", id, err)
		}
	}

	r := NewScheduledTaskRepo(database, clock.Real())

	// proj-x: one enabled, one disabled
	if err := r.Create(makeTask("x-enabled", "proj-x", true, []string{"w1"})); err != nil {
		t.Fatalf("Create x-enabled: %v", err)
	}
	if err := r.Create(makeTask("x-disabled", "proj-x", false, []string{"w2"})); err != nil {
		t.Fatalf("Create x-disabled: %v", err)
	}

	// proj-y: one enabled, one disabled
	if err := r.Create(makeTask("y-enabled", "proj-y", true, []string{"w3"})); err != nil {
		t.Fatalf("Create y-enabled: %v", err)
	}
	if err := r.Create(makeTask("y-disabled", "proj-y", false, []string{"w4"})); err != nil {
		t.Fatalf("Create y-disabled: %v", err)
	}

	enabled, err := r.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 2 {
		t.Fatalf("ListEnabled count = %d, want 2", len(enabled))
	}

	seenIDs := map[string]bool{}
	for _, task := range enabled {
		seenIDs[task.ID] = true
		if !task.Enabled {
			t.Errorf("task %q: Enabled = false, want true", task.ID)
		}
	}
	if !seenIDs["x-enabled"] {
		t.Errorf("x-enabled missing from ListEnabled")
	}
	if !seenIDs["y-enabled"] {
		t.Errorf("y-enabled missing from ListEnabled")
	}
}

func TestScheduledTaskRepo_UpdateTriggerTimestamps(t *testing.T) {
	fixedTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	r := NewScheduledTaskRepo(database, clk)
	task := makeTask("ts-task", "p1", true, []string{"w"})
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Set concrete timestamps
	last := time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)
	next := time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC)
	if err := r.UpdateTriggerTimestamps("ts-task", &last, &next); err != nil {
		t.Fatalf("UpdateTriggerTimestamps with values: %v", err)
	}

	got, err := r.Get("ts-task")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastTriggeredAt == nil {
		t.Fatal("LastTriggeredAt is nil, want non-nil")
	}
	if !got.LastTriggeredAt.Equal(last) {
		t.Errorf("LastTriggeredAt = %v, want %v", got.LastTriggeredAt, last)
	}
	if got.NextRunAt == nil {
		t.Fatal("NextRunAt is nil, want non-nil")
	}
	if !got.NextRunAt.Equal(next) {
		t.Errorf("NextRunAt = %v, want %v", got.NextRunAt, next)
	}

	// Now clear both back to nil
	if err := r.UpdateTriggerTimestamps("ts-task", nil, nil); err != nil {
		t.Fatalf("UpdateTriggerTimestamps nil: %v", err)
	}

	got2, err := r.Get("ts-task")
	if err != nil {
		t.Fatalf("Get after nil update: %v", err)
	}
	if got2.LastTriggeredAt != nil {
		t.Errorf("LastTriggeredAt = %v, want nil", got2.LastTriggeredAt)
	}
	if got2.NextRunAt != nil {
		t.Errorf("NextRunAt = %v, want nil", got2.NextRunAt)
	}
}

func TestScheduledTaskRepo_UpdateTriggerTimestamps_NotFound(t *testing.T) {
	r, _ := setupScheduledTaskDB(t)

	err := r.UpdateTriggerTimestamps("no-such", nil, nil)
	if err == nil {
		t.Fatalf("UpdateTriggerTimestamps missing task: expected error, got nil")
	}
}

func TestScheduledTaskRepo_Create_WorkflowsEmptySlice(t *testing.T) {
	r, projectID := setupScheduledTaskDB(t)

	task := makeTask("task-empty-wf", projectID, true, []string{})
	if err := r.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("task-empty-wf")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Workflows == nil {
		t.Errorf("Workflows is nil, want empty slice")
	}
	if len(got.Workflows) != 0 {
		t.Errorf("Workflows len = %d, want 0", len(got.Workflows))
	}
}
