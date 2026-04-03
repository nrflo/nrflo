package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// --- Restore ---

func TestDefaultTemplate_Restore_Valid(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	original, err := svc.Get("setup-analyzer")
	if err != nil {
		t.Fatalf("Get before update: %v", err)
	}
	if original.DefaultTemplate == nil {
		t.Fatal("DefaultTemplate is nil, want non-nil for seeded readonly")
	}
	originalDefault := *original.DefaultTemplate

	newText := "completely different text"
	if err := svc.Update("setup-analyzer", &types.DefaultTemplateUpdateRequest{Template: &newText}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Verify edit persisted
	after, err := svc.Get("setup-analyzer")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if after.Template != "completely different text" {
		t.Fatalf("Template after update = %q, want %q", after.Template, "completely different text")
	}

	if err := svc.Restore("setup-analyzer"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restored, err := svc.Get("setup-analyzer")
	if err != nil {
		t.Fatalf("Get after restore: %v", err)
	}
	if restored.Template != originalDefault {
		t.Errorf("Template after restore = %q, want %q", restored.Template, originalDefault)
	}
}

func TestDefaultTemplate_Restore_DefaultTemplateColumnUnchanged(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	before, err := svc.Get("test-writer")
	if err != nil {
		t.Fatalf("Get before: %v", err)
	}
	if before.DefaultTemplate == nil {
		t.Fatal("DefaultTemplate is nil, want non-nil for seeded readonly")
	}
	originalDefault := *before.DefaultTemplate

	newText := "temp edit"
	if err := svc.Update("test-writer", &types.DefaultTemplateUpdateRequest{Template: &newText}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := svc.Restore("test-writer"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	after, err := svc.Get("test-writer")
	if err != nil {
		t.Fatalf("Get after restore: %v", err)
	}
	// default_template column must not be changed by restore
	if after.DefaultTemplate == nil || *after.DefaultTemplate != originalDefault {
		t.Errorf("DefaultTemplate after restore = %v, want %q (must not change)", after.DefaultTemplate, originalDefault)
	}
	if after.Template != originalDefault {
		t.Errorf("Template after restore = %q, want %q", after.Template, originalDefault)
	}
}

func TestDefaultTemplate_Restore_NonReadonlyReturnsError(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "user-restore-tmpl", Name: "User Restore", Template: "some content",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := svc.Restore("user-restore-tmpl")
	if err == nil {
		t.Fatal("expected error for restore on non-readonly template, got nil")
	}
	if !strings.Contains(err.Error(), "non-readonly") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "non-readonly")
	}
}

func TestDefaultTemplate_Restore_NotFound(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	err := svc.Restore("no-such-template")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// --- Delete ---

func TestDefaultTemplate_Delete_ReadonlyRejected(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	err := svc.Delete("qa-verifier")
	if err == nil {
		t.Fatal("expected error for delete on readonly template, got nil")
	}
	if !strings.Contains(err.Error(), "readonly") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "readonly")
	}
	// Template must still exist
	if _, err := svc.Get("qa-verifier"); err != nil {
		t.Errorf("Get after failed delete: %v", err)
	}
}

func TestDefaultTemplate_Delete_UserCreatedAllowed(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "deletable-tmpl", Name: "Deletable", Template: "content",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete("deletable-tmpl"); err != nil {
		t.Fatalf("Delete user-created: %v", err)
	}
	if _, err := svc.Get("deletable-tmpl"); err == nil {
		t.Error("Get after delete: expected not-found error, got nil")
	}
}
