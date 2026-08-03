package service

// Split from default_template_type_test.go to stay under the 300-line cap
// (Rule 5): seeded-injectable verification, restore, and mixed-type list
// filtering after creates.

import (
	"testing"

	"be/internal/types"
)

// --- Seeded injectables verification ---

func TestDefaultTemplate_SeededInjectables(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	expected := []struct {
		id       string
		name     string
		readonly bool
	}{
		{"low-context", "Low-context restart", true},
		{"callback", "Callback instructions", true},
		{"user-instructions", "User instructions", true},
	}
	for _, want := range expected {
		t.Run(want.id, func(t *testing.T) {
			tmpl, err := svc.Get(want.id)
			if err != nil {
				t.Fatalf("Get(%q): %v", want.id, err)
			}
			if tmpl.Name != want.name {
				t.Errorf("Name = %q, want %q", tmpl.Name, want.name)
			}
			if tmpl.Type != "injectable" {
				t.Errorf("Type = %q, want %q", tmpl.Type, "injectable")
			}
			if tmpl.Readonly != want.readonly {
				t.Errorf("Readonly = %v, want %v", tmpl.Readonly, want.readonly)
			}
			if tmpl.DefaultTemplate == nil {
				t.Fatal("DefaultTemplate = nil, want non-nil")
			}
			if *tmpl.DefaultTemplate != tmpl.Template {
				t.Errorf("DefaultTemplate != Template (seeded values should match)")
			}
		})
	}
}

// --- Restore injectable ---

func TestDefaultTemplate_Restore_InjectableReadonly(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	original, err := svc.Get("callback")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	originalText := original.Template

	newText := "modified callback text"
	if err := svc.Update("callback", &types.DefaultTemplateUpdateRequest{Template: &newText}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := svc.Get("callback")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if after.Template != "modified callback text" {
		t.Fatalf("Template after update = %q, want %q", after.Template, "modified callback text")
	}

	if err := svc.Restore("callback"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restored, err := svc.Get("callback")
	if err != nil {
		t.Fatalf("Get after restore: %v", err)
	}
	if restored.Template != originalText {
		t.Errorf("Template after restore = %q, want %q", restored.Template, originalText)
	}
	if restored.Type != "injectable" {
		t.Errorf("Type after restore = %q, want %q (should be unchanged)", restored.Type, "injectable")
	}
}

// --- List with filter after creating mixed types ---

func TestDefaultTemplate_List_FilterAfterCreatingMixed(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "extra-agent", Name: "Extra Agent", Template: "c", Type: "agent",
	}); err != nil {
		t.Fatalf("Create agent: %v", err)
	}
	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "extra-inj", Name: "Extra Inj", Template: "c", Type: "injectable",
	}); err != nil {
		t.Fatalf("Create injectable: %v", err)
	}

	agents, err := svc.List("agent")
	if err != nil {
		t.Fatalf("List(agent): %v", err)
	}
	if len(agents) != 7 {
		t.Errorf("List(agent) len = %d, want 7 (6 seeded + 1 created)", len(agents))
	}

	injectables, err := svc.List("injectable")
	if err != nil {
		t.Fatalf("List(injectable): %v", err)
	}
	if len(injectables) != 20 {
		t.Errorf("List(injectable) len = %d, want 20 (19 seeded + 1 created)", len(injectables))
	}

	all, err := svc.List("")
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(all) != 27 {
		t.Errorf("List() len = %d, want 27 (25 seeded + 2 created)", len(all))
	}
}
