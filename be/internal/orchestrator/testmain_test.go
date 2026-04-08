package orchestrator

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// orchTemplateDBPath is a pre-migrated SQLite file shared across all orchestrator package tests.
var orchTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-orch-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	orchTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(orchTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	os.Exit(m.Run())
}

// orchCopyTemplateDB copies the pre-migrated template DB to dst.
func orchCopyTemplateDB(dst string) error {
	src, err := os.Open(orchTemplateDBPath)
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
