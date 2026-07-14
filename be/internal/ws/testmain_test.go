package ws

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

var wsTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-ws-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}

	wsTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(wsTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func openWSTestPool(t *testing.T, dst string) (*db.Pool, error) {
	t.Helper()
	src, err := os.Open(wsTemplateDBPath)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(out, src); err != nil {
		out.Close()
		return nil, err
	}
	if err = out.Close(); err != nil {
		return nil, err
	}
	return db.NewPoolPathExisting(dst, db.DefaultPoolConfig())
}
