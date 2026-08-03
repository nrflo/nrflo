package service

import (
	"testing"

	"be/internal/types"
)

// --- List with type filter ---

func TestDefaultTemplate_List_FilterByTypeAgent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	templates, err := svc.List("injectable")
	if err != nil {
		t.Fatalf("List(injectable): %v", err)
	}
	if len(templates) != 19 {
		t.Fatalf("List(injectable) len = %d, want 19", len(templates))
	}
	wantIDs := map[string]bool{
		"low-context": true, "callback": true, "user-instructions": true,
		"system-prompt-suffix": true, "finish-reminder": true, "system-prompt": true, "working-set": true,
		"api-system-prompt": true,
		"tier-t0-decider":   true, "tier-t1-executor": true, "tier-t2-extractor": true,
		"delegation-guidance": true, "tier-t0-bare": true, "crash-resume": true, "stepwise-guidance": true,
		"validation-failure": true, "timeout-restart": true,
		"workspace-live-tree": true, "workspace-worktree": true}
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
	t.Parallel()
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
	t.Parallel()
	svc, cleanup := setupDefaultTemplateTestEnv(t)
	defer cleanup()

	templates, err := svc.List("")
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(templates) != 25 {
		t.Fatalf("List() len = %d, want 25", len(templates))
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
	if injectableCount != 19 {
		t.Errorf("injectable count = %d, want 19", injectableCount)
	}
}

// --- Create type handling ---

func TestDefaultTemplate_Create_DefaultsTypeToAgent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// Seeded-injectable verification, restore, and mixed-type list filtering
// after creates live in default_template_type_seeded_test.go (300-line split).
