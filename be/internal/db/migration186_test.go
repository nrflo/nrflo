package db

import (
	"strings"
	"testing"
)

// TestMigration186_OpenRouterRowInsertsPostMigration verifies the widened
// CHECK constraint accepts provider='openrouter'.
func TestMigration186_OpenRouterRowInsertsPostMigration(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Exec(`INSERT INTO models
		(id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
		 cli_context, api_context, fallback_models, default_effort, read_only, enabled,
		 created_at, updated_at)
		VALUES ('or-test-model', 'openrouter', 'OR Test', '', 'openai/gpt-4o', '[]', '[]',
		 200000, 200000, '', '', 0, 1, datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert openrouter row: %v", err)
	}

	var provider string
	if err := pool.QueryRow(`SELECT provider FROM models WHERE id = 'or-test-model'`).Scan(&provider); err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if provider != "openrouter" {
		t.Errorf("provider = %q, want openrouter", provider)
	}
}

// TestMigration186_BogusProviderStillRejected verifies the CHECK constraint
// still rejects a provider outside the widened set — the rebuild must not
// have accidentally dropped the constraint entirely.
func TestMigration186_BogusProviderStillRejected(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Exec(`INSERT INTO models
		(id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
		 cli_context, api_context, fallback_models, default_effort, read_only, enabled,
		 created_at, updated_at)
		VALUES ('bogus-provider-model', 'bogus', 'Bogus', 'x', '', '[]', '[]',
		 200000, 200000, '', '', 0, 1, datetime('now'), datetime('now'))`)
	if err == nil {
		t.Fatal("insert with bogus provider succeeded, want CHECK constraint violation")
	}
	if !strings.Contains(err.Error(), "CHECK") && !strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Errorf("error = %v, want a CHECK-constraint failure", err)
	}
}

// TestMigration186_PreservedReadOnlyRowsAndPricingSurvive verifies the
// table-rebuild carried existing seeded rows through intact — read_only flag
// and pricing columns (migration 000183) survive the copy.
func TestMigration186_PreservedReadOnlyRowsAndPricingSurvive(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var readOnly int
	var priceIn *float64
	if err := pool.QueryRow(`SELECT read_only, price_in FROM models WHERE id = 'sonnet-5'`).Scan(&readOnly, &priceIn); err != nil {
		t.Fatalf("query sonnet-5: %v", err)
	}
	if readOnly != 1 {
		t.Errorf("sonnet-5 read_only = %d, want 1 (seeded built-in model)", readOnly)
	}
	if priceIn == nil || *priceIn != 3 {
		t.Errorf("sonnet-5 price_in = %v, want 3 (seeded by migration 000183)", priceIn)
	}

	var total int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM models`).Scan(&total); err != nil {
		t.Fatalf("count models: %v", err)
	}
	if total == 0 {
		t.Error("models table is empty after rebuild, want seeded rows to survive")
	}
}

// TestMigration186_NoSeededOpenRouterRows verifies the migration seeds no
// openrouter catalog rows — openrouter rows are strictly user-created.
func TestMigration186_NoSeededOpenRouterRows(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM models WHERE provider = 'openrouter'`).Scan(&count); err != nil {
		t.Fatalf("count openrouter rows: %v", err)
	}
	if count != 0 {
		t.Errorf("openrouter row count = %d, want 0 (no seeded catalog)", count)
	}
}

// TestMigration186_CLIModelOrAPIModelCheckStillEnforced verifies the
// pre-existing "cli_model or api_model required" CHECK survived the rebuild
// alongside the widened provider CHECK.
func TestMigration186_CLIModelOrAPIModelCheckStillEnforced(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Exec(`INSERT INTO models
		(id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
		 cli_context, api_context, fallback_models, default_effort, read_only, enabled,
		 created_at, updated_at)
		VALUES ('no-mode-model', 'openrouter', 'No Mode', '', '', '[]', '[]',
		 200000, 200000, '', '', 0, 1, datetime('now'), datetime('now'))`)
	if err == nil {
		t.Fatal("insert with empty cli_model and api_model succeeded, want CHECK constraint violation")
	}
}
