package service

import (
	"path/filepath"
	"testing"

	"be/internal/db"
)

func newStepwiseCapTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("svcCopyTemplateDB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestStepRejectionCap_DefaultsWhenUnset verifies the fallback default
// applies with no config rows at all.
func TestStepRejectionCap_DefaultsWhenUnset(t *testing.T) {
	t.Parallel()
	pool := newStepwiseCapTestPool(t)
	if got := StepRejectionCap(pool, "proj-x"); got != DefaultStepRejectionCap {
		t.Errorf("StepRejectionCap = %d, want default %d", got, DefaultStepRejectionCap)
	}
}

// TestStepRejectionCap_GlobalOverridesDefault verifies a global config value
// wins over the hardcoded default.
func TestStepRejectionCap_GlobalOverridesDefault(t *testing.T) {
	t.Parallel()
	pool := newStepwiseCapTestPool(t)
	if err := pool.SetConfig(StepRejectionCapKey, "8"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := StepRejectionCap(pool, "proj-x"); got != 8 {
		t.Errorf("StepRejectionCap = %d, want 8 (global)", got)
	}
}

// TestStepRejectionCap_ProjectOverridesGlobal verifies project-scoped config
// wins over a global value for that project only.
func TestStepRejectionCap_ProjectOverridesGlobal(t *testing.T) {
	t.Parallel()
	pool := newStepwiseCapTestPool(t)
	if err := pool.SetConfig(StepRejectionCapKey, "8"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := pool.SetProjectConfig("proj-a", StepRejectionCapKey, "3"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	if got := StepRejectionCap(pool, "proj-a"); got != 3 {
		t.Errorf("StepRejectionCap(proj-a) = %d, want 3 (project override)", got)
	}
	if got := StepRejectionCap(pool, "proj-b"); got != 8 {
		t.Errorf("StepRejectionCap(proj-b) = %d, want 8 (falls back to global)", got)
	}
}

// TestStepRejectionCap_InvalidValueFallsBackToDefault covers unparsable and
// sub-1 values from either scope degrading to the default rather than
// erroring or admitting a zero/negative cap.
func TestStepRejectionCap_InvalidValueFallsBackToDefault(t *testing.T) {
	t.Parallel()
	cases := []string{"", "not-a-number", "0", "-5"}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			pool := newStepwiseCapTestPool(t)
			if raw != "" {
				if err := pool.SetConfig(StepRejectionCapKey, raw); err != nil {
					t.Fatalf("SetConfig(%q): %v", raw, err)
				}
			}
			if got := StepRejectionCap(pool, "proj-x"); got != DefaultStepRejectionCap {
				t.Errorf("StepRejectionCap with config=%q = %d, want default %d", raw, got, DefaultStepRejectionCap)
			}
		})
	}
}
