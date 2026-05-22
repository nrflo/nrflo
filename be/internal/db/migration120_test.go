package db

import (
	"testing"
)

// TestMigration120_APICredentialsTableAbsent verifies that the api_credentials table
// is absent from sqlite_master after all migrations have been applied.
func TestMigration120_APICredentialsTableAbsent(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	row := pool.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='api_credentials'`)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan sqlite_master: %v", err)
	}
	if count != 0 {
		t.Errorf("api_credentials table found in sqlite_master; migration 000120 should have dropped it")
	}
}

// TestMigration120_APICredentialsIndexAbsent verifies that idx_api_credentials_provider_project
// is absent from sqlite_master after all migrations have been applied.
func TestMigration120_APICredentialsIndexAbsent(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	row := pool.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_api_credentials_provider_project'`)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan sqlite_master: %v", err)
	}
	if count != 0 {
		t.Errorf("idx_api_credentials_provider_project found in sqlite_master; migration 000120 should have dropped it")
	}
}
