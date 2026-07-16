package db

import (
	"testing"
)

// TestMigration153_SonnetFive verifies Sonnet 5 has a 1M context in both modes.
func TestMigration153_SonnetFive(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var cliMapped, apiMapped string
	var cliContext, apiContext int
	if err := pool.QueryRow(
		`SELECT cli_model, cli_context, api_model, api_context FROM models WHERE id = 'sonnet-5'`,
	).Scan(&cliMapped, &cliContext, &apiMapped, &apiContext); err != nil {
		t.Fatalf("query models: %v", err)
	}
	if cliMapped != "claude-sonnet-5" || cliContext != 1000000 {
		t.Errorf("models[sonnet-5] CLI = (%q, %d), want (%q, %d)", cliMapped, cliContext, "claude-sonnet-5", 1000000)
	}
	if apiMapped != "claude-sonnet-5" || apiContext != 1000000 {
		t.Errorf("models[sonnet-5] API = (%q, %d), want (%q, %d)", apiMapped, apiContext, "claude-sonnet-5", 1000000)
	}
}
