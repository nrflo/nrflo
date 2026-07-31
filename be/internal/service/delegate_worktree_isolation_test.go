package service

import (
	"path/filepath"
	"testing"

	"be/internal/db"
)

// setupDelegateIsolationTestPool gives each test its own isolated pool so
// config-key writes in one test can't bleed into another.
func setupDelegateIsolationTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "delegate_worktree_isolation_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestDelegateWorktreeIsolation_DefaultsTrue verifies the escape hatch
// defaults to enabled absent any config row.
func TestDelegateWorktreeIsolation_DefaultsTrue(t *testing.T) {
	t.Parallel()
	pool := setupDelegateIsolationTestPool(t)

	if !DelegateWorktreeIsolation(pool, "proj-1") {
		t.Error("DelegateWorktreeIsolation() = false, want true by default")
	}
}

// TestDelegateWorktreeIsolation_GlobalFalse verifies a global config="false"
// disables isolation for every project.
func TestDelegateWorktreeIsolation_GlobalFalse(t *testing.T) {
	t.Parallel()
	pool := setupDelegateIsolationTestPool(t)

	if err := pool.SetConfig(DelegateWorktreeIsolationKey, "false"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if DelegateWorktreeIsolation(pool, "proj-1") {
		t.Error("DelegateWorktreeIsolation() = true, want false with global override")
	}
}

// TestDelegateWorktreeIsolation_ProjectOverrideWinsOverGlobal verifies a
// project-scoped override takes precedence over a global setting, in both
// directions.
func TestDelegateWorktreeIsolation_ProjectOverrideWinsOverGlobal(t *testing.T) {
	t.Parallel()
	pool := setupDelegateIsolationTestPool(t)

	if err := pool.SetConfig(DelegateWorktreeIsolationKey, "false"); err != nil {
		t.Fatalf("SetConfig global: %v", err)
	}
	if err := pool.SetProjectConfig("proj-1", DelegateWorktreeIsolationKey, "true"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	if !DelegateWorktreeIsolation(pool, "proj-1") {
		t.Error("DelegateWorktreeIsolation(proj-1) = false, want true (project override beats global false)")
	}
	if DelegateWorktreeIsolation(pool, "proj-2") {
		t.Error("DelegateWorktreeIsolation(proj-2) = true, want false (falls through to global)")
	}
}

// TestDelegateWorktreeIsolation_CaseInsensitive verifies the "false" match is
// case-insensitive and trims whitespace, mirroring SubworkflowToolsEnabled's
// shape.
func TestDelegateWorktreeIsolation_CaseInsensitive(t *testing.T) {
	t.Parallel()
	pool := setupDelegateIsolationTestPool(t)

	if err := pool.SetConfig(DelegateWorktreeIsolationKey, " FALSE "); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if DelegateWorktreeIsolation(pool, "proj-1") {
		t.Error("DelegateWorktreeIsolation() = true, want false for ' FALSE ' (case/whitespace insensitive)")
	}
}
