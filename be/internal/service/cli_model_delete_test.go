package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// --- Delete ---

func TestCLIModel_Delete(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// --- IsValidModel ---

func TestCLIModel_IsValidModel(t *testing.T) {
	t.Parallel()
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

func TestCLIModel_IsValidModelNotFound(t *testing.T) {
	t.Parallel()
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

// --- Case-insensitive ID lookup across Get / Delete / IsValidModel ---

// TestCLIModel_CaseInsensitiveLookup verifies that Get, Delete, and IsValidModel all
// normalize the model ID to lowercase before lookup.
func TestCLIModel_CaseInsensitiveLookup(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// Get and IsValidModel against a seeded (lowercase) row via uppercase input.
	if m, err := svc.Get("OPUS_4_7"); err != nil {
		t.Fatalf("Get with uppercase: %v", err)
	} else if m.ID != "opus_4_7" {
		t.Errorf("Get ID = %q, want %q", m.ID, "opus_4_7")
	}
	if ok, err := svc.IsValidModel("SONNET"); err != nil {
		t.Fatalf("IsValidModel with uppercase: %v", err)
	} else if !ok {
		t.Error("IsValidModel(SONNET) = false, want true")
	}

	// Delete against a user-created row via uppercase input.
	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "case-del",
		CLIType:     "codex",
		DisplayName: "Case Delete",
		MappedModel: "gpt-4",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete("CASE-DEL"); err != nil {
		t.Fatalf("Delete with uppercase: %v", err)
	}
	if _, err := svc.Get("case-del"); err == nil {
		t.Fatal("expected not-found after Delete, got nil")
	}
}
