package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupDefaultTemplateTestEnv creates an isolated DB copy for default template tests.
func setupDefaultTemplateTestEnv(t *testing.T) (*DefaultTemplateService, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "default_template_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	svc := NewDefaultTemplateService(pool, clock.Real())
	return svc, func() { pool.Close() }
}

// --- List ---

func TestDefaultTemplate_List_ReadonlyHasDefaultTemplate(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	templates, err := svc.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(templates) != 10 {
		t.Fatalf("List len = %d, want 10 pre-seeded readonly templates", len(templates))
	}
	for _, tmpl := range templates {
		if !tmpl.Readonly {
			t.Errorf("template %q: Readonly = false, want true", tmpl.ID)
		}
		if tmpl.DefaultTemplate == nil {
			t.Errorf("template %q: DefaultTemplate is nil, want non-nil for readonly", tmpl.ID)
		}
		if tmpl.DefaultTemplate != nil && *tmpl.DefaultTemplate != tmpl.Template {
			t.Errorf("template %q: DefaultTemplate != Template before any edit", tmpl.ID)
		}
	}
}

func TestDefaultTemplate_List_UserCreatedNoDefaultTemplate(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "user-tmpl", Name: "User Template", Template: "user content",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	templates, err := svc.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, tmpl := range templates {
		if tmpl.ID == "user-tmpl" {
			if tmpl.DefaultTemplate != nil {
				t.Errorf("user-created DefaultTemplate = %q, want nil", *tmpl.DefaultTemplate)
			}
			return
		}
	}
	t.Error("created template not found in list")
}

// --- Get ---

func TestDefaultTemplate_Get_ReadonlyHasDefaultTemplate(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	tmpl, err := svc.Get("implementor")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !tmpl.Readonly {
		t.Error("Readonly = false, want true")
	}
	if tmpl.DefaultTemplate == nil {
		t.Fatal("DefaultTemplate = nil, want non-nil for readonly template")
	}
	if *tmpl.DefaultTemplate != tmpl.Template {
		t.Errorf("DefaultTemplate %q != Template %q before any edit", *tmpl.DefaultTemplate, tmpl.Template)
	}
}

func TestDefaultTemplate_Get_UserCreatedNoDefaultTemplate(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "no-def-tmpl", Name: "No Default", Template: "content",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	tmpl, err := svc.Get("no-def-tmpl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tmpl.DefaultTemplate != nil {
		t.Errorf("DefaultTemplate = %q, want nil for user-created", *tmpl.DefaultTemplate)
	}
}

func TestDefaultTemplate_Get_NotFound(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	_, err := svc.Get("no-such-template")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// --- Update ---

func TestDefaultTemplate_Update_ReadonlyTemplateTextAllowed(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	newText := "custom implementor prompt"
	if err := svc.Update("implementor", &types.DefaultTemplateUpdateRequest{Template: &newText}); err != nil {
		t.Fatalf("Update readonly template text: %v", err)
	}

	tmpl, err := svc.Get("implementor")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if tmpl.Template != "custom implementor prompt" {
		t.Errorf("Template = %q, want %q", tmpl.Template, "custom implementor prompt")
	}
	// DefaultTemplate (original column) must not be affected
	if tmpl.DefaultTemplate != nil && *tmpl.DefaultTemplate == "custom implementor prompt" {
		t.Error("DefaultTemplate was modified — should remain the original seeded value")
	}
}

func TestDefaultTemplate_Update_ReadonlyNameRejected(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	newName := "Hacked Name"
	err := svc.Update("implementor", &types.DefaultTemplateUpdateRequest{Name: &newName})
	if err == nil {
		t.Fatal("expected error for name update on readonly template, got nil")
	}
	if !strings.Contains(err.Error(), "cannot modify name") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "cannot modify name")
	}
	// Name must be unchanged
	tmpl, err := svc.Get("implementor")
	if err != nil {
		t.Fatalf("Get after failed update: %v", err)
	}
	if tmpl.Name == "Hacked Name" {
		t.Error("Name was modified despite rejection")
	}
}

func TestDefaultTemplate_Update_ReadonlyNameAndTemplateBothRejected(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	newName := "Hacked"
	newText := "new text"
	err := svc.Update("implementor", &types.DefaultTemplateUpdateRequest{Name: &newName, Template: &newText})
	if err == nil {
		t.Fatal("expected error when name provided for readonly, got nil")
	}
	if !strings.Contains(err.Error(), "cannot modify name") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "cannot modify name")
	}
}

func TestDefaultTemplate_Update_NonReadonlyAllFields(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "editable-tmpl", Name: "Original", Template: "original content",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newName, newText := "Updated Name", "updated content"
	if err := svc.Update("editable-tmpl", &types.DefaultTemplateUpdateRequest{
		Name:     &newName,
		Template: &newText,
	}); err != nil {
		t.Fatalf("Update non-readonly: %v", err)
	}

	tmpl, err := svc.Get("editable-tmpl")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if tmpl.Name != "Updated Name" {
		t.Errorf("Name = %q, want %q", tmpl.Name, "Updated Name")
	}
	if tmpl.Template != "updated content" {
		t.Errorf("Template = %q, want %q", tmpl.Template, "updated content")
	}
}

func TestDefaultTemplate_Update_NotFound(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	newText := "whatever"
	err := svc.Update("no-such", &types.DefaultTemplateUpdateRequest{Template: &newText})
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}
