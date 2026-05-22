package socket

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// templateDBPath is a pre-migrated SQLite file shared across all tests in this
// package. newHandlerTestEnv copies it instead of running all migrations per
// test (the per-test migrate dominated this package's wall time under -p).
// See be/CLAUDE.md "DB tests never migrate per-test".
var templateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-socket-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}

	templateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(templateDBPath, db.DefaultPoolConfig()) // migrate once
	if err != nil {
		panic("failed to build template DB: " + err.Error())
	}
	pool.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// copyTemplateDB copies the pre-migrated template DB to dst. Fatals on error.
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
