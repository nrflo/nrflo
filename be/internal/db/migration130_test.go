package db

import "testing"

// Migration 000130 rebuilds cli_models without the per-model override toggle and
// seeds the replacement global setting claude_system_prompt_override_enabled=false
// (project_id='' sentinel). GetConfig returns "" → off on miss, so the seed is for
// visibility only; these tests pin the seeded default.

// TestMigration130_ClaudeSysPromptOverrideSeededFalse verifies the global config row
// is seeded with value 'false' under the global (empty) project_id.
func TestMigration130_ClaudeSysPromptOverrideSeededFalse(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var value string
	err = pool.QueryRow(
		`SELECT value FROM config WHERE project_id = '' AND key = 'claude_system_prompt_override_enabled'`,
	).Scan(&value)
	if err != nil {
		t.Fatalf("query seeded config row: %v", err)
	}
	if value != "false" {
		t.Errorf("claude_system_prompt_override_enabled = %q, want %q", value, "false")
	}
}

// TestMigration130_ClaudeSysPromptOverrideViaGetConfig verifies the seed is readable
// through the production GetConfig accessor and resolves off-by-default.
func TestMigration130_ClaudeSysPromptOverrideViaGetConfig(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	val, err := pool.GetConfig("claude_system_prompt_override_enabled")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "false" {
		t.Errorf("GetConfig = %q, want %q (toggle off by default)", val, "false")
	}
	if val == "true" {
		t.Error("seeded default must not be 'true' (override off by default)")
	}
}

// TestMigration130_ClaudeSysPromptOverrideSingleRow verifies exactly one global row
// exists for the key (the INSERT OR IGNORE seed does not duplicate).
func TestMigration130_ClaudeSysPromptOverrideSingleRow(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM config WHERE project_id = '' AND key = 'claude_system_prompt_override_enabled'`,
	).Scan(&count); err != nil {
		t.Fatalf("count seeded rows: %v", err)
	}
	if count != 1 {
		t.Errorf("seeded global rows = %d, want 1", count)
	}
}
