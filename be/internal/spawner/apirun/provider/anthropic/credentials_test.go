package anthropic

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"be/internal/logger"
	"be/internal/model"
)

// fakeRepo is an in-memory APICredentialRepo. Keys are "provider|projectID";
// missing rows return sql.ErrNoRows. err lets a test inject a non-NotFound
// error to verify it propagates.
type fakeRepo struct {
	rows map[string]*model.APICredential
	err  error
}

func (f *fakeRepo) Resolve(provider, projectID string) (*model.APICredential, error) {
	if f.err != nil {
		return nil, f.err
	}
	c, ok := f.rows[provider+"|"+projectID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return c, nil
}

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

// captureLogger redirects logger output to a buffer for the test, restoring
// the original writer in t.Cleanup.
func captureLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	orig := logger.GetWriter()
	buf := &bytes.Buffer{}
	logger.SetWriter(buf)
	t.Cleanup(func() { logger.SetWriter(orig) })
	return buf
}

// resetLiteralWarned resets the package-global sync.Once so each test that
// asserts the literal-key warning starts from a clean slate. Test-only helper.
func resetLiteralWarned(t *testing.T) {
	t.Helper()
	literalWarned = sync.Once{}
}

func TestResolveAPIKey_PerProjectBeatsGlobal(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "from-env")
	t.Setenv("ANTHROPIC_PROJ_KEY", "proj-key")
	t.Setenv("ANTHROPIC_GLOBAL_KEY", "global-key")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|proj-1": {SecretRef: "env:ANTHROPIC_PROJ_KEY"},
		"anthropic|":       {SecretRef: "env:ANTHROPIC_GLOBAL_KEY"},
	}}
	got, err := ResolveAPIKey(context.Background(), repo, nil, "proj-1")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "proj-key" {
		t.Errorf("got %q, want %q (per-project must win)", got.Value, "proj-key")
	}
}

func TestResolveAPIKey_GlobalBeatsEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "from-env")
	t.Setenv("ANTHROPIC_GLOBAL_KEY", "global-key")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "env:ANTHROPIC_GLOBAL_KEY"},
	}}
	got, err := ResolveAPIKey(context.Background(), repo, nil, "proj-x")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "global-key" {
		t.Errorf("got %q, want %q (global must beat env)", got.Value, "global-key")
	}
}

func TestResolveAPIKey_EnvFallbackWhenNoRows(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "from-env")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	repo := &fakeRepo{rows: map[string]*model.APICredential{}}
	got, err := ResolveAPIKey(context.Background(), repo, nil, "proj-x")
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
	got, err := ResolveAPIKey(context.Background(), nil, nil, "")
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
	repo := &fakeRepo{rows: map[string]*model.APICredential{}}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "proj-x")
	if err == nil {
		t.Fatalf("expected error when no key resolves")
	}
	msg := err.Error()
	for _, sub := range []string{"per-project", "global", "ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("error %q missing %q", msg, sub)
		}
	}
}

func TestResolveAPIKey_RepoErrorPropagates(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "from-env")
	repo := &fakeRepo{err: errors.New("db boom")}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "proj-x")
	if err == nil {
		t.Fatalf("expected DB error to propagate")
	}
	if !strings.Contains(err.Error(), "db boom") {
		t.Errorf("err = %v, want it to wrap 'db boom'", err)
	}
}

func TestResolveAPIKey_EmptyProjectIDSkipsPerProject(t *testing.T) {
	// With projectID="", the per-project lookup must be skipped — otherwise
	// it would match the global row (provider|"") by accident and the
	// 'tried' message would lie.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	repo := &fakeRepo{rows: map[string]*model.APICredential{}}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "per-project") {
		t.Errorf("err = %q, must NOT mention per-project when projectID is empty", msg)
	}
	if !strings.Contains(msg, "global credential") {
		t.Errorf("err = %q, want it to mention global", msg)
	}
}
