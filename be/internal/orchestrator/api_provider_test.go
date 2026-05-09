package orchestrator

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// TestLoadProjectEnv_HappyPath verifies that existing project env vars are returned
// as "KEY=value" formatted strings in the correct order.
func TestLoadProjectEnv_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(env.pool, clk)
	if _, err := r.Upsert(env.project, "MY_KEY", "my_value"); err != nil {
		t.Fatalf("Upsert MY_KEY: %v", err)
	}
	if _, err := r.Upsert(env.project, "ANOTHER_KEY", "another_value"); err != nil {
		t.Fatalf("Upsert ANOTHER_KEY: %v", err)
	}

	got := loadProjectEnv(context.Background(), env.pool, env.project, clk)

	wantSet := map[string]bool{
		"MY_KEY=my_value":           true,
		"ANOTHER_KEY=another_value": true,
	}
	if len(got) != len(wantSet) {
		t.Fatalf("loadProjectEnv returned %d entries, want %d; got=%v", len(got), len(wantSet), got)
	}
	for _, entry := range got {
		if !wantSet[entry] {
			t.Errorf("unexpected entry %q in result", entry)
		}
	}
}

// TestLoadProjectEnv_Empty verifies that a project with no env vars returns an empty result.
func TestLoadProjectEnv_Empty(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	got := loadProjectEnv(context.Background(), env.pool, env.project, clock.Real())
	if len(got) != 0 {
		t.Errorf("loadProjectEnv with no vars = %v, want empty", got)
	}
}

// TestLoadProjectEnv_NonexistentProject verifies graceful degradation: an unknown
// project ID returns an empty slice and does not panic or error.
func TestLoadProjectEnv_NonexistentProject(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	got := loadProjectEnv(context.Background(), env.pool, "nonexistent-project-id-xyz", clock.Real())
	if len(got) != 0 {
		t.Errorf("loadProjectEnv with unknown project = %v, want empty", got)
	}
}

// TestLoadProjectEnv_FormatsAsKeyEqualsValue verifies the "KEY=value" format of each entry.
func TestLoadProjectEnv_FormatsAsKeyEqualsValue(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(env.pool, clk)
	if _, err := r.Upsert(env.project, "API_BASE_URL", "https://example.com/api"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got := loadProjectEnv(context.Background(), env.pool, env.project, clk)
	if len(got) != 1 {
		t.Fatalf("loadProjectEnv returned %d entries, want 1; got=%v", len(got), got)
	}
	if got[0] != "API_BASE_URL=https://example.com/api" {
		t.Errorf("entry = %q, want API_BASE_URL=https://example.com/api", got[0])
	}
}
