package db

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// templateDBPath is a SQLite file with all migrations applied once, shared by
// every test in this package. Migration tests assert the *outcome* of the
// migration chain (schema, seed rows, constraints), not the migration process,
// so they copy this template and open it with NewPoolPathExisting instead of
// re-running all migrations per test. TestMain itself is the from-scratch
// "migrations apply cleanly" guarantee (it panics if they don't); see also
// TestMigrationsApplyFromScratch.
var templateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-db-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}

	templateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := NewPoolPath(templateDBPath, DefaultPoolConfig()) // runs all migrations once
	if err != nil {
		panic("failed to build template DB (migrations did not apply cleanly): " + err.Error())
	}
	pool.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// copyTemplateDB copies the pre-migrated template DB to dst (cheap file copy vs.
// re-running all migrations). Each caller gets its own copy, so write/constraint
// tests stay isolated. Fatals on error.
func copyTemplateDB(t *testing.T, dst string) {
	t.Helper()
	src, err := os.Open(templateDBPath)
	if err != nil {
		t.Fatalf("open template db: %v", err)
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		t.Fatalf("copy template db: %v", err)
	}
}

// newMigratedTestPool returns a pool over a fresh copy of the migrated template.
func newMigratedTestPool(t *testing.T) (*Pool, error) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	copyTemplateDB(t, dbPath)
	return NewPoolPathExisting(dbPath, DefaultPoolConfig())
}

// TestMigrationsApplyFromScratch is the one test that exercises the real
// migration chain end-to-end (the other migration tests use the shared
// template). It also re-opens the DB to confirm a second migrate run is a no-op.
func TestMigrationsApplyFromScratch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "scratch.db")

	pool, err := NewPoolPath(dbPath, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("from-scratch migrate failed: %v", err)
	}
	pool.Close()

	// Re-open: RunMigrations must be a clean no-op on an already-migrated DB.
	again, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("re-migrate (idempotency) failed: %v", err)
	}
	again.Close()
}
