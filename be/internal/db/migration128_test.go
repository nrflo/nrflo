package db

import (
	"fmt"
	"testing"
)

// TestMigration128_AllColumnsPresent verifies the api_models table has all expected columns.
func TestMigration128_AllColumnsPresent(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "api_models")
	required := []string{
		"id", "provider", "display_name", "mapped_model",
		"reasoning_effort", "context_length", "read_only",
		"enabled", "created_at", "updated_at",
	}
	for _, name := range required {
		if _, ok := cols[name]; !ok {
			t.Errorf("api_models missing column %q after migration 128", name)
		}
	}
}

// TestMigration128_CheckRejectsInvalidProvider verifies the CHECK constraint
// on api_models.provider rejects values outside {anthropic, openai}.
func TestMigration128_CheckRejectsInvalidProvider(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cases := []struct {
		provider string
		wantOK   bool
	}{
		{"anthropic", true},
		{"openai", true},
		{"azure", false},
		{"gemini", false},
		{"other", false},
	}

	for i, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			id := fmt.Sprintf("test-128-%d", i)
			_, err := pool.Exec(
				`INSERT INTO api_models (id, provider, display_name, mapped_model, created_at, updated_at)
				 VALUES (?, ?, 'Test', 'test-model', datetime('now'), datetime('now'))`,
				id, tc.provider)
			if tc.wantOK && err != nil {
				t.Errorf("provider=%q: unexpected error: %v", tc.provider, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("provider=%q: expected CHECK constraint error, got nil", tc.provider)
			}
		})
	}
}

// TestMigration128_SeededAnthropicRowCount verifies exactly 6 anthropic read-only rows.
func TestMigration128_SeededAnthropicRowCount(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM api_models WHERE provider = 'anthropic' AND read_only = 1`,
	).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT anthropic: %v", err)
	}
	if count != 6 {
		t.Errorf("seeded anthropic rows = %d, want 6", count)
	}
}

// TestMigration128_SeededOpenAIRowCount verifies exactly 6 openai read-only rows.
func TestMigration128_SeededOpenAIRowCount(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM api_models WHERE provider = 'openai' AND read_only = 1`,
	).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT openai: %v", err)
	}
	if count != 6 {
		t.Errorf("seeded openai rows = %d, want 6", count)
	}
}

// TestMigration128_SeededRowsAllEnabled verifies all seeded rows start enabled.
func TestMigration128_SeededRowsAllEnabled(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM api_models WHERE read_only = 1 AND enabled != 1`,
	).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT disabled seeded: %v", err)
	}
	if count != 0 {
		t.Errorf("seeded rows with enabled != 1 = %d, want 0", count)
	}
}

// TestMigration128_NoOverrideSystemPromptColumn verifies api_models does NOT
// have an override_system_prompt column (unlike cli_models).
func TestMigration128_NoOverrideSystemPromptColumn(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "api_models")
	if _, ok := cols["override_system_prompt"]; ok {
		t.Error("api_models has override_system_prompt column, it should not")
	}
}

// TestMigration128_AnthropicOpus47Present verifies the opus_4_7 seeded row is present.
func TestMigration128_AnthropicOpus47Present(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var provider, mappedModel string
	var readOnly int
	err = pool.QueryRow(
		`SELECT provider, mapped_model, read_only FROM api_models WHERE id = 'opus_4_7'`,
	).Scan(&provider, &mappedModel, &readOnly)
	if err != nil {
		t.Fatalf("SELECT opus_4_7: %v", err)
	}
	if provider != "anthropic" {
		t.Errorf("opus_4_7 provider = %q, want anthropic", provider)
	}
	if mappedModel != "claude-opus-4-7" {
		t.Errorf("opus_4_7 mapped_model = %q, want claude-opus-4-7", mappedModel)
	}
	if readOnly != 1 {
		t.Errorf("opus_4_7 read_only = %d, want 1", readOnly)
	}
}
