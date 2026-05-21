package tools_builtin

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// builtinTemplateDBPath is a pre-migrated SQLite file shared across all tests.
// Tests copy it to a fresh temp path instead of running migrations per-test.
var builtinTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-builtin-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	builtinTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(builtinTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	os.Exit(m.Run())
}

// copyBuiltinTemplateDB copies the pre-migrated template DB to dst.
func copyBuiltinTemplateDB(dst string) error {
	src, err := os.Open(builtinTemplateDBPath)
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}
