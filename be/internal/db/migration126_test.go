package db

import (
	"fmt"
	"strings"
	"testing"
)

// TestMigration126_OverrideSystemPromptColumnSchema verifies migration 000126 adds the
// override_system_prompt INTEGER NOT NULL DEFAULT 0 column to cli_models.
func TestMigration126_OverrideSystemPromptColumnSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "cli_models")
	col, ok := cols["override_system_prompt"]
	if !ok {
		t.Fatal("override_system_prompt column missing from cli_models; migration 000126 may not have run")
	}
	if col.colType != "INTEGER" {
		t.Errorf("override_system_prompt colType = %q, want INTEGER", col.colType)
	}
	if col.notNull != 1 {
		t.Errorf("override_system_prompt notNull = %d, want 1", col.notNull)
	}
	dflt := fmt.Sprintf("%v", col.dflt)
	if dflt != "0" {
		t.Errorf("override_system_prompt default = %q, want \"0\"", dflt)
	}
}

// TestMigration126_ExistingRowsDefaultToZero verifies that seeded cli_models rows
// all have override_system_prompt = 0 after the migration.
func TestMigration126_ExistingRowsDefaultToZero(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM cli_models WHERE override_system_prompt != 0`,
	).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT: %v", err)
	}
	if count != 0 {
		t.Errorf("found %d cli_models rows with override_system_prompt != 0, want 0", count)
	}
}

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
