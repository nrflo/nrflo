package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/types"
)

func setupPythonScriptTestDB(t *testing.T) (*PythonScriptRepo, string) {
	t.Helper()
	pool := newTestPool(t)

	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-ps', 'TestProject', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return NewPythonScriptRepo(pool, clock.Real()), "proj-ps"
}

func makePythonScript(projectID, id, name string) *model.PythonScript {
	return &model.PythonScript{
		ID:          id,
		ProjectID:   projectID,
		Name:        name,
		Description: "Test description",
		Code:        "print('hello')",
	}
}

func TestPythonScriptRepo_CreateAndGet(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	script := makePythonScript(projectID, "ps-abc123", "My Script")
	if err := r.Create(script); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := r.Get(projectID, "ps-abc123")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.ID != "ps-abc123" {
		t.Errorf("ID = %q, want %q", got.ID, "ps-abc123")
	}
	if got.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.Name != "My Script" {
		t.Errorf("Name = %q, want %q", got.Name, "My Script")
	}
	if got.Description != "Test description" {
		t.Errorf("Description = %q, want %q", got.Description, "Test description")
	}
	if got.Code != "print('hello')" {
		t.Errorf("Code = %q, want %q", got.Code, "print('hello')")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestPythonScriptRepo_GetNotFound(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	_, err := r.Get(projectID, "ps-doesnotexist")
	if err == nil {
		t.Error("Get() expected error for missing script, got nil")
	}
}

func TestPythonScriptRepo_GetCrossProject(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-a', 'A', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed proj-a: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-b', 'B', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed proj-b: %v", err)
	}

	r := NewPythonScriptRepo(pool, clock.Real())
	script := makePythonScript("proj-a", "ps-xproj", "Script")
	if err := r.Create(script); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	_, err := r.Get("proj-b", "ps-xproj")
	if err == nil {
		t.Error("Get() from wrong project expected error, got nil")
	}
}

func TestPythonScriptRepo_ListOrderedByName(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	names := []string{"Zebra", "Alpha", "Mango"}
	for i, name := range names {
		s := makePythonScript(projectID, "ps-"+string(rune('a'+i)), name)
		if err := r.Create(s); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}

	list, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List() = %d items, want 3", len(list))
	}
	if list[0].Name != "Alpha" {
		t.Errorf("list[0].Name = %q, want Alpha (ASC order)", list[0].Name)
	}
	if list[1].Name != "Mango" {
		t.Errorf("list[1].Name = %q, want Mango", list[1].Name)
	}
	if list[2].Name != "Zebra" {
		t.Errorf("list[2].Name = %q, want Zebra", list[2].Name)
	}
}

func TestPythonScriptRepo_ListEmpty(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	list, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if list != nil {
		t.Errorf("List() on empty = %v, want nil (no rows)", list)
	}
}

func TestPythonScriptRepo_Update(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	script := makePythonScript(projectID, "ps-upd", "Original")
	if err := r.Create(script); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	newName := "Updated"
	newCode := "print('updated')"
	req := &types.PythonScriptUpdateRequest{
		Name: &newName,
		Code: &newCode,
	}
	if err := r.Update(projectID, "ps-upd", req); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got, err := r.Get(projectID, "ps-upd")
	if err != nil {
		t.Fatalf("Get() after Update(): %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated")
	}
	if got.Code != "print('updated')" {
		t.Errorf("Code = %q, want %q", got.Code, "print('updated')")
	}
	// Description unchanged since not in update request
	if got.Description != "Test description" {
		t.Errorf("Description = %q, want unchanged %q", got.Description, "Test description")
	}
}

func TestPythonScriptRepo_UpdateCrossProject(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	for _, pid := range []string{"proj-a", "proj-b"} {
		if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, pid); err != nil {
			t.Fatalf("seed %s: %v", pid, err)
		}
	}
	r := NewPythonScriptRepo(pool, clock.Real())
	if err := r.Create(makePythonScript("proj-a", "ps-xupd", "Script")); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	newName := "Hacked"
	err := r.Update("proj-b", "ps-xupd", &types.PythonScriptUpdateRequest{Name: &newName})
	if err == nil {
		t.Error("Update() from wrong project expected error, got nil")
	}
}

func TestPythonScriptRepo_Delete(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	if err := r.Create(makePythonScript(projectID, "ps-del", "Script")); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	if err := r.Delete(projectID, "ps-del"); err != nil {
		t.Fatalf("Delete(): %v", err)
	}

	_, err := r.Get(projectID, "ps-del")
	if err == nil {
		t.Error("Get() after Delete() expected error, got nil")
	}
}

func TestPythonScriptRepo_DeleteNotFound(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	err := r.Delete(projectID, "ps-missing")
	if err == nil {
		t.Error("Delete() on missing script expected error, got nil")
	}
}

func TestPythonScriptRepo_DeleteCrossProject(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	for _, pid := range []string{"proj-a", "proj-b"} {
		if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, pid); err != nil {
			t.Fatalf("seed %s: %v", pid, err)
		}
	}
	r := NewPythonScriptRepo(pool, clock.Real())
	if err := r.Create(makePythonScript("proj-a", "ps-xdel", "Script")); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	err := r.Delete("proj-b", "ps-xdel")
	if err == nil {
		t.Error("Delete() from wrong project expected error, got nil")
	}
}

func TestPythonScriptRepo_UpdateNoFields(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)
	if err := r.Create(makePythonScript(projectID, "ps-noop", "Script")); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	// Empty update request should be a no-op, not an error.
	if err := r.Update(projectID, "ps-noop", &types.PythonScriptUpdateRequest{}); err != nil {
		t.Errorf("Update() with no fields: unexpected error: %v", err)
	}
}

func TestPythonScriptRepo_CaseInsensitiveID(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)
	if err := r.Create(makePythonScript(projectID, "ps-case", "Script")); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	// Get with uppercase ID should still find it.
	got, err := r.Get(projectID, "PS-CASE")
	if err != nil {
		t.Fatalf("Get() with uppercase ID: %v", err)
	}
	if got.ID != "ps-case" {
		t.Errorf("ID = %q, want %q", got.ID, "ps-case")
	}
}
