package anthropic

import (
	"context"
	"errors"
	"strings"
	"testing"

	"be/internal/model"
)

func TestDetectAuthMethod(t *testing.T) {
	tests := []struct {
		value  string
		method AuthMethod
	}{
		{"sk-ant-oat01-abc", MethodOAuthBearer},
		{"sk-ant-oat01-", MethodOAuthBearer},
		{"sk-ant-api03-abc", MethodAPIKey},
		{"anything-else", MethodAPIKey},
		{"", MethodAPIKey},
		{"sk-ant-oat01", MethodAPIKey}, // no trailing dash — not the prefix
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got := detectAuthMethod(tt.value)
			if got != tt.method {
				t.Errorf("detectAuthMethod(%q) = %q, want %q", tt.value, got, tt.method)
			}
		})
	}
}

// TestResolveAPIKey_ProjectEnvOAuthToken verifies that an ANTHROPIC_OAUTH_TOKEN
// in per-project env resolves to MethodOAuthBearer.
func TestResolveAPIKey_ProjectEnvOAuthToken(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	envRepo := &fakeEnvRepo{vars: map[string]string{
		"proj-1|ANTHROPIC_OAUTH_TOKEN": "sk-ant-oat01-mytoken",
	}}
	got, err := ResolveAPIKey(context.Background(), nil, envRepo, "proj-1")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "sk-ant-oat01-mytoken" {
		t.Errorf("Value = %q, want %q", got.Value, "sk-ant-oat01-mytoken")
	}
	if got.Method != MethodOAuthBearer {
		t.Errorf("Method = %q, want %q", got.Method, MethodOAuthBearer)
	}
}

// TestResolveAPIKey_ProjectEnvAPIKeyBeatsOAuth verifies that ANTHROPIC_API_KEY
// is checked before ANTHROPIC_OAUTH_TOKEN in per-project env.
func TestResolveAPIKey_ProjectEnvAPIKeyBeatsOAuth(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	envRepo := &fakeEnvRepo{vars: map[string]string{
		"proj-1|ANTHROPIC_API_KEY":     "sk-ant-api03-proj",
		"proj-1|ANTHROPIC_OAUTH_TOKEN": "sk-ant-oat01-proj",
	}}
	got, err := ResolveAPIKey(context.Background(), nil, envRepo, "proj-1")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "sk-ant-api03-proj" {
		t.Errorf("Value = %q, want ANTHROPIC_API_KEY wins", got.Value)
	}
	if got.Method != MethodAPIKey {
		t.Errorf("Method = %q, want %q", got.Method, MethodAPIKey)
	}
}

// TestResolveAPIKey_DBRowBeatsProjectEnv verifies that a per-project DB row
// wins over per-project env vars.
func TestResolveAPIKey_DBRowBeatsProjectEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	t.Setenv("DB_PROJ_KEY", "sk-from-db")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|proj-1": {SecretRef: "env:DB_PROJ_KEY"},
	}}
	envRepo := &fakeEnvRepo{vars: map[string]string{
		"proj-1|ANTHROPIC_API_KEY": "sk-from-env",
	}}
	got, err := ResolveAPIKey(context.Background(), repo, envRepo, "proj-1")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "sk-from-db" {
		t.Errorf("Value = %q, want DB row to win over env", got.Value)
	}
}

// TestResolveAPIKey_ProjectEnvBeatsGlobalDB verifies that a per-project env var
// beats a global DB row.
func TestResolveAPIKey_ProjectEnvBeatsGlobalDB(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	t.Setenv("GLOBAL_KEY", "global-db-key")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "env:GLOBAL_KEY"},
	}}
	envRepo := &fakeEnvRepo{vars: map[string]string{
		"proj-1|ANTHROPIC_API_KEY": "sk-from-proj-env",
	}}
	got, err := ResolveAPIKey(context.Background(), repo, envRepo, "proj-1")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "sk-from-proj-env" {
		t.Errorf("Value = %q, want per-project env to beat global DB row", got.Value)
	}
}

// TestResolveAPIKey_EnvRepoError verifies that an error from ProjectEnvVarRepo
// is propagated rather than silently skipped.
func TestResolveAPIKey_EnvRepoError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	envRepo := &fakeEnvRepo{err: errors.New("env lookup failed")}
	_, err := ResolveAPIKey(context.Background(), nil, envRepo, "proj-1")
	if err == nil {
		t.Fatalf("expected env error to propagate")
	}
	if !strings.Contains(err.Error(), "env lookup failed") {
		t.Errorf("err = %v, want it to wrap 'env lookup failed'", err)
	}
}

// TestResolveAPIKey_ServerEnvOAuthToken verifies that ANTHROPIC_OAUTH_TOKEN in
// the server process env resolves with MethodOAuthBearer (step 4 fallback).
func TestResolveAPIKey_ServerEnvOAuthToken(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "sk-ant-oat01-server")
	got, err := ResolveAPIKey(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "sk-ant-oat01-server" {
		t.Errorf("Value = %q, want server ANTHROPIC_OAUTH_TOKEN", got.Value)
	}
	if got.Method != MethodOAuthBearer {
		t.Errorf("Method = %q, want %q", got.Method, MethodOAuthBearer)
	}
}

// TestResolveAPIKey_ServerEnvAPIKeyBeatsOAuthToken verifies that
// ANTHROPIC_API_KEY takes precedence over ANTHROPIC_OAUTH_TOKEN at step 4.
func TestResolveAPIKey_ServerEnvAPIKeyBeatsOAuthToken(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-server")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "sk-ant-oat01-server")
	got, err := ResolveAPIKey(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "sk-ant-api03-server" {
		t.Errorf("Value = %q, want ANTHROPIC_API_KEY to win over ANTHROPIC_OAUTH_TOKEN", got.Value)
	}
	if got.Method != MethodAPIKey {
		t.Errorf("Method = %q, want %q", got.Method, MethodAPIKey)
	}
}
