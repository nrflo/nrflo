package anthropic

import (
	"context"
	"fmt"
	"os"
	"strings"
)

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
)

// detectAuthMethod returns MethodOAuthBearer for OAuth tokens (sk-ant-oat01- prefix),
// MethodAPIKey otherwise.
func detectAuthMethod(value string) AuthMethod {
	if strings.HasPrefix(value, "sk-ant-oat01-") {
		return MethodOAuthBearer
	}
	return MethodAPIKey
}

// ResolveAPIKey returns the Anthropic credentials for the given project, applying
// the precedence: per-project env > server env.
//
// Returns a descriptive error when no source resolves so the caller can fail
// before issuing a request to the Anthropic API.
func ResolveAPIKey(_ context.Context, envRepo ProjectEnvVarRepo, projectID string) (Credentials, error) {
	tried := []string{}

	// 1. Per-project env vars.
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

	// 2. Server-process env fallback.
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
