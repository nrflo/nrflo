package spawner

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// templateDBPath is a pre-migrated SQLite file shared across all tests in this
// package. Tests copy it to a fresh path and open it with db.OpenPathExisting /
// db.NewPoolPathExisting instead of running all migrations per test — the
// migration run dominated this package's wall time under -p parallelism.
var templateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-spawner-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}

	templateDBPath = filepath.Join(tmpDir, "template.db")
	database, err := db.Open(templateDBPath) // runs all migrations once
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	database.Close()

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// copyTemplateDB copies the pre-migrated template DB to dst, far cheaper than
// re-running migrations. Fatals on error.
func copyTemplateDB(t *testing.T, dst string) {
	t.Helper()
	src, err := os.Open(templateDBPath)
	if err != nil {
		t.Fatalf("open template db: %v", err)
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		t.Fatalf("copy template db: %v", err)
	}
}
