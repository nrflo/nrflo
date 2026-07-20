package openai

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

func TestResolve_ServerEnvOpenAIAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "server-env-key")
	t.Setenv("CODEX_API_KEY", "")
	got, err := Resolve(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "server-env-key" {
		t.Errorf("Value = %q, want %q", got.Value, "server-env-key")
	}
}

func TestResolve_CodexAPIKeyFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "codex-fallback-key")
	got, err := Resolve(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "codex-fallback-key" {
		t.Errorf("Value = %q, want %q", got.Value, "codex-fallback-key")
	}
}

func TestResolve_PerProjectOpenAIKeyBeatsServerEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "server-key")
	t.Setenv("CODEX_API_KEY", "")
	repo := &fakeEnvRepo{vars: map[string]string{
		"proj-1|OPENAI_API_KEY": "proj-key",
	}}
	got, err := Resolve(context.Background(), repo, "proj-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "proj-key" {
		t.Errorf("Value = %q, want per-project key", got.Value)
	}
}

func TestResolve_PerProjectCodexKeyFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	repo := &fakeEnvRepo{vars: map[string]string{
		"proj-2|CODEX_API_KEY": "proj-codex-key",
	}}
	got, err := Resolve(context.Background(), repo, "proj-2")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "proj-codex-key" {
		t.Errorf("Value = %q, want per-project CODEX_API_KEY", got.Value)
	}
}

func TestResolve_OpenAIKeyBeatsCodexInProjectEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	repo := &fakeEnvRepo{vars: map[string]string{
		"p|OPENAI_API_KEY": "proj-oai",
		"p|CODEX_API_KEY":  "proj-codex",
	}}
	got, err := Resolve(context.Background(), repo, "p")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "proj-oai" {
		t.Errorf("Value = %q, want OPENAI_API_KEY to win over CODEX_API_KEY", got.Value)
	}
}

func TestResolve_NeitherSet_ErrorMentionsBothKeys(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	repo := &fakeEnvRepo{vars: map[string]string{}}
	_, err := Resolve(context.Background(), repo, "proj-x")
	if err == nil {
		t.Fatalf("expected error when no key found")
	}
	msg := err.Error()
	for _, want := range []string{"OPENAI_API_KEY", "CODEX_API_KEY"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing substring %q", msg, want)
		}
	}
}

func TestResolve_NilRepo_FallsToServerEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-only")
	t.Setenv("CODEX_API_KEY", "")
	got, err := Resolve(context.Background(), nil, "proj-x")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Value != "env-only" {
		t.Errorf("Value = %q, want %q", got.Value, "env-only")
	}
}

func TestResolve_RepoError_Propagates(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	repo := &fakeEnvRepo{err: errors.New("db unavailable")}
	_, err := Resolve(context.Background(), repo, "proj-1")
	if err == nil {
		t.Fatalf("expected repo error to propagate")
	}
	if !strings.Contains(err.Error(), "db unavailable") {
		t.Errorf("err = %v, want it to contain 'db unavailable'", err)
	}
}

// Base-URL resolution: per-project env wins over server env; independent of
// which source supplied the key; empty everywhere -> empty (SDK default).
func TestResolve_BaseURL_PerProjectBeatsServerEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "server-key")
	t.Setenv("OPENAI_BASE_URL", "https://server.example/v1")
	repo := &fakeEnvRepo{vars: map[string]string{
		"p1|OPENAI_BASE_URL": "https://openrouter.ai/api/v1",
	}}
	got, err := Resolve(context.Background(), repo, "p1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q, want per-project override", got.BaseURL)
	}
	if got.Value != "server-key" {
		t.Errorf("Value = %q — base URL must resolve independently of key source", got.Value)
	}
}

func TestResolve_BaseURL_ServerEnvFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "server-key")
	t.Setenv("OPENAI_BASE_URL", "https://server.example/v1")
	got, err := Resolve(context.Background(), &fakeEnvRepo{vars: map[string]string{}}, "p1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != "https://server.example/v1" {
		t.Errorf("BaseURL = %q, want server env fallback", got.BaseURL)
	}
}

func TestResolve_BaseURL_EmptyDefault(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "server-key")
	t.Setenv("OPENAI_BASE_URL", "")
	got, err := Resolve(context.Background(), &fakeEnvRepo{vars: map[string]string{
		"p1|OPENAI_API_KEY": "proj-key",
	}}, "p1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty (SDK default)", got.BaseURL)
	}
	if got.Value != "proj-key" {
		t.Errorf("Value = %q, want per-project key", got.Value)
	}
}
