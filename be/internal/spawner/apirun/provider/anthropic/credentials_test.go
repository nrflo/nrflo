package anthropic

import (
	"context"
	"strings"
	"testing"
)

// fakeEnvRepo is an in-memory ProjectEnvVarRepo. Keys are "projectID|name".
// err lets a test inject an error to verify propagation.
type fakeEnvRepo struct {
	vars map[string]string
	err  error
}

func (r *fakeEnvRepo) Get(projectID, name string) (string, bool, error) {
	if r.err != nil {
		return "", false, r.err
	}
	v, ok := r.vars[projectID+"|"+name]
	return v, ok, nil
}

func TestResolveAPIKey_EnvFallbackWhenNoRows(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "from-env")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	got, err := ResolveAPIKey(context.Background(), nil, "proj-x")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "from-env" {
		t.Errorf("got %q, want %q", got.Value, "from-env")
	}
}

func TestResolveAPIKey_NilRepoFallsThroughToEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-only")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	got, err := ResolveAPIKey(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "env-only" {
		t.Errorf("got %q, want %q", got.Value, "env-only")
	}
}

func TestResolveAPIKey_NoSourceErrors(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	envRepo := &fakeEnvRepo{vars: map[string]string{}}
	_, err := ResolveAPIKey(context.Background(), envRepo, "proj-x")
	if err == nil {
		t.Fatalf("expected error when no key resolves")
	}
	msg := err.Error()
	for _, sub := range []string{"per-project", "ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("error %q missing %q", msg, sub)
		}
	}
}
