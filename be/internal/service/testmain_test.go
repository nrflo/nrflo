package service

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// svcTemplateDBPath is a pre-migrated SQLite file shared across all service package tests.
// Tests copy it instead of running migrations per-test.
var svcTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-svc-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	svcTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(svcTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// svcCopyTemplateDB copies the template DB to dst.
// Much faster than running migrations from scratch.
func svcCopyTemplateDB(dst string) error {
	src, err := os.Open(svcTemplateDBPath)
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
