package repo

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// repoTemplateDBPath holds the path to a pre-migrated DB created once by TestMain.
// All repo tests copy this file instead of running 74 migrations per test.
var repoTemplateDBPath string

// TestMain creates a single pre-migrated template DB before running any tests.
// Copying the template is ~45x faster than re-running all migrations per test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "repo-template-*")
	if err != nil {
		panic("create template dir: " + err.Error())
	}
	repoTemplateDBPath = filepath.Join(dir, "template.db")
	d, err := db.OpenPath(repoTemplateDBPath)
	if err != nil {
		panic("create template DB: " + err.Error())
	}
	d.Close()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// newTestDB copies the pre-migrated template and opens it without running migrations.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "test.db")
	if err := copyDBFile(repoTemplateDBPath, dest); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	d, err := db.OpenPathExisting(dest)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestPool copies the pre-migrated template and opens a pool without running migrations.
func newTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "test.db")
	if err := copyDBFile(repoTemplateDBPath, dest); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.NewPoolPathExisting(dest, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// setupTestDB creates a test database with required FK dependencies for agent session tests.
func setupTestDB(t *testing.T) (*db.DB, *AgentSessionRepo, string) {
	t.Helper()
	database := newTestDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj', 'Test Project', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj', 'test-workflow', 'Test Workflow', 'ticket', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	wfiID := "wfi-test-123"
	if _, err := database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, 'proj', '', 'test-workflow', 'active', 'ticket', '{}', datetime('now'), datetime('now'))`, wfiID); err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}
	r := NewAgentSessionRepo(database, clock.Real())
	return database, r, wfiID
}

func copyDBFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
