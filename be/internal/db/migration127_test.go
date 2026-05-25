package db

import (
	"fmt"
	"testing"
)

// TestMigration127_NoGeminiOpenCodeRows verifies that after all migrations
// no cli_models rows with cli_type gemini or opencode remain.
func TestMigration127_NoGeminiOpenCodeRows(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM cli_models WHERE cli_type IN ('gemini', 'opencode')`,
	).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT: %v", err)
	}
	if count != 0 {
		t.Errorf("cli_models with cli_type gemini/opencode = %d, want 0", count)
	}
}

// TestMigration127_CheckConstraintRejectsRemovedTypes verifies the rebuilt
// cli_models table rejects gemini and opencode insertions.
func TestMigration127_CheckConstraintRejectsRemovedTypes(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cases := []struct {
		cliType string
		wantOK  bool
	}{
		{"claude", true},
		{"codex", true},
		{"gemini", false},
		{"opencode", false},
		{"other", false},
	}
	for i, tc := range cases {
		t.Run(tc.cliType, func(t *testing.T) {
			id := fmt.Sprintf("test-127-%d", i)
			_, err := pool.Exec(
				`INSERT INTO cli_models (id, cli_type, display_name, mapped_model, created_at, updated_at)
				 VALUES (?, ?, 'Test', 'test-model', datetime('now'), datetime('now'))`,
				id, tc.cliType)
			if tc.wantOK && err != nil {
				t.Errorf("cli_type=%q: unexpected error: %v", tc.cliType, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("cli_type=%q: expected CHECK constraint error, got nil", tc.cliType)
			}
		})
	}
}

// TestMigration127_AllColumnsPresent verifies the full column set is intact
// after the table rebuild.
func TestMigration127_AllColumnsPresent(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "cli_models")
	required := []string{
		"id", "cli_type", "display_name", "mapped_model",
		"reasoning_effort", "context_length", "read_only",
		"created_at", "updated_at", "enabled",
	}
	for _, name := range required {
		if _, ok := cols[name]; !ok {
			t.Errorf("cli_models missing column %q after migration 127", name)
		}
	}
}
