package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/types"
)

// Tests for validateFilePath and Create/Update with file_path field.

func TestValidateFilePath_Empty(t *testing.T) {
	if err := validateFilePath(""); err != nil {
		t.Errorf("validateFilePath(\"\") = %v, want nil", err)
	}
}

func TestValidateFilePath_Relative(t *testing.T) {
	err := validateFilePath("relative/script.py")
	if err == nil {
		t.Fatal("validateFilePath(relative) = nil, want error")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error = %q, want to contain \"absolute\"", err.Error())
	}
}

func TestValidateFilePath_NonExistent(t *testing.T) {
	err := validateFilePath("/nonexistent/no/such/file.py")
	if err == nil {
		t.Fatal("validateFilePath(non-existent) = nil, want error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want to contain \"does not exist\"", err.Error())
	}
}

func TestValidateFilePath_Directory(t *testing.T) {
	dir := t.TempDir()
	pyDir := filepath.Join(dir, "script.py")
	if err := os.Mkdir(pyDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := validateFilePath(pyDir)
	if err == nil {
		t.Fatal("validateFilePath(directory) = nil, want error")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Errorf("error = %q, want to contain \"regular file\"", err.Error())
	}
}

func TestValidateFilePath_NotPy(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(f, []byte("#!/bin/sh"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	err := validateFilePath(f)
	if err == nil {
		t.Fatal("validateFilePath(.sh) = nil, want error")
	}
	if !strings.Contains(err.Error(), ".py") {
		t.Errorf("error = %q, want to contain \".py\"", err.Error())
	}
}

func TestValidateFilePath_Valid(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "script.py")
	if err := os.WriteFile(f, []byte("print('hi')"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := validateFilePath(f); err != nil {
		t.Errorf("validateFilePath(valid) = %v, want nil", err)
	}
}

func TestPythonScriptService_CreateWithFilePath(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	dir := t.TempDir()
	pyFile := filepath.Join(dir, "script.py")
	if err := os.WriteFile(pyFile, []byte("print('from file')"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:     "FileScript",
		FilePath: &pyFile,
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if script.FilePath != pyFile {
		t.Errorf("FilePath = %q, want %q", script.FilePath, pyFile)
	}

	got, err := svc.Get(projectID, script.ID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.FilePath != pyFile {
		t.Errorf("Get() FilePath = %q, want %q", got.FilePath, pyFile)
	}
}

func TestPythonScriptService_CreateRejectsRelativeFilePath(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	rel := "relative/script.py"
	_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:     "Bad",
		FilePath: &rel,
	})
	if err == nil {
		t.Error("Create() with relative path expected error, got nil")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error = %q, want to contain \"absolute\"", err.Error())
	}
}

func TestPythonScriptService_CreateRejectsNonExistentFilePath(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	bad := "/no/such/file.py"
	_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:     "Bad",
		FilePath: &bad,
	})
	if err == nil {
		t.Error("Create() with missing file expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want to contain \"does not exist\"", err.Error())
	}
}

func TestPythonScriptService_CreateRejectsNonPyFilePath(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	dir := t.TempDir()
	f := filepath.Join(dir, "script.txt")
	if err := os.WriteFile(f, []byte("text"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:     "Bad",
		FilePath: &f,
	})
	if err == nil {
		t.Error("Create() with .txt path expected error, got nil")
	}
	if !strings.Contains(err.Error(), ".py") {
		t.Errorf("error = %q, want to contain \".py\"", err.Error())
	}
}

func TestPythonScriptService_UpdateFilePathRoundTrip(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{Name: "Script"})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	dir := t.TempDir()
	pyFile := filepath.Join(dir, "v2.py")
	if err := os.WriteFile(pyFile, []byte("x=2"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := svc.Update(projectID, script.ID, &types.PythonScriptUpdateRequest{FilePath: &pyFile}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	got, err := svc.Get(projectID, script.ID)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.FilePath != pyFile {
		t.Errorf("FilePath = %q, want %q", got.FilePath, pyFile)
	}
}

func TestPythonScriptService_UpdateClearsFilePath(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	dir := t.TempDir()
	pyFile := filepath.Join(dir, "script.py")
	if err := os.WriteFile(pyFile, []byte("x=1"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:     "Script",
		FilePath: &pyFile,
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	empty := ""
	if err := svc.Update(projectID, script.ID, &types.PythonScriptUpdateRequest{FilePath: &empty}); err != nil {
		t.Fatalf("Update() clear file_path: %v", err)
	}

	got, err := svc.Get(projectID, script.ID)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.FilePath != "" {
		t.Errorf("FilePath = %q, want empty after clear", got.FilePath)
	}
}

func TestPythonScriptService_UpdateRejectsInvalidFilePath(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{Name: "Script"})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	rel := "relative/script.py"
	err = svc.Update(projectID, script.ID, &types.PythonScriptUpdateRequest{FilePath: &rel})
	if err == nil {
		t.Error("Update() with relative path expected error, got nil")
	}
}
