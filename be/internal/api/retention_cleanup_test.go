package api

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// newRetentionCleanupDB sets up an isolated DB for retention cleanup tests.
func newRetentionCleanupDB(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "retention_cleanup_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedProjectAndInstances inserts a project and N completed workflow instances.
// Returns the project ID.
func seedProjectAndInstances(t *testing.T, pool *db.Pool, projectID string, n int) {
	t.Helper()
	clk := clock.Real()
	projectRepo := repo.NewProjectRepo(pool, clk)
	if err := projectRepo.Create(&model.Project{
		ID:   projectID,
		Name: "Retention Test Project " + projectID,
	}); err != nil {
		t.Fatalf("create project %s: %v", projectID, err)
	}

	// Insert a minimal workflow definition required by FK constraints.
	wfID := projectID + "-wf"
	_, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		wfID, projectID, "test workflow", "ticket")
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		id := projectID + "-wfi-" + string(rune('a'+i))
		ts := now.Add(time.Duration(-n+i) * time.Second).Format(time.RFC3339Nano)
		_, err := pool.Exec(`INSERT INTO workflow_instances
			(id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, projectID, "TKT-"+id, wfID, string(model.WorkflowInstanceCompleted), "ticket", ts, ts)
		if err != nil {
			t.Fatalf("create workflow instance %d: %v", i, err)
		}
	}
}

// countInstances returns the number of workflow_instances rows for the given project.
func countInstances(t *testing.T, pool *db.Pool, projectID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances WHERE LOWER(project_id) = LOWER(?)`,
		projectID).Scan(&count); err != nil {
		t.Fatalf("count instances for %s: %v", projectID, err)
	}
	return count
}

// runCleanupFor mirrors the inner loop body from server.startRetentionCleanup
// but operates only on a single named project.
func runCleanupFor(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	clk := clock.Real()
	svc := service.NewGlobalSettingsService(pool, clk)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clk)

	enabled, err := svc.GetWorkflowCleanupEnabled(projectID)
	if err != nil {
		t.Fatalf("GetWorkflowCleanupEnabled: %v", err)
	}
	if !enabled {
		return
	}
	keep, err := svc.GetSessionRetentionLimit(projectID)
	if err != nil {
		t.Fatalf("GetSessionRetentionLimit: %v", err)
	}
	if keep <= 0 {
		return
	}
	if _, err := wfiRepo.CleanupKeepLatestForProject(projectID, keep); err != nil {
		t.Fatalf("CleanupKeepLatestForProject: %v", err)
	}
}

// TestRetentionCleanup_Disabled verifies cleanup does nothing when disabled.
func TestRetentionCleanup_Disabled(t *testing.T) {
	const n = 10
	pool := newRetentionCleanupDB(t)
	projectID := "proj-cleanup-disabled"
	seedProjectAndInstances(t, pool, projectID, n)

	// cleanup_enabled defaults to false — no-op expected
	runCleanupFor(t, pool, projectID)

	if got := countInstances(t, pool, projectID); got != n {
		t.Errorf("disabled cleanup: want %d instances, got %d", n, got)
	}
}

// TestRetentionCleanup_EnabledNoLimit verifies cleanup does nothing when limit is 0 (unset).
func TestRetentionCleanup_EnabledNoLimit(t *testing.T) {
	const n = 10
	pool := newRetentionCleanupDB(t)
	projectID := "proj-cleanup-nolimit"
	seedProjectAndInstances(t, pool, projectID, n)

	svc := service.NewGlobalSettingsService(pool, clock.Real())
	if err := svc.SetWorkflowCleanupEnabled(projectID, true); err != nil {
		t.Fatalf("SetWorkflowCleanupEnabled: %v", err)
	}
	// retention limit stays at 0 (sentinel, unset) — cleanup should skip

	runCleanupFor(t, pool, projectID)

	if got := countInstances(t, pool, projectID); got != n {
		t.Errorf("enabled+no-limit cleanup: want %d instances, got %d", n, got)
	}
}

// TestRetentionCleanup_EnabledWithLimit verifies cleanup keeps only the N newest instances.
func TestRetentionCleanup_EnabledWithLimit(t *testing.T) {
	const n = 30
	const keep = 25
	pool := newRetentionCleanupDB(t)
	projectID := "proj-cleanup-limited"
	seedProjectAndInstances(t, pool, projectID, n)

	svc := service.NewGlobalSettingsService(pool, clock.Real())
	if err := svc.SetWorkflowCleanupEnabled(projectID, true); err != nil {
		t.Fatalf("SetWorkflowCleanupEnabled: %v", err)
	}
	if err := svc.SetSessionRetentionLimit(projectID, keep); err != nil {
		t.Fatalf("SetSessionRetentionLimit: %v", err)
	}

	runCleanupFor(t, pool, projectID)

	if got := countInstances(t, pool, projectID); got != keep {
		t.Errorf("cleanup with limit=%d: want %d instances remaining, got %d", keep, keep, got)
	}
}

// TestRetentionCleanup_OnlyAffectsTargetProject verifies cleanup does not touch other projects.
func TestRetentionCleanup_OnlyAffectsTargetProject(t *testing.T) {
	const n = 15
	const keep = 10
	pool := newRetentionCleanupDB(t)

	projectA := "proj-cleanup-a"
	projectB := "proj-cleanup-b"
	seedProjectAndInstances(t, pool, projectA, n)
	seedProjectAndInstances(t, pool, projectB, n)

	svc := service.NewGlobalSettingsService(pool, clock.Real())
	if err := svc.SetWorkflowCleanupEnabled(projectA, true); err != nil {
		t.Fatalf("SetWorkflowCleanupEnabled projectA: %v", err)
	}
	if err := svc.SetSessionRetentionLimit(projectA, keep); err != nil {
		t.Fatalf("SetSessionRetentionLimit projectA: %v", err)
	}
	// projectB: enabled but no limit set (keep = 0)
	if err := svc.SetWorkflowCleanupEnabled(projectB, true); err != nil {
		t.Fatalf("SetWorkflowCleanupEnabled projectB: %v", err)
	}

	runCleanupFor(t, pool, projectA)
	runCleanupFor(t, pool, projectB)

	if got := countInstances(t, pool, projectA); got != keep {
		t.Errorf("projectA: want %d instances, got %d", keep, got)
	}
	if got := countInstances(t, pool, projectB); got != n {
		t.Errorf("projectB (no-limit): want %d instances (untouched), got %d", n, got)
	}
}

// TestRetentionCleanup_GetSessionRetentionLimit_Zero confirms default is 0 (sentinel).
func TestRetentionCleanup_GetSessionRetentionLimit_Zero(t *testing.T) {
	pool := newRetentionCleanupDB(t)
	svc := service.NewGlobalSettingsService(pool, clock.Real())

	limit, err := svc.GetSessionRetentionLimit("any-project")
	if err != nil {
		t.Fatalf("GetSessionRetentionLimit: %v", err)
	}
	if limit != 0 {
		t.Errorf("unset retention limit = %d, want 0 (sentinel)", limit)
	}
}
