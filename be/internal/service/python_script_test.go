package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

func setupPythonScriptSvc(t *testing.T) (*PythonScriptService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ps_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "proj-ps-svc"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'TestProject', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return NewPythonScriptService(pool, clock.Real()), projectID
}

func TestPythonScriptService_CreateAutoGeneratesID(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "My Script",
		Code: "print('hi')",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if script.ID == "" {
		t.Error("Create() ID is empty, want auto-generated")
	}
	if !strings.HasPrefix(script.ID, "ps-") {
		t.Errorf("ID = %q, want prefix ps-", script.ID)
	}
}

func TestPythonScriptService_CreateRequiresName(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "",
		Code: "print('hi')",
	})
	if err == nil {
		t.Error("Create() with empty name expected error, got nil")
	}
}

func TestPythonScriptService_CreateRoundTrip(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	created, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:        "Round Trip",
		Description: "test desc",
		Code:        "x = 1",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	got, err := svc.Get(projectID, created.ID)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Name != "Round Trip" {
		t.Errorf("Name = %q, want %q", got.Name, "Round Trip")
	}
	if got.Description != "test desc" {
		t.Errorf("Description = %q, want %q", got.Description, "test desc")
	}
	if got.Code != "x = 1" {
		t.Errorf("Code = %q, want %q", got.Code, "x = 1")
	}
	if got.ProjectID != strings.ToLower(projectID) {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, strings.ToLower(projectID))
	}
}

func TestPythonScriptService_ListReturnsEmptySlice(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if list == nil {
		t.Error("List() = nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("List() = %d items, want 0", len(list))
	}
}

func TestPythonScriptService_ListOrderedByName(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	for _, name := range []string{"Zebra", "Alpha", "Mango"} {
		if _, err := svc.Create(projectID, &types.PythonScriptCreateRequest{Name: name}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List() = %d items, want 3", len(list))
	}
	if list[0].Name != "Alpha" {
		t.Errorf("list[0].Name = %q, want Alpha", list[0].Name)
	}
	if list[2].Name != "Zebra" {
		t.Errorf("list[2].Name = %q, want Zebra", list[2].Name)
	}
}

func TestPythonScriptService_UpdateEmptyNameError(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	created, err := svc.Create(projectID, &types.PythonScriptCreateRequest{Name: "Original"})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	empty := ""
	err = svc.Update(projectID, created.ID, &types.PythonScriptUpdateRequest{Name: &empty})
	if err == nil {
		t.Error("Update() with empty name expected error, got nil")
	}
}

func TestPythonScriptService_UpdateProjectScoped(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	created, err := svc.Create(projectID, &types.PythonScriptCreateRequest{Name: "Script"})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	newName := "Updated"
	if err := svc.Update(projectID, created.ID, &types.PythonScriptUpdateRequest{Name: &newName}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got, err := svc.Get(projectID, created.ID)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("Name = %q, want Updated", got.Name)
	}
}

func TestPythonScriptService_UpdateRefusesOtherProject(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ps_xproj.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, pid := range []string{"proj-owner", "proj-other"} {
		if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, pid); err != nil {
			t.Fatalf("seed %s: %v", pid, err)
		}
	}

	svc := NewPythonScriptService(pool, clock.Real())
	created, err := svc.Create("proj-owner", &types.PythonScriptCreateRequest{Name: "Script"})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	newName := "Hacked"
	err = svc.Update("proj-other", created.ID, &types.PythonScriptUpdateRequest{Name: &newName})
	if err == nil {
		t.Error("Update() from wrong project expected error, got nil")
	}
}

func TestPythonScriptService_DeleteRefusesOtherProject(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ps_xdel.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, pid := range []string{"proj-owner", "proj-other"} {
		if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, pid); err != nil {
			t.Fatalf("seed %s: %v", pid, err)
		}
	}

	svc := NewPythonScriptService(pool, clock.Real())
	created, err := svc.Create("proj-owner", &types.PythonScriptCreateRequest{Name: "Script"})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	err = svc.Delete("proj-other", created.ID)
	if err == nil {
		t.Error("Delete() from wrong project expected error, got nil")
	}
}

func TestPythonScriptService_GetNotFound(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	_, err := svc.Get(projectID, "ps-doesnotexist")
	if err == nil {
		t.Error("Get() expected error for missing script, got nil")
	}
}

func TestPythonScriptService_DeleteNotFound(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	err := svc.Delete(projectID, "ps-missing")
	if err == nil {
		t.Error("Delete() expected error for missing script, got nil")
	}
}
