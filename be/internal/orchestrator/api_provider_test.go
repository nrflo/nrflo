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

// TestProjectEnvAdapter_Get_HappyPath verifies that a var seeded in the DB is returned
// by the adapter's Get method.
func TestProjectEnvAdapter_Get_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(env.pool, clk)
	if _, err := r.Upsert(env.project, "ANTHROPIC_API_KEY", "sk-ant-api03-test"); err != nil {
		t.Fatalf("Upsert ANTHROPIC_API_KEY: %v", err)
	}

	adapter := newProjectEnvAdapter(env.pool, clk, env.project)
	v, ok, err := adapter.Get(env.project, "ANTHROPIC_API_KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get ok=false, want true")
	}
	if v != "sk-ant-api03-test" {
		t.Errorf("Get value = %q, want %q", v, "sk-ant-api03-test")
	}
}

// TestProjectEnvAdapter_Get_Missing verifies that a missing var returns ok=false.
func TestProjectEnvAdapter_Get_Missing(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	adapter := newProjectEnvAdapter(env.pool, clock.Real(), env.project)
	_, ok, err := adapter.Get(env.project, "ANTHROPIC_API_KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Error("Get ok=true for missing var, want false")
	}
}

// TestProjectEnvAdapter_IgnoresProjectIDArg verifies the adapter is pre-scoped: the
// projectID argument to Get is ignored since vars are loaded at construction time.
func TestProjectEnvAdapter_IgnoresProjectIDArg(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(env.pool, clk)
	if _, err := r.Upsert(env.project, "MY_VAR", "myval"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	adapter := newProjectEnvAdapter(env.pool, clk, env.project)
	v, ok, err := adapter.Get("other-project-id", "MY_VAR")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get ok=false; pre-scoped adapter should return var regardless of projectID arg")
	}
	if v != "myval" {
		t.Errorf("Get value = %q, want %q", v, "myval")
	}
}

// TestBuildAPIProvider_ReturnNilWhenNoKey verifies buildAPIProvider returns nil when no
// Anthropic API key is resolvable from either per-project env or server env.
func TestBuildAPIProvider_ReturnNilWhenNoKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	env := newTestEnv(t)

	got := buildAPIProvider(context.Background(), env.pool, env.project, clock.Real())
	if got != nil {
		t.Error("buildAPIProvider = non-nil, want nil when no key is configured")
	}
}

// TestBuildAPIProvider_ReturnsProviderFromProjectEnv verifies that ANTHROPIC_API_KEY set as
// a per-project env var results in a non-nil provider, with no DB credential row required.
func TestBuildAPIProvider_ReturnsProviderFromProjectEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	env := newTestEnv(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(env.pool, clk)
	if _, err := r.Upsert(env.project, "ANTHROPIC_API_KEY", "sk-ant-api03-proj"); err != nil {
		t.Fatalf("Upsert ANTHROPIC_API_KEY: %v", err)
	}

	got := buildAPIProvider(context.Background(), env.pool, env.project, clk)
	if got == nil {
		t.Error("buildAPIProvider = nil, want non-nil provider when ANTHROPIC_API_KEY is in project env")
	}
}
