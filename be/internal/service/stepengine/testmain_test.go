package stepengine

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// stepengineTemplateDBPath holds a pre-migrated DB created once by TestMain.
// Every DB-backed test copies this file instead of migrating per test
// (be/CLAUDE.md: DB tests never migrate per-test).
var stepengineTemplateDBPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "stepengine-template-*")
	if err != nil {
		panic("create template dir: " + err.Error())
	}
	stepengineTemplateDBPath = filepath.Join(dir, "template.db")
	d, err := db.OpenPath(stepengineTemplateDBPath)
	if err != nil {
		panic("create template DB: " + err.Error())
	}
	d.Close()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// newTestPool copies the pre-migrated template DB and opens a pool without
// running migrations.
func newTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "test.db")
	if err := copyStepengineTemplateDB(stepengineTemplateDBPath, dest); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.NewPoolPathExisting(dest, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func copyStepengineTemplateDB(src, dst string) error {
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
