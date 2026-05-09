package repo

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
)

func setupEnvVarTestDB(t *testing.T) (*ProjectEnvVarRepo, string) {
	t.Helper()
	pool := newTestPool(t)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-ev', 'TestProject', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return NewProjectEnvVarRepo(pool, clock.Real()), "proj-ev"
}

func TestProjectEnvVarRepo_ListEmpty(t *testing.T) {
	t.Parallel()
	r, projectID := setupEnvVarTestDB(t)

	vars, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("List() = %d items, want 0", len(vars))
	}
}

func TestProjectEnvVarRepo_UpsertAndList(t *testing.T) {
	t.Parallel()
	r, projectID := setupEnvVarTestDB(t)

	v, err := r.Upsert(projectID, "MY_VAR", "hello")
	if err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}
	if v.Name != "MY_VAR" {
		t.Errorf("Name = %q, want MY_VAR", v.Name)
	}
	if v.Value != "hello" {
		t.Errorf("Value = %q, want hello", v.Value)
	}
	if v.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}

	list, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() = %d items, want 1", len(list))
	}
	if list[0].Name != "MY_VAR" || list[0].Value != "hello" {
		t.Errorf("list[0] = {%q, %q}, want {MY_VAR, hello}", list[0].Name, list[0].Value)
	}
}

func TestProjectEnvVarRepo_UpsertOverwrite(t *testing.T) {
	t.Parallel()
	r, projectID := setupEnvVarTestDB(t)

	if _, err := r.Upsert(projectID, "MY_VAR", "original"); err != nil {
		t.Fatalf("Upsert() first: %v", err)
	}
	if _, err := r.Upsert(projectID, "MY_VAR", "overwritten"); err != nil {
		t.Fatalf("Upsert() second: %v", err)
	}

	list, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() = %d items, want 1 after overwrite", len(list))
	}
	if list[0].Value != "overwritten" {
		t.Errorf("Value = %q, want overwritten", list[0].Value)
	}
}

func TestProjectEnvVarRepo_ListOrderedByName(t *testing.T) {
	t.Parallel()
	r, projectID := setupEnvVarTestDB(t)

	for _, name := range []string{"ZEBRA", "ALPHA", "MANGO"} {
		if _, err := r.Upsert(projectID, name, "value"); err != nil {
			t.Fatalf("Upsert(%s): %v", name, err)
		}
	}

	list, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List() = %d items, want 3", len(list))
	}
	if list[0].Name != "ALPHA" {
		t.Errorf("list[0].Name = %q, want ALPHA (ASC order)", list[0].Name)
	}
	if list[1].Name != "MANGO" {
		t.Errorf("list[1].Name = %q, want MANGO", list[1].Name)
	}
	if list[2].Name != "ZEBRA" {
		t.Errorf("list[2].Name = %q, want ZEBRA", list[2].Name)
	}
}

func TestProjectEnvVarRepo_Delete(t *testing.T) {
	t.Parallel()
	r, projectID := setupEnvVarTestDB(t)

	if _, err := r.Upsert(projectID, "TO_DELETE", "value"); err != nil {
		t.Fatalf("Upsert(): %v", err)
	}
	if err := r.Delete(projectID, "TO_DELETE"); err != nil {
		t.Fatalf("Delete(): %v", err)
	}

	list, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List() after Delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() after Delete = %d items, want 0", len(list))
	}
}

func TestProjectEnvVarRepo_DeleteNotFound(t *testing.T) {
	t.Parallel()
	r, projectID := setupEnvVarTestDB(t)

	err := r.Delete(projectID, "MISSING")
	if err == nil {
		t.Error("Delete() on missing var expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Delete() error = %q, want 'not found'", err.Error())
	}
}

func TestProjectEnvVarRepo_CaseInsensitiveProjectID(t *testing.T) {
	t.Parallel()
	r, projectID := setupEnvVarTestDB(t)

	if _, err := r.Upsert(projectID, "CASE_VAR", "value"); err != nil {
		t.Fatalf("Upsert(): %v", err)
	}

	// List with uppercase project ID should still find it.
	list, err := r.List(strings.ToUpper(projectID))
	if err != nil {
		t.Fatalf("List() with uppercase project ID: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() = %d items, want 1 (case-insensitive)", len(list))
	}

	// Delete with uppercase project ID should work.
	if err := r.Delete(strings.ToUpper(projectID), "CASE_VAR"); err != nil {
		t.Fatalf("Delete() with uppercase project ID: %v", err)
	}
}

func TestProjectEnvVarRepo_CrossProject(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	for _, pid := range []string{"proj-a-ev", "proj-b-ev"} {
		if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
			VALUES (?, 'P', datetime('now'), datetime('now'))`, pid); err != nil {
			t.Fatalf("seed %s: %v", pid, err)
		}
	}
	r := NewProjectEnvVarRepo(pool, clock.Real())

	if _, err := r.Upsert("proj-a-ev", "SHARED_NAME", "project-a-value"); err != nil {
		t.Fatalf("Upsert proj-a: %v", err)
	}

	// List for proj-b should not show proj-a vars.
	listB, err := r.List("proj-b-ev")
	if err != nil {
		t.Fatalf("List(proj-b): %v", err)
	}
	if len(listB) != 0 {
		t.Errorf("List(proj-b) = %d items, want 0", len(listB))
	}

	// Delete from wrong project should fail.
	if err := r.Delete("proj-b-ev", "SHARED_NAME"); err == nil {
		t.Error("Delete() from wrong project expected error, got nil")
	}
}

func TestProjectEnvVarRepo_CascadeOnProjectDelete(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-cascade-ev', 'Cascade', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	r := NewProjectEnvVarRepo(pool, clock.Real())

	for _, name := range []string{"VAR_ONE", "VAR_TWO"} {
		if _, err := r.Upsert("proj-cascade-ev", name, "value"); err != nil {
			t.Fatalf("Upsert(%s): %v", name, err)
		}
	}

	if _, err := pool.Exec(`DELETE FROM projects WHERE id = 'proj-cascade-ev'`); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	var count int
	row := pool.QueryRow(`SELECT COUNT(*) FROM project_env_vars WHERE project_id = 'proj-cascade-ev'`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count env vars: %v", err)
	}
	if count != 0 {
		t.Errorf("env var count after project delete = %d, want 0 (cascade)", count)
	}
}

func TestProjectEnvVarRepo_UpdatedAtChangesOnOverwrite(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-ts-ev', 'TS', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewProjectEnvVarRepo(pool, clk)

	v1, err := r.Upsert("proj-ts-ev", "TS_VAR", "first")
	if err != nil {
		t.Fatalf("Upsert first: %v", err)
	}

	clk.Advance(time.Hour)

	v2, err := r.Upsert("proj-ts-ev", "TS_VAR", "second")
	if err != nil {
		t.Fatalf("Upsert second: %v", err)
	}

	if !v2.UpdatedAt.After(v1.UpdatedAt) {
		t.Errorf("UpdatedAt not advanced after overwrite: v1=%v v2=%v", v1.UpdatedAt, v2.UpdatedAt)
	}
}

func TestProjectEnvVarRepo_LowercaseProjectIDStored(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-lower-ev', 'Lower', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	r := NewProjectEnvVarRepo(pool, clock.Real())

	v, err := r.Upsert("PROJ-LOWER-EV", "LC_VAR", "value")
	if err != nil {
		t.Fatalf("Upsert(): %v", err)
	}
	if v.ProjectID != "proj-lower-ev" {
		t.Errorf("ProjectID = %q, want lowercase %q", v.ProjectID, "proj-lower-ev")
	}
}
