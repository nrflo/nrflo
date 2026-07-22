package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// TestMigration197_GPT56ContextFix verifies the isolated pre->post 000197
// transition: cli_context corrected 372000->272000 for sol/terra/luna, with
// api_context (already 1050000 via 000170) and api_model left untouched.
func TestMigration197_GPT56ContextFix(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gpt56-context-fix.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(196); err != nil {
		t.Fatalf("migrate to 196: %v", err)
	}

	for _, id := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		var cliContext int
		if err := sqlDB.QueryRow(`SELECT cli_context FROM models WHERE id = ?`, id).Scan(&cliContext); err != nil {
			t.Fatalf("query pre-197 cli_context for %s: %v", id, err)
		}
		if cliContext != 372000 {
			t.Fatalf("pre-197 models[%q].cli_context = %d, want 372000 (fixture drifted)", id, cliContext)
		}
	}

	if err := m.Migrate(197); err != nil {
		t.Fatalf("migrate to 197: %v", err)
	}

	tests := []struct {
		id             string
		wantCLIContext int
		wantAPIContext int
		wantAPIModel   string
	}{
		{"gpt-5.6-sol", 272000, 1050000, "gpt-5.6-sol"},
		{"gpt-5.6-terra", 272000, 1050000, "gpt-5.6-terra"},
		{"gpt-5.6-luna", 272000, 1050000, "gpt-5.6-luna"},
	}
	for _, tt := range tests {
		var cliContext, apiContext int
		var apiModel string
		if err := sqlDB.QueryRow(`SELECT cli_context, api_context, api_model FROM models WHERE id = ?`, tt.id).
			Scan(&cliContext, &apiContext, &apiModel); err != nil {
			t.Fatalf("query post-197 model %s: %v", tt.id, err)
		}
		if cliContext != tt.wantCLIContext {
			t.Errorf("models[%q].cli_context = %d, want %d", tt.id, cliContext, tt.wantCLIContext)
		}
		if apiContext != tt.wantAPIContext {
			t.Errorf("models[%q].api_context = %d, want %d", tt.id, apiContext, tt.wantAPIContext)
		}
		if apiModel != tt.wantAPIModel {
			t.Errorf("models[%q].api_model = %q, want %q", tt.id, apiModel, tt.wantAPIModel)
		}
	}

	// Unrelated model must be unaffected by the migration.
	var otherCLI int
	if err := sqlDB.QueryRow(`SELECT cli_context FROM models WHERE id = 'gpt-5.4'`).Scan(&otherCLI); err != nil {
		t.Fatalf("query unaffected model: %v", err)
	}
	if otherCLI != 200000 {
		t.Errorf("models[\"gpt-5.4\"].cli_context = %d, want 200000 (unaffected)", otherCLI)
	}

	// Second application is a no-op (idempotent forward migration).
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("second migrate.Up should be no-op, got: %v", err)
	}
}
