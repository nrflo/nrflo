package openrouter

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeEnvRepo is an in-memory ProjectEnvVarRepo. Keys are "projectID|name".
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

func TestResolve_ServerEnvKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "server-env-key")
	got, err := Resolve(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "server-env-key" {
		t.Errorf("Value = %q, want %q", got.Value, "server-env-key")
	}
}

func TestResolve_PerProjectKeyBeatsServerEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "server-key")
	repo := &fakeEnvRepo{vars: map[string]string{
		"proj-1|OPENROUTER_API_KEY": "proj-key",
	}}
	got, err := Resolve(context.Background(), repo, "proj-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "proj-key" {
		t.Errorf("Value = %q, want per-project key", got.Value)
	}
}

// TestResolve_NoCodexFallback verifies the openrouter ladder does not
// consult CODEX_API_KEY/OPENAI_API_KEY the way the openai provider does —
// only OPENROUTER_API_KEY is ever consulted.
func TestResolve_NoCodexFallback(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "should-not-be-used")
	t.Setenv("CODEX_API_KEY", "should-not-be-used")
	repo := &fakeEnvRepo{vars: map[string]string{}}
	_, err := Resolve(context.Background(), repo, "proj-x")
	if err == nil {
		t.Fatal("expected error: no OPENROUTER_API_KEY set, cross-provider keys must not be consulted")
	}
}

func TestResolve_NeitherSet_ErrorMentionsKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	repo := &fakeEnvRepo{vars: map[string]string{}}
	_, err := Resolve(context.Background(), repo, "proj-x")
	if err == nil {
		t.Fatalf("expected error when no key found")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Errorf("error %q missing substring OPENROUTER_API_KEY", err.Error())
	}
}

func TestResolve_NilRepo_FallsToServerEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "env-only")
	got, err := Resolve(context.Background(), nil, "proj-x")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "env-only" {
		t.Errorf("Value = %q, want %q", got.Value, "env-only")
	}
}

func TestResolve_RepoError_Propagates(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	repo := &fakeEnvRepo{err: errors.New("db unavailable")}
	_, err := Resolve(context.Background(), repo, "proj-1")
	if err == nil {
		t.Fatalf("expected repo error to propagate")
	}
	if !strings.Contains(err.Error(), "db unavailable") {
		t.Errorf("err = %v, want it to contain 'db unavailable'", err)
	}
}

// TestResolve_BaseURL_DefaultConstant verifies BaseURL defaults to
// DefaultBaseURL (never empty), unlike openai where empty means SDK default.
func TestResolve_BaseURL_DefaultConstant(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "server-key")
	t.Setenv("OPENROUTER_BASE_URL", "")
	got, err := Resolve(context.Background(), &fakeEnvRepo{vars: map[string]string{}}, "p1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want default constant %q", got.BaseURL, DefaultBaseURL)
	}
}

func TestResolve_BaseURL_PerProjectBeatsServerEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "server-key")
	t.Setenv("OPENROUTER_BASE_URL", "https://server.example/v1")
	repo := &fakeEnvRepo{vars: map[string]string{
		"p1|OPENROUTER_BASE_URL": "https://project-proxy.example/v1",
	}}
	got, err := Resolve(context.Background(), repo, "p1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != "https://project-proxy.example/v1" {
		t.Errorf("BaseURL = %q, want per-project override", got.BaseURL)
	}
	if got.Value != "server-key" {
		t.Errorf("Value = %q — base URL must resolve independently of key source", got.Value)
	}
}

func TestResolve_BaseURL_ServerEnvFallback(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "server-key")
	t.Setenv("OPENROUTER_BASE_URL", "https://server.example/v1")
	got, err := Resolve(context.Background(), &fakeEnvRepo{vars: map[string]string{}}, "p1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != "https://server.example/v1" {
		t.Errorf("BaseURL = %q, want server env fallback", got.BaseURL)
	}
}
