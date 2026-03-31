package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// TestSystemAgentDef_SeededRowAccessible verifies that the conflict-resolver row
// seeded by migration 000039 is accessible via the service layer immediately after
// DB creation, without any additional setup or manual seeding.
func TestSystemAgentDef_SeededRowAccessible(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed_accessible.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewSystemAgentDefinitionService(pool, clock.Real())

	def, err := svc.Get("conflict-resolver")
	if err != nil {
		t.Fatalf("Get seeded conflict-resolver: %v", err)
	}

	if def.Model != "sonnet" {
		t.Errorf("seeded model = %q, want %q", def.Model, "sonnet")
	}
	if def.Timeout != 20 {
		t.Errorf("seeded timeout = %d, want 20", def.Timeout)
	}
	if !strings.Contains(def.Prompt, "Merge Conflict Resolver") {
		t.Errorf("seeded prompt missing 'Merge Conflict Resolver'")
	}
	// Optional fields must be nil — spawner uses built-in defaults when NULL.
	if def.RestartThreshold != nil {
		t.Errorf("seeded restart_threshold = %v, want nil", *def.RestartThreshold)
	}
	if def.MaxFailRestarts != nil {
		t.Errorf("seeded max_fail_restarts = %v, want nil", *def.MaxFailRestarts)
	}
}

// TestSystemAgentDef_SeededRowInList verifies that the seeded conflict-resolver row
// appears in the List response.
func TestSystemAgentDef_SeededRowInList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed_list.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewSystemAgentDefinitionService(pool, clock.Real())

	defs, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := false
	for _, d := range defs {
		if d.ID == "conflict-resolver" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("List does not include seeded conflict-resolver; got %d defs", len(defs))
	}
}

// TestSystemAgentDef_SeededRowDeleteAndRecreate verifies that after deleting the
// migration-seeded row, a new conflict-resolver can be created with different values.
func TestSystemAgentDef_SeededRowDeleteAndRecreate(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	// Start state: migration-seeded conflict-resolver exists with model=sonnet.
	orig, err := svc.Get("conflict-resolver")
	if err != nil {
		t.Fatalf("Get initial seeded row: %v", err)
	}
	if orig.Model != "sonnet" {
		t.Fatalf("seeded model = %q, want sonnet", orig.Model)
	}

	// Delete seeded row.
	if err := svc.Delete("conflict-resolver"); err != nil {
		t.Fatalf("Delete seeded row: %v", err)
	}

	// Should be gone.
	if _, err := svc.Get("conflict-resolver"); err == nil {
		t.Fatal("expected not-found after Delete seeded row, got nil")
	}

	// Recreate with custom values — should succeed now that the row is gone.
	newDef, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "conflict-resolver",
		Model:  "opus",
		Prompt: "custom prompt for testing",
	})
	if err != nil {
		t.Fatalf("Create after Delete: %v", err)
	}
	if newDef.Model != "opus" {
		t.Errorf("recreated model = %q, want opus", newDef.Model)
	}
}

// TestSystemAgentDef_SeededConflictResolverPromptVars verifies that the seeded
// prompt contains the ExtraVars placeholders used by the spawner for conflict resolution.
func TestSystemAgentDef_SeededConflictResolverPromptVars(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed_vars.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewSystemAgentDefinitionService(pool, clock.Real())

	def, err := svc.Get("conflict-resolver")
	if err != nil {
		t.Fatalf("Get seeded conflict-resolver: %v", err)
	}

	for _, v := range []string{"${BRANCH_NAME}", "${DEFAULT_BRANCH}", "${MERGE_ERROR}"} {
		if !strings.Contains(def.Prompt, v) {
			t.Errorf("seeded prompt missing ExtraVars placeholder %q", v)
		}
	}
}
