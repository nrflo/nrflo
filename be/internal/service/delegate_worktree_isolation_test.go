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
// defaults to enabled absent any config row (unknown project).
func TestDelegateWorktreeIsolation_DefaultsTrue(t *testing.T) {
	t.Parallel()
	pool := setupDelegateIsolationTestPool(t)

	if !DelegateWorktreeIsolation(pool, "proj-1") {
		t.Error("DelegateWorktreeIsolation() = false, want true by default")
	}
}

// TestDelegateWorktreeIsolation_FollowsProjectWorktreeFlag verifies that
// absent explicit config, isolation follows projects.use_git_worktrees — a
// project that disabled git worktrees gets in-place delegations — and that
// an explicit config key still overrides the flag in both directions.
func TestDelegateWorktreeIsolation_FollowsProjectWorktreeFlag(t *testing.T) {
	t.Parallel()
	pool := setupDelegateIsolationTestPool(t)
	for _, p := range []struct {
		id   string
		flag int
	}{{"proj-nowt", 0}, {"proj-wt", 1}} {
		if _, err := pool.Exec(
			`INSERT INTO projects (id, name, use_git_worktrees, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
			p.id, p.id, p.flag,
		); err != nil {
			t.Fatalf("seed project %s: %v", p.id, err)
		}
	}

	if DelegateWorktreeIsolation(pool, "proj-nowt") {
		t.Error("DelegateWorktreeIsolation(proj-nowt) = true, want false (use_git_worktrees=0)")
	}
	if !DelegateWorktreeIsolation(pool, "proj-wt") {
		t.Error("DelegateWorktreeIsolation(proj-wt) = false, want true (use_git_worktrees=1)")
	}

	// Explicit config beats the project flag.
	if err := pool.SetProjectConfig("proj-nowt", DelegateWorktreeIsolationKey, "true"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}
	if !DelegateWorktreeIsolation(pool, "proj-nowt") {
		t.Error("DelegateWorktreeIsolation(proj-nowt) = false, want true (explicit config overrides flag)")
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
