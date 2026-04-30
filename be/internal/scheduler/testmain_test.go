package scheduler

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// schedTemplateDBPath is a pre-migrated SQLite file shared across all scheduler package tests.
var schedTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-sched-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	schedTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(schedTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	os.Exit(m.Run())
}

// schedCopyTemplateDB copies the pre-migrated template DB to dst.
func schedCopyTemplateDB(dst string) error {
	src, err := os.Open(schedTemplateDBPath)
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
