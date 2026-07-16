package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// TestProjectList_ExcludesGlobalNamespace verifies the reserved GlobalProjectID
// storage row never surfaces in the project listing, even though it exists as an
// FK parent for global workflow definitions.
func TestProjectList_ExcludesGlobalNamespace(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "projlist.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.Real()

	// Seeds the hidden __global__ namespace row + the dynamic definition.
	if err := EnsureGlobalDynamicWorkflow(pool, clk, t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDynamicWorkflow: %v", err)
	}
	now := "2026-01-01T00:00:00Z"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('real','Real',NULL,?,?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	projects, err := NewProjectService(pool, clk).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range projects {
		if p.ID == GlobalProjectID {
			t.Fatalf("project listing leaked reserved namespace %q", GlobalProjectID)
		}
	}
	if len(projects) != 1 || projects[0].ID != "real" {
		t.Fatalf("List returned %d projects, want exactly [real]", len(projects))
	}
}
