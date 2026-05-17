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
// package depends on. The runner wires the concrete repo to an adapter
// that satisfies this interface; tests fake it directly.
type APICredentialRepo interface {
	Resolve(provider, projectID string) (*model.APICredential, error)
}

// ProjectEnvVarRepo is a narrow interface for per-project env var lookup.
// Implementations load vars once and cache; the projectID parameter is
// informational (adapters may be pre-scoped to a single project).
type ProjectEnvVarRepo interface {
	Get(projectID, name string) (string, bool, error)
}

// AuthMethod identifies the authentication scheme for a resolved credential.
type AuthMethod string

const (
	MethodAPIKey      AuthMethod = "api_key"
	MethodOAuthBearer AuthMethod = "oauth_bearer"
)

// Credentials holds a resolved API credential and the auth method to use.
type Credentials struct {
	Value  string
	Method AuthMethod
}

const (
	envANTHROPICAPIKey     = "ANTHROPIC_API_KEY"
	envANTHROPICOAuthToken = "ANTHROPIC_OAUTH_TOKEN"
	providerAnthropic      = "anthropic"
)

var literalWarned sync.Once

// detectAuthMethod returns MethodOAuthBearer for OAuth tokens (sk-ant-oat01- prefix),
// MethodAPIKey otherwise.
func detectAuthMethod(value string) AuthMethod {
	if strings.HasPrefix(value, "sk-ant-oat01-") {
		return MethodOAuthBearer
	}
	return MethodAPIKey
}

// ResolveAPIKey returns the Anthropic credentials for the given project, applying
// the precedence: per-project DB row > per-project env > global DB row > server env.
//
// secret_ref values are dereferenced before being returned:
//   - "env:NAME"  -> os.Getenv(NAME); errors if empty.
//   - "file:/path" -> file contents (whitespace trimmed); errors if missing/empty.
//   - "literal:VALUE" -> VALUE; logs a one-time warning per process.
//
// Returns a descriptive error when no source resolves so the caller can fail
// before issuing a request to the Anthropic API.
func ResolveAPIKey(ctx context.Context, credRepo APICredentialRepo, envRepo ProjectEnvVarRepo, projectID string) (Credentials, error) {
	tried := []string{}

	// 1. Per-project api_credentials row.
	if projectID != "" && credRepo != nil {
		creds, ok, err := resolveFromRepo(ctx, credRepo, projectID)
		if err != nil {
			return Credentials{}, err
		}
		if ok {
			return creds, nil
		}
		tried = append(tried, "per-project credential")
	}

	// 2. Per-project env vars.
	if projectID != "" && envRepo != nil {
		for _, name := range []string{envANTHROPICAPIKey, envANTHROPICOAuthToken} {
			v, ok, err := envRepo.Get(projectID, name)
			if err != nil {
				return Credentials{}, fmt.Errorf("per-project env %s: %w", name, err)
			}
			if ok && v != "" {
				return Credentials{Value: v, Method: detectAuthMethod(v)}, nil
			}
		}
		tried = append(tried, "per-project env")
	}

	// 3. Global (project_id IS NULL) api_credentials row.
	if credRepo != nil {
		creds, ok, err := resolveFromRepo(ctx, credRepo, "")
		if err != nil {
			return Credentials{}, err
		}
		if ok {
			return creds, nil
		}
		tried = append(tried, "global credential")
	}

	// 4. Server-process env fallback.
	if v := os.Getenv(envANTHROPICAPIKey); v != "" {
		return Credentials{Value: v, Method: MethodAPIKey}, nil
	}
	tried = append(tried, envANTHROPICAPIKey+" env")
	if v := os.Getenv(envANTHROPICOAuthToken); v != "" {
		return Credentials{Value: v, Method: MethodOAuthBearer}, nil
	}
	tried = append(tried, envANTHROPICOAuthToken+" env")

	return Credentials{}, fmt.Errorf("no anthropic API key found (tried: %s)", strings.Join(tried, ", "))
}

func resolveFromRepo(ctx context.Context, repo APICredentialRepo, projectID string) (Credentials, bool, error) {
	cred, err := repo.Resolve(providerAnthropic, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Credentials{}, false, nil
		}
		return Credentials{}, false, fmt.Errorf("resolve anthropic credential: %w", err)
	}
	if cred == nil {
		return Credentials{}, false, nil
	}
	key, err := dereferenceSecretRef(ctx, cred.SecretRef)
	if err != nil {
		return Credentials{}, false, err
	}
	return Credentials{Value: key, Method: detectAuthMethod(key)}, true, nil
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
