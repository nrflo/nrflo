package limitsrefresh

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

var rfrTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-rfr-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	rfrTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(rfrTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	os.Exit(m.Run())
}

func rfrCopyTemplateDB(dst string) error {
	src, err := os.Open(rfrTemplateDBPath)
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
