package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// setupAPIProviderTest returns a fresh pool + a seeded project id, for
// projectEnvAdapter/BuildAPIProvider tests.
func setupAPIProviderTest(t *testing.T) (*db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api_provider_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "proj-api-provider"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'TestProject', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return pool, projectID
}

// TestProjectEnvAdapter_Get_HappyPath verifies that a var seeded in the DB is
// returned by the adapter's Get method.
func TestProjectEnvAdapter_Get_HappyPath(t *testing.T) {
	t.Parallel()
	pool, projectID := setupAPIProviderTest(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(pool, clk)
	if _, err := r.Upsert(projectID, "ANTHROPIC_API_KEY", "sk-ant-api03-test"); err != nil {
		t.Fatalf("Upsert ANTHROPIC_API_KEY: %v", err)
	}

	adapter := newProjectEnvAdapter(pool, clk, projectID)
	v, ok, err := adapter.Get(projectID, "ANTHROPIC_API_KEY")
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
	pool, projectID := setupAPIProviderTest(t)

	adapter := newProjectEnvAdapter(pool, clock.Real(), projectID)
	_, ok, err := adapter.Get(projectID, "ANTHROPIC_API_KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Error("Get ok=true for missing var, want false")
	}
}

// TestProjectEnvAdapter_IgnoresProjectIDArg verifies the adapter is
// pre-scoped: the projectID argument to Get is ignored since vars are loaded
// at construction time.
func TestProjectEnvAdapter_IgnoresProjectIDArg(t *testing.T) {
	t.Parallel()
	pool, projectID := setupAPIProviderTest(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(pool, clk)
	if _, err := r.Upsert(projectID, "MY_VAR", "myval"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	adapter := newProjectEnvAdapter(pool, clk, projectID)
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

// TestBuildAPIProvider_UnknownProvider_ReturnsError verifies an unknown
// provider name errors without touching credentials.
func TestBuildAPIProvider_UnknownProvider_ReturnsError(t *testing.T) {
	t.Parallel()
	pool, projectID := setupAPIProviderTest(t)

	got, err := BuildAPIProvider(context.Background(), pool, clock.Real(), "unknown-llm", projectID)
	if err == nil {
		t.Error("BuildAPIProvider = nil error, want error for unknown provider")
	}
	if got != nil {
		t.Error("BuildAPIProvider = non-nil provider, want nil for unknown provider")
	}
}

// TestBuildAPIProvider_Anthropic_ReturnsProviderFromProjectEnv verifies that
// ANTHROPIC_API_KEY set as a per-project env var resolves to a non-nil provider.
func TestBuildAPIProvider_Anthropic_ReturnsProviderFromProjectEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	pool, projectID := setupAPIProviderTest(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(pool, clk)
	if _, err := r.Upsert(projectID, "ANTHROPIC_API_KEY", "sk-ant-api03-proj"); err != nil {
		t.Fatalf("Upsert ANTHROPIC_API_KEY: %v", err)
	}

	got, err := BuildAPIProvider(context.Background(), pool, clk, "anthropic", projectID)
	if err != nil {
		t.Fatalf("BuildAPIProvider error: %v", err)
	}
	if got == nil {
		t.Error("BuildAPIProvider = nil, want non-nil provider when ANTHROPIC_API_KEY is in project env")
	}
}

// TestBuildAPIProvider_Anthropic_NoKey_ReturnsError verifies BuildAPIProvider
// errors when no Anthropic credential is resolvable.
func TestBuildAPIProvider_Anthropic_NoKey_ReturnsError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	pool, projectID := setupAPIProviderTest(t)

	got, err := BuildAPIProvider(context.Background(), pool, clock.Real(), "anthropic", projectID)
	if err == nil {
		t.Error("BuildAPIProvider = nil error, want error when no Anthropic key is configured")
	}
	if got != nil {
		t.Error("BuildAPIProvider = non-nil provider, want nil on error")
	}
}

// TestBuildAPIProvider_OpenAI_NoKey_ReturnsError verifies BuildAPIProvider
// errors when no OpenAI credential is resolvable.
func TestBuildAPIProvider_OpenAI_NoKey_ReturnsError(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	pool, projectID := setupAPIProviderTest(t)

	got, err := BuildAPIProvider(context.Background(), pool, clock.Real(), "openai", projectID)
	if err == nil {
		t.Error("BuildAPIProvider = nil error, want error when no OpenAI key is configured")
	}
	if got != nil {
		t.Error("BuildAPIProvider = non-nil provider, want nil on error")
	}
}

// TestBuildAPIProvider_OpenAI_ReturnsProviderFromProjectEnv verifies
// OPENAI_API_KEY set as a per-project env var resolves to a non-nil provider.
func TestBuildAPIProvider_OpenAI_ReturnsProviderFromProjectEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	pool, projectID := setupAPIProviderTest(t)

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewProjectEnvVarRepo(pool, clk)
	if _, err := r.Upsert(projectID, "OPENAI_API_KEY", "sk-openai-test-key"); err != nil {
		t.Fatalf("Upsert OPENAI_API_KEY: %v", err)
	}

	got, err := BuildAPIProvider(context.Background(), pool, clk, "openai", projectID)
	if err != nil {
		t.Fatalf("BuildAPIProvider error: %v", err)
	}
	if got == nil {
		t.Error("BuildAPIProvider = nil, want non-nil provider when OPENAI_API_KEY is in project env")
	}
}
