package service

import (
	"path/filepath"
	"testing"

	"be/internal/db"
)

// openPlanAutoTestPool copies the shared migrated template DB and opens it
// without re-running migrations (see be/CLAUDE.md "DB tests never migrate
// per-test"), inserting a project row for FK-shaped realism.
func openPlanAutoTestPool(t *testing.T) (*db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "plan_auto.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const projectID = "proj-plan-auto"
	now := "2026-01-01T00:00:00Z"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, NULL, ?, ?)`,
		projectID, "Plan Auto Test Project", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return pool, projectID
}

func TestDynamicAutoEnabled_DefaultFalse(t *testing.T) {
	t.Parallel()
	pool, projectID := openPlanAutoTestPool(t)

	if got := DynamicAutoEnabled(pool, projectID); got {
		t.Errorf("DynamicAutoEnabled = %v, want false (no config set)", got)
	}
}

func TestDynamicAutoEnabled_GlobalOverride(t *testing.T) {
	t.Parallel()
	pool, projectID := openPlanAutoTestPool(t)

	if err := pool.SetConfig(DynamicWorkflowAutoEnabledKey, "true"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	if got := DynamicAutoEnabled(pool, projectID); !got {
		t.Errorf("DynamicAutoEnabled = %v, want true (global override set)", got)
	}
	// Any project (even one never explicitly configured) sees the global default.
	if got := DynamicAutoEnabled(pool, "some-other-project-never-configured"); !got {
		t.Errorf("DynamicAutoEnabled (unconfigured project) = %v, want true (global override applies)", got)
	}
}

func TestDynamicAutoEnabled_ProjectOverridesGlobal(t *testing.T) {
	t.Parallel()
	pool, projectID := openPlanAutoTestPool(t)

	if err := pool.SetConfig(DynamicWorkflowAutoEnabledKey, "true"); err != nil {
		t.Fatalf("SetConfig (global): %v", err)
	}
	if err := pool.SetProjectConfig(projectID, DynamicWorkflowAutoEnabledKey, "false"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	if got := DynamicAutoEnabled(pool, projectID); got {
		t.Errorf("DynamicAutoEnabled = %v, want false (project override wins over global true)", got)
	}
}

func TestDynamicAutoEnabled_ProjectOverrideEnablesWhenGlobalOff(t *testing.T) {
	t.Parallel()
	pool, projectID := openPlanAutoTestPool(t)

	// Global left unset (falls back to default false).
	if err := pool.SetProjectConfig(projectID, DynamicWorkflowAutoEnabledKey, "true"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	if got := DynamicAutoEnabled(pool, projectID); !got {
		t.Errorf("DynamicAutoEnabled = %v, want true (project override can enable when global is off)", got)
	}
}

func TestDynamicAutoEnabled_CaseAndWhitespace(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value string
		want  bool
	}{
		{"lower", "true", true},
		{"upper", "TRUE", true},
		{"mixed", "True", true},
		{"padded", " true ", true}, // implementation strings.TrimSpace's before comparing
		{"false", "false", false},
		{"empty", "", false},
		{"garbage", "yes", false},
		{"truthy-substr", "true-ish", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, projectID := openPlanAutoTestPool(t)
			if tc.value != "" {
				if err := pool.SetProjectConfig(projectID, DynamicWorkflowAutoEnabledKey, tc.value); err != nil {
					t.Fatalf("SetProjectConfig: %v", err)
				}
			}
			if got := DynamicAutoEnabled(pool, projectID); got != tc.want {
				t.Errorf("DynamicAutoEnabled(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
