package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// --- Delete ---

func TestCLIModel_Delete(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "del-model",
		CLIType:     "claude",
		DisplayName: "Delete Me",
		MappedModel: "sonnet",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete("del-model"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Subsequent Get returns not-found.
	_, err := svc.Get("del-model")
	if err == nil {
		t.Fatal("expected not-found after Delete, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestCLIModel_DeleteReadonly(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	err := svc.Delete("opus_4_7")
	if err == nil {
		t.Fatal("expected error deleting readonly model, got nil")
	}
	if !strings.Contains(err.Error(), "cannot delete system model") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "cannot delete system model")
	}

	// Model still exists.
	if _, err := svc.Get("opus_4_7"); err != nil {
		t.Errorf("model should still exist after failed delete: %v", err)
	}
}

func TestCLIModel_DeleteNotFound(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	err := svc.Delete("nonexistent-model")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestCLIModel_DeleteCaseInsensitive(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "case-del",
		CLIType:     "codex",
		DisplayName: "Case Delete",
		MappedModel: "gpt-4",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete with uppercase should work (case-insensitive).
	if err := svc.Delete("CASE-DEL"); err != nil {
		t.Fatalf("Delete with uppercase: %v", err)
	}

	_, err := svc.Get("case-del")
	if err == nil {
		t.Fatal("expected not-found after Delete, got nil")
	}
}

// --- IsValidModel ---

func TestCLIModel_IsValidModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	ok, err := svc.IsValidModel("opus_4_7")
	if err != nil {
		t.Fatalf("IsValidModel: %v", err)
	}
	if !ok {
		t.Error("IsValidModel(opus_4_7) = false, want true")
	}
}

func TestCLIModel_IsValidModelCaseInsensitive(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	ok, err := svc.IsValidModel("SONNET")
	if err != nil {
		t.Fatalf("IsValidModel: %v", err)
	}
	if !ok {
		t.Error("IsValidModel(SONNET) = false, want true")
	}
}

func TestCLIModel_IsValidModelNotFound(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	ok, err := svc.IsValidModel("nonexistent-model")
	if err != nil {
		t.Fatalf("IsValidModel: %v", err)
	}
	if ok {
		t.Error("IsValidModel(nonexistent-model) = true, want false")
	}
}

func TestCLIModel_IsValidModelAfterCreate(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "new-valid",
		CLIType:     "opencode",
		DisplayName: "New Valid",
		MappedModel: "openai/gpt-4",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ok, err := svc.IsValidModel("new-valid")
	if err != nil {
		t.Fatalf("IsValidModel: %v", err)
	}
	if !ok {
		t.Error("IsValidModel(new-valid) = false, want true after Create")
	}
}

func TestCLIModel_IsValidModelAfterDelete(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "to-delete",
		CLIType:     "claude",
		DisplayName: "To Delete",
		MappedModel: "haiku",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete("to-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	ok, err := svc.IsValidModel("to-delete")
	if err != nil {
		t.Fatalf("IsValidModel after delete: %v", err)
	}
	if ok {
		t.Error("IsValidModel(to-delete) = true after Delete, want false")
	}
}
