package service

import (
	"testing"

	"be/internal/types"
)

// --- List with type filter ---

func TestDefaultTemplate_List_FilterByTypeAgent(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	templates, err := svc.List("agent")
	if err != nil {
		t.Fatalf("List(agent): %v", err)
	}
	if len(templates) != 6 {
		t.Fatalf("List(agent) len = %d, want 6", len(templates))
	}
	for _, tmpl := range templates {
		if tmpl.Type != "agent" {
			t.Errorf("template %q: Type = %q, want %q", tmpl.ID, tmpl.Type, "agent")
		}
	}
}

func TestDefaultTemplate_List_FilterByTypeInjectable(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	templates, err := svc.List("injectable")
	if err != nil {
		t.Fatalf("List(injectable): %v", err)
	}
	if len(templates) != 4 {
		t.Fatalf("List(injectable) len = %d, want 4", len(templates))
	}
	wantIDs := map[string]bool{
		"continuation":      true,
		"low-context":       true,
		"callback":          true,
		"user-instructions": true,
	}
	for _, tmpl := range templates {
		if tmpl.Type != "injectable" {
			t.Errorf("template %q: Type = %q, want %q", tmpl.ID, tmpl.Type, "injectable")
		}
		if !wantIDs[tmpl.ID] {
			t.Errorf("unexpected injectable template ID: %q", tmpl.ID)
		}
	}
}

func TestDefaultTemplate_List_FilterByTypeUnknownReturnsEmpty(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	templates, err := svc.List("nonexistent")
	if err != nil {
		t.Fatalf("List(nonexistent): %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("List(nonexistent) len = %d, want 0", len(templates))
	}
}

func TestDefaultTemplate_List_NoFilterReturnsAll(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	templates, err := svc.List("")
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(templates) != 10 {
		t.Fatalf("List() len = %d, want 10", len(templates))
	}
	agentCount, injectableCount := 0, 0
	for _, tmpl := range templates {
		switch tmpl.Type {
		case "agent":
			agentCount++
		case "injectable":
			injectableCount++
		default:
			t.Errorf("template %q: unexpected Type = %q", tmpl.ID, tmpl.Type)
		}
	}
	if agentCount != 6 {
		t.Errorf("agent count = %d, want 6", agentCount)
	}
	if injectableCount != 4 {
		t.Errorf("injectable count = %d, want 4", injectableCount)
	}
}

// --- Create type handling ---

func TestDefaultTemplate_Create_DefaultsTypeToAgent(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	tmpl, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "no-type-tmpl", Name: "No Type", Template: "content",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tmpl.Type != "agent" {
		t.Errorf("Type = %q, want %q (default)", tmpl.Type, "agent")
	}

	got, err := svc.Get("no-type-tmpl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Type != "agent" {
		t.Errorf("persisted Type = %q, want %q", got.Type, "agent")
	}
}

func TestDefaultTemplate_Create_InjectableType(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	tmpl, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "custom-injectable", Name: "Custom Inj", Template: "inj content", Type: "injectable",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tmpl.Type != "injectable" {
		t.Errorf("Type = %q, want %q", tmpl.Type, "injectable")
	}

	got, err := svc.Get("custom-injectable")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Type != "injectable" {
		t.Errorf("persisted Type = %q, want %q", got.Type, "injectable")
	}
}

func TestDefaultTemplate_Create_CustomType(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	tmpl, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "macro-tmpl", Name: "Macro", Template: "macro body", Type: "macro",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tmpl.Type != "macro" {
		t.Errorf("Type = %q, want %q", tmpl.Type, "macro")
	}
}

// --- Update type handling ---

func TestDefaultTemplate_Update_ReadonlyIgnoresTypeChange(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	newType := "custom"
	err := svc.Update("implementor", &types.DefaultTemplateUpdateRequest{Type: &newType})
	if err != nil {
		t.Fatalf("Update readonly with type change returned error: %v (should silently ignore)", err)
	}

	tmpl, err := svc.Get("implementor")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tmpl.Type != "agent" {
		t.Errorf("Type = %q, want %q (should be unchanged)", tmpl.Type, "agent")
	}
}

func TestDefaultTemplate_Update_ReadonlyInjectableIgnoresTypeChange(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	newType := "agent"
	err := svc.Update("callback", &types.DefaultTemplateUpdateRequest{Type: &newType})
	if err != nil {
		t.Fatalf("Update readonly injectable with type change returned error: %v", err)
	}

	tmpl, err := svc.Get("callback")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tmpl.Type != "injectable" {
		t.Errorf("Type = %q, want %q (should be unchanged)", tmpl.Type, "injectable")
	}
}

func TestDefaultTemplate_Update_NonReadonlyAllowsTypeChange(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.DefaultTemplateCreateRequest{
		ID: "changeable", Name: "Changeable", Template: "content",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newType := "injectable"
	if err := svc.Update("changeable", &types.DefaultTemplateUpdateRequest{Type: &newType}); err != nil {
		t.Fatalf("Update type: %v", err)
	}

	tmpl, err := svc.Get("changeable")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tmpl.Type != "injectable" {
		t.Errorf("Type = %q, want %q", tmpl.Type, "injectable")
	}
}

// --- Seeded injectables verification ---

func TestDefaultTemplate_SeededInjectables(t *testing.T) {
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	expected := []struct {
		id       string
		name     string
		readonly bool
	}{
		{"continuation", "Continuation (stall/fail restart)", true},
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
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	original, err := svc.Get("continuation")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	originalText := original.Template

	newText := "modified continuation text"
	if err := svc.Update("continuation", &types.DefaultTemplateUpdateRequest{Template: &newText}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := svc.Get("continuation")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if after.Template != "modified continuation text" {
		t.Fatalf("Template after update = %q, want %q", after.Template, "modified continuation text")
	}

	if err := svc.Restore("continuation"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restored, err := svc.Get("continuation")
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
	if len(injectables) != 5 {
		t.Errorf("List(injectable) len = %d, want 5 (4 seeded + 1 created)", len(injectables))
	}

	all, err := svc.List("")
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(all) != 12 {
		t.Errorf("List() len = %d, want 12 (10 seeded + 2 created)", len(all))
	}
}
