package anthropic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"be/internal/logger"
	"be/internal/model"
)

// APICredentialRepo is the minimal subset of repo.APICredentialRepo this
// package depends on. The runner (T3) wires the concrete repo to an adapter
// that satisfies this interface; tests fake it directly.
type APICredentialRepo interface {
	Resolve(provider, projectID string) (*model.APICredential, error)
}

const (
	envANTHROPICAPIKey = "ANTHROPIC_API_KEY"
	providerAnthropic  = "anthropic"
)

var literalWarned sync.Once

// ResolveAPIKey returns the Anthropic API key for the given project, applying
// the precedence: per-project DB row > global DB row > ANTHROPIC_API_KEY env.
//
// secret_ref values are dereferenced before being returned:
//   - "env:NAME"  -> os.Getenv(NAME); errors if empty.
//   - "file:/path" -> file contents (whitespace trimmed); errors if missing/empty.
//   - "literal:VALUE" -> VALUE; logs a one-time warning per process.
//
// Returns a descriptive error when no source resolves so the caller can fail
// before issuing a request to the Anthropic API.
//
// ctx is reserved for future cancellable lookups; the current implementation
// does only synchronous reads.
func ResolveAPIKey(ctx context.Context, repo APICredentialRepo, projectID string) (string, error) {
	tried := []string{}

	// 1. Per-project row.
	if projectID != "" && repo != nil {
		key, ok, err := resolveFromRepo(ctx, repo, projectID)
		if err != nil {
			return "", err
		}
		if ok {
			return key, nil
		}
		tried = append(tried, "per-project credential")
	}

	// 2. Global (project_id IS NULL) row.
	if repo != nil {
		key, ok, err := resolveFromRepo(ctx, repo, "")
		if err != nil {
			return "", err
		}
		if ok {
			return key, nil
		}
		tried = append(tried, "global credential")
	}

	// 3. ANTHROPIC_API_KEY env fallback.
	if v := os.Getenv(envANTHROPICAPIKey); v != "" {
		return v, nil
	}
	tried = append(tried, envANTHROPICAPIKey+" env")

	return "", fmt.Errorf("no anthropic API key found (tried: %s)", strings.Join(tried, ", "))
}

func resolveFromRepo(ctx context.Context, repo APICredentialRepo, projectID string) (string, bool, error) {
	cred, err := repo.Resolve(providerAnthropic, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("resolve anthropic credential: %w", err)
	}
	if cred == nil {
		return "", false, nil
	}
	key, err := dereferenceSecretRef(ctx, cred.SecretRef)
	if err != nil {
		return "", false, err
	}
	return key, true, nil
}

func dereferenceSecretRef(ctx context.Context, ref string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "env:"):
		name := strings.TrimPrefix(ref, "env:")
		if name == "" {
			return "", fmt.Errorf("invalid secret_ref %q: env name empty", ref)
		}
		v := os.Getenv(name)
		if v == "" {
			return "", fmt.Errorf("env var %s referenced by api_credentials is empty", name)
		}
		return v, nil

	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimPrefix(ref, "file:")
		if path == "" {
			return "", fmt.Errorf("invalid secret_ref %q: file path empty", ref)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read api_credentials file %s: %w", path, err)
		}
		v := strings.TrimSpace(string(data))
		if v == "" {
			return "", fmt.Errorf("api_credentials file %s is empty", path)
		}
		return v, nil

	case strings.HasPrefix(ref, "literal:"):
		v := strings.TrimPrefix(ref, "literal:")
		if v == "" {
			return "", fmt.Errorf("invalid secret_ref %q: literal value empty", ref)
		}
		literalWarned.Do(func() {
			logger.Warn(ctx, "literal API key in api_credentials")
		})
		return v, nil
	}

	return "", fmt.Errorf("unsupported secret_ref scheme: %q", ref)
}
