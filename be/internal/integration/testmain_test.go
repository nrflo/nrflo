package integration

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// templateDBPath is a pre-migrated SQLite file shared across all tests.
// Tests copy it to a fresh temp path instead of running migrations per-test.
var templateDBPath string

func TestMain(m *testing.M) {
	// Create template DB with all migrations applied once.
	tmpDir, err := os.MkdirTemp("", "nrf-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	templateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(templateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	os.Exit(m.Run())
}

// copyTemplateDB copies the template DB to dst and returns the path.
// Much faster than running migrations from scratch.
func copyTemplateDB(dst string) error {
	src, err := os.Open(templateDBPath)
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
