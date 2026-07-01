package db

import (
	"testing"
)

// TestMigration153_SonnetFive verifies the seeded "sonnet" cli_models and
// api_models rows are repointed to Claude Sonnet 5 with a 1M context window
// in both tables.
func TestMigration153_SonnetFive(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var cliMapped string
	var cliContext int
	if err := pool.QueryRow(
		`SELECT mapped_model, context_length FROM cli_models WHERE id = 'sonnet' AND cli_type = 'claude'`,
	).Scan(&cliMapped, &cliContext); err != nil {
		t.Fatalf("query cli_models: %v", err)
	}
	if cliMapped != "claude-sonnet-5" || cliContext != 1000000 {
		t.Errorf("cli_models[sonnet] = (%q, %d), want (%q, %d)", cliMapped, cliContext, "claude-sonnet-5", 1000000)
	}

	var apiMapped string
	var apiContext int
	if err := pool.QueryRow(
		`SELECT mapped_model, context_length FROM api_models WHERE id = 'sonnet' AND provider = 'anthropic'`,
	).Scan(&apiMapped, &apiContext); err != nil {
		t.Fatalf("query api_models: %v", err)
	}
	if apiMapped != "claude-sonnet-5" || apiContext != 1000000 {
		t.Errorf("api_models[sonnet] = (%q, %d), want (%q, %d)", apiMapped, apiContext, "claude-sonnet-5", 1000000)
	}
}
