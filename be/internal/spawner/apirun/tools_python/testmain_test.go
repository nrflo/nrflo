package tools_python

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

var pythonToolsTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-python-tools-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	pythonToolsTemplateDBPath = filepath.Join(tmpDir, "template.db")
	database, err := db.OpenPath(pythonToolsTemplateDBPath)
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	database.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func openPythonToolsTestDB(t *testing.T, dst string) (*db.DB, error) {
	t.Helper()
	src, err := os.Open(pythonToolsTemplateDBPath)
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
	return db.OpenPathExisting(dst)
}
