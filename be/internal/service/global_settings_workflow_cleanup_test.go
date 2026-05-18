package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func setupCleanupTestEnv(t *testing.T) (*GlobalSettingsService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cleanup_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewGlobalSettingsService(pool, clock.Real()), "proj-cleanup-test"
}

func TestGetWorkflowCleanupEnabled_DefaultFalse(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCleanupTestEnv(t)

	enabled, err := svc.GetWorkflowCleanupEnabled(projectID)
	if err != nil {
		t.Fatalf("GetWorkflowCleanupEnabled() error: %v", err)
	}
	if enabled {
		t.Errorf("default cleanup enabled = true, want false")
	}
}

func TestSetWorkflowCleanupEnabled_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCleanupTestEnv(t)

	if err := svc.SetWorkflowCleanupEnabled(projectID, true); err != nil {
		t.Fatalf("SetWorkflowCleanupEnabled(true): %v", err)
	}
	enabled, err := svc.GetWorkflowCleanupEnabled(projectID)
	if err != nil {
		t.Fatalf("GetWorkflowCleanupEnabled() after set true: %v", err)
	}
	if !enabled {
		t.Errorf("enabled = false, want true after setting true")
	}

	if err := svc.SetWorkflowCleanupEnabled(projectID, false); err != nil {
		t.Fatalf("SetWorkflowCleanupEnabled(false): %v", err)
	}
	enabled, err = svc.GetWorkflowCleanupEnabled(projectID)
	if err != nil {
		t.Fatalf("GetWorkflowCleanupEnabled() after set false: %v", err)
	}
	if enabled {
		t.Errorf("enabled = true, want false after setting false")
	}
}

func TestGetSessionRetentionLimit_DefaultIsZero(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCleanupTestEnv(t)

	limit, err := svc.GetSessionRetentionLimit(projectID)
	if err != nil {
		t.Fatalf("GetSessionRetentionLimit() error: %v", err)
	}
	if limit != 0 {
		t.Errorf("default retention limit = %d, want 0", limit)
	}
}

func TestSetSessionRetentionLimit_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCleanupTestEnv(t)

	if err := svc.SetSessionRetentionLimit(projectID, 500); err != nil {
		t.Fatalf("SetSessionRetentionLimit(500): %v", err)
	}
	got, err := svc.GetSessionRetentionLimit(projectID)
	if err != nil {
		t.Fatalf("GetSessionRetentionLimit(): %v", err)
	}
	if got != 500 {
		t.Errorf("retention limit = %d, want 500", got)
	}
}

func TestSetSessionRetentionLimit_RejectsBelow10(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCleanupTestEnv(t)

	cases := []struct{ n int }{
		{0}, {1}, {9}, {-1},
	}
	for _, tc := range cases {
		err := svc.SetSessionRetentionLimit(projectID, tc.n)
		if err == nil {
			t.Errorf("SetSessionRetentionLimit(%d) expected error, got nil", tc.n)
		}
	}

	// Limit should remain at default (0 = not configured).
	got, err := svc.GetSessionRetentionLimit(projectID)
	if err != nil {
		t.Fatalf("GetSessionRetentionLimit(): %v", err)
	}
	if got != 0 {
		t.Errorf("limit after rejected set = %d, want 0", got)
	}
}

func TestSetSessionRetentionLimit_ExactlyTenIsValid(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCleanupTestEnv(t)

	if err := svc.SetSessionRetentionLimit(projectID, 10); err != nil {
		t.Fatalf("SetSessionRetentionLimit(10) unexpected error: %v", err)
	}
	got, err := svc.GetSessionRetentionLimit(projectID)
	if err != nil {
		t.Fatalf("GetSessionRetentionLimit(): %v", err)
	}
	if got != 10 {
		t.Errorf("retention limit = %d, want 10", got)
	}
}

func TestGetWorkflowCleanupEnabled_UnknownProjectReturnsDefault(t *testing.T) {
	t.Parallel()
	svc, _ := setupCleanupTestEnv(t)

	enabled, err := svc.GetWorkflowCleanupEnabled("unknown-project-xyz")
	if err != nil {
		t.Fatalf("GetWorkflowCleanupEnabled(unknown) error: %v", err)
	}
	if enabled {
		t.Errorf("unknown project cleanup enabled = true, want false")
	}
}

func TestGetSessionRetentionLimit_UnknownProjectReturnsZero(t *testing.T) {
	t.Parallel()
	svc, _ := setupCleanupTestEnv(t)

	limit, err := svc.GetSessionRetentionLimit("unknown-project-xyz")
	if err != nil {
		t.Fatalf("GetSessionRetentionLimit(unknown) error: %v", err)
	}
	if limit != 0 {
		t.Errorf("unknown project retention limit = %d, want 0", limit)
	}
}
