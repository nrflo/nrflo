package console

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// consoleTemplateDBPath is a pre-migrated SQLite file shared across all
// console package tests. Tests copy it to a fresh temp path instead of
// running migrations per-test (be/CLAUDE.md § DB tests never migrate per-test).
var consoleTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-console-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	consoleTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(consoleTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// copyConsoleTemplateDB copies the pre-migrated template DB to dst.
func copyConsoleTemplateDB(dst string) error {
	src, err := os.Open(consoleTemplateDBPath)
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
