package repo

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// repoTemplateDBPath holds the path to a pre-migrated DB created once by TestMain.
// All repo tests copy this file instead of running 74 migrations per test.
var repoTemplateDBPath string

// TestMain creates a single pre-migrated template DB before running any tests.
// Copying the template is ~45x faster than re-running all migrations per test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "repo-template-*")
	if err != nil {
		panic("create template dir: " + err.Error())
	}
	repoTemplateDBPath = filepath.Join(dir, "template.db")
	d, err := db.OpenPath(repoTemplateDBPath)
	if err != nil {
		panic("create template DB: " + err.Error())
	}
	d.Close()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// newTestDB copies the pre-migrated template and opens it without running migrations.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "test.db")
	if err := copyDBFile(repoTemplateDBPath, dest); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	d, err := db.OpenPathExisting(dest)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestPool copies the pre-migrated template and opens a pool without running migrations.
func newTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "test.db")
	if err := copyDBFile(repoTemplateDBPath, dest); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.NewPoolPathExisting(dest, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func copyDBFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
