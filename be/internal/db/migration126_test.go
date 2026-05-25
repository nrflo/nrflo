package db

import (
	"strings"
	"testing"
)

// TestMigration126_SystemPromptTemplateSeeded verifies the 'system-prompt' injectable
// default_template row is seeded with correct values.
func TestMigration126_SystemPromptTemplateSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var id, name, tmplType, template string
	var readonly int
	err = pool.QueryRow(
		`SELECT id, name, type, template, readonly FROM default_templates WHERE id = 'system-prompt'`,
	).Scan(&id, &name, &tmplType, &template, &readonly)
	if err != nil {
		t.Fatalf("query system-prompt template: %v", err)
	}

	if id != "system-prompt" {
		t.Errorf("id = %q, want 'system-prompt'", id)
	}
	if name != "System prompt (override)" {
		t.Errorf("name = %q, want 'System prompt (override)'", name)
	}
	if tmplType != "injectable" {
		t.Errorf("type = %q, want 'injectable'", tmplType)
	}
	if readonly != 1 {
		t.Errorf("readonly = %d, want 1", readonly)
	}
	if strings.TrimSpace(template) == "" {
		t.Error("template text is empty")
	}
}

// TestMigration126_SystemPromptTemplateHasDefaultTemplate verifies that the seeded
// system-prompt row has default_template populated (required for restore support).
func TestMigration126_SystemPromptTemplateHasDefaultTemplate(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var defaultTemplate string
	if err := pool.QueryRow(
		`SELECT COALESCE(default_template, '') FROM default_templates WHERE id = 'system-prompt'`,
	).Scan(&defaultTemplate); err != nil {
		t.Fatalf("query default_template: %v", err)
	}
	if defaultTemplate == "" {
		t.Error("system-prompt: default_template is empty; must be set for readonly templates")
	}
}
