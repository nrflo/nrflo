package service

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// clearProviderEnv blanks every server-env credential source so the tests are
// deterministic on hosts that have real keys exported. t.Setenv also
// forbids t.Parallel, keeping the env mutation race-free.
func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN", "OPENAI_API_KEY", "CODEX_API_KEY", "OPENROUTER_API_KEY"} {
		t.Setenv(k, "")
	}
}

func TestHasAPICredentials_MissingEverywhere(t *testing.T) {
	clearProviderEnv(t)
	pool, projectID := setupAPIProviderTest(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	for _, prov := range []string{"anthropic", "openai", "openrouter"} {
		if HasAPICredentials(context.Background(), pool, clk, prov, projectID) {
			t.Errorf("HasAPICredentials(%q) = true, want false with no credential source", prov)
		}
	}
}

func TestHasAPICredentials_PerProjectEnvVar(t *testing.T) {
	clearProviderEnv(t)
	pool, projectID := setupAPIProviderTest(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	r := repo.NewProjectEnvVarRepo(pool, clk)
	if _, err := r.Upsert(projectID, "ANTHROPIC_API_KEY", "sk-ant-api03-test"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !HasAPICredentials(context.Background(), pool, clk, "anthropic", projectID) {
		t.Error("HasAPICredentials(anthropic) = false, want true with per-project key")
	}
}

func TestHasAPICredentials_ServerEnv(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "sk-ant-oat01-test")
	pool, projectID := setupAPIProviderTest(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	if !HasAPICredentials(context.Background(), pool, clk, "anthropic", projectID) {
		t.Error("HasAPICredentials(anthropic) = false, want true with server-env OAuth token")
	}
}

// TestHasAPICredentials_NonBuiltinReportsTrue: custom-registry providers carry
// credentials in their row, so the static check must defer to the build.
func TestHasAPICredentials_NonBuiltinReportsTrue(t *testing.T) {
	clearProviderEnv(t)
	pool, projectID := setupAPIProviderTest(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	if !HasAPICredentials(context.Background(), pool, clk, "my-custom", projectID) {
		t.Error("HasAPICredentials(my-custom) = false, want true (non-builtin defers to build)")
	}
}
