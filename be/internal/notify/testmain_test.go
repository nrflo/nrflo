package notify

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

var notifyTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-notify-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	notifyTemplateDBPath = filepath.Join(tmpDir, "template.db")
	database, err := db.OpenPath(notifyTemplateDBPath)
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	database.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func copyNotifyTemplateDB(dst string) error {
	src, err := os.Open(notifyTemplateDBPath)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, src); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func openNotifyTestDB(t *testing.T, dst string) (*db.DB, error) {
	t.Helper()
	if err := copyNotifyTemplateDB(dst); err != nil {
		return nil, err
	}
	return db.OpenPathExisting(dst)
}

func openNotifyTestPool(t *testing.T, dst string) (*db.Pool, error) {
	t.Helper()
	if err := copyNotifyTemplateDB(dst); err != nil {
		return nil, err
	}
	return db.OpenPoolExisting(dst, db.DefaultPoolConfig())
}
