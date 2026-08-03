package apirun

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// apirunTemplateDBPath holds a pre-migrated template DB, copied per test
// that needs a real DispatchRepo (tool_audit_test.go) — mirrors
// repo/testhelper_test.go's pattern.
var apirunTemplateDBPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "apirun-template-*")
	if err != nil {
		panic("create template dir: " + err.Error())
	}
	apirunTemplateDBPath = filepath.Join(dir, "template.db")
	d, err := db.OpenPath(apirunTemplateDBPath)
	if err != nil {
		panic("create template DB: " + err.Error())
	}
	d.Close()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func openAPIRunTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "test.db")
	src, err := os.Open(apirunTemplateDBPath)
	if err != nil {
		t.Fatalf("open template: %v", err)
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		t.Fatalf("copy template: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close dest: %v", err)
	}
	pool, err := db.NewPoolPathExisting(dst, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}
