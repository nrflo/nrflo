package api

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// apiTemplateDBPath is a pre-migrated SQLite file shared across all api package tests.
var apiTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-api-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	apiTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(apiTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	os.Exit(m.Run())
}

// apiCopyTemplateDB copies the pre-migrated template DB to dst.
func apiCopyTemplateDB(dst string) error {
	src, err := os.Open(apiTemplateDBPath)
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
