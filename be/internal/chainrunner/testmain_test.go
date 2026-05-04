package chainrunner

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// chainTemplateDBPath is a pre-migrated SQLite file shared across all chainrunner tests.
var chainTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-chain-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	chainTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(chainTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	os.Exit(m.Run())
}

// chainCopyTemplateDB copies the pre-migrated template DB to dst.
func chainCopyTemplateDB(dst string) error {
	src, err := os.Open(chainTemplateDBPath)
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
