package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func setupCaptureThinkingTestEnv(t *testing.T) (*GlobalSettingsService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "capture_thinking_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewGlobalSettingsService(pool, clock.Real()), "proj-capture-thinking-test"
}

func TestGetCaptureThinkingEnabled_DefaultFalse(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	enabled, err := svc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled() error: %v", err)
	}
	if enabled {
		t.Errorf("default capture_thinking_enabled = true, want false")
	}
}

func TestGetCaptureThinkingEnabled_GlobalTrueOnly(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	if err := svc.SetCaptureThinkingEnabled(true); err != nil {
		t.Fatalf("SetCaptureThinkingEnabled(true): %v", err)
	}
	enabled, err := svc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled() error: %v", err)
	}
	if !enabled {
		t.Errorf("global=true, no project override → enabled = false, want true")
	}
}

func TestGetCaptureThinkingEnabled_GlobalFalseOnly(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	if err := svc.SetCaptureThinkingEnabled(false); err != nil {
		t.Fatalf("SetCaptureThinkingEnabled(false): %v", err)
	}
	enabled, err := svc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled() error: %v", err)
	}
	if enabled {
		t.Errorf("global=false, no project override → enabled = true, want false")
	}
}

func TestGetCaptureThinkingEnabled_ProjectTrueOverridesGlobalFalse(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	if err := svc.SetCaptureThinkingEnabled(false); err != nil {
		t.Fatalf("SetCaptureThinkingEnabled(false): %v", err)
	}
	if err := svc.SetCaptureThinkingEnabledForProject(projectID, boolPtr(true)); err != nil {
		t.Fatalf("SetCaptureThinkingEnabledForProject(true): %v", err)
	}
	enabled, err := svc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled() error: %v", err)
	}
	if !enabled {
		t.Errorf("global=false, project=true → enabled = false, want true")
	}
}

func TestGetCaptureThinkingEnabled_ProjectFalseOverridesGlobalTrue(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	if err := svc.SetCaptureThinkingEnabled(true); err != nil {
		t.Fatalf("SetCaptureThinkingEnabled(true): %v", err)
	}
	if err := svc.SetCaptureThinkingEnabledForProject(projectID, boolPtr(false)); err != nil {
		t.Fatalf("SetCaptureThinkingEnabledForProject(false): %v", err)
	}
	enabled, err := svc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled() error: %v", err)
	}
	if enabled {
		t.Errorf("global=true, project=false → enabled = true, want false")
	}
}

func TestSetCaptureThinkingEnabledForProject_NilClearsToGlobalTrue(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	if err := svc.SetCaptureThinkingEnabled(true); err != nil {
		t.Fatalf("SetCaptureThinkingEnabled(true): %v", err)
	}
	// Set project-level override first.
	if err := svc.SetCaptureThinkingEnabledForProject(projectID, boolPtr(false)); err != nil {
		t.Fatalf("SetCaptureThinkingEnabledForProject(false): %v", err)
	}
	// Clear project override via nil.
	if err := svc.SetCaptureThinkingEnabledForProject(projectID, nil); err != nil {
		t.Fatalf("SetCaptureThinkingEnabledForProject(nil): %v", err)
	}
	enabled, err := svc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled() after clear: %v", err)
	}
	if !enabled {
		t.Errorf("after clearing project override, global=true → enabled = false, want true")
	}
}

func TestSetCaptureThinkingEnabledForProject_NilClearsToGlobalFalse(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	if err := svc.SetCaptureThinkingEnabled(false); err != nil {
		t.Fatalf("SetCaptureThinkingEnabled(false): %v", err)
	}
	// Set project-level override first.
	if err := svc.SetCaptureThinkingEnabledForProject(projectID, boolPtr(true)); err != nil {
		t.Fatalf("SetCaptureThinkingEnabledForProject(true): %v", err)
	}
	// Clear project override via nil.
	if err := svc.SetCaptureThinkingEnabledForProject(projectID, nil); err != nil {
		t.Fatalf("SetCaptureThinkingEnabledForProject(nil): %v", err)
	}
	enabled, err := svc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled() after clear: %v", err)
	}
	if enabled {
		t.Errorf("after clearing project override, global=false → enabled = true, want false")
	}
}

func TestSetCaptureThinkingEnabled_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, projectID := setupCaptureThinkingTestEnv(t)

	cases := []struct {
		set  bool
		want bool
	}{
		{true, true},
		{false, false},
		{true, true},
	}
	for _, tc := range cases {
		if err := svc.SetCaptureThinkingEnabled(tc.set); err != nil {
			t.Fatalf("SetCaptureThinkingEnabled(%v): %v", tc.set, err)
		}
		got, err := svc.GetCaptureThinkingEnabled(projectID)
		if err != nil {
			t.Fatalf("GetCaptureThinkingEnabled() error: %v", err)
		}
		if got != tc.want {
			t.Errorf("SetCaptureThinkingEnabled(%v) → GetCaptureThinkingEnabled() = %v, want %v",
				tc.set, got, tc.want)
		}
	}
}

func TestGetCaptureThinkingEnabled_UnknownProjectReturnsDefault(t *testing.T) {
	t.Parallel()
	svc, _ := setupCaptureThinkingTestEnv(t)

	enabled, err := svc.GetCaptureThinkingEnabled("unknown-project-xyz")
	if err != nil {
		t.Fatalf("GetCaptureThinkingEnabled(unknown) error: %v", err)
	}
	if enabled {
		t.Errorf("unknown project capture_thinking_enabled = true, want false")
	}
}
