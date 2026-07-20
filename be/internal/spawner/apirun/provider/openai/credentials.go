package openai

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// ProjectEnvVarRepo is a narrow interface for per-project env var lookup.
type ProjectEnvVarRepo interface {
	Get(projectID, name string) (string, bool, error)
}

// Credentials holds a resolved OpenAI API key and optional base-URL override.
type Credentials struct {
	Value string
	// BaseURL overrides the SDK endpoint (e.g. an OpenRouter proxy). Resolved
	// per-project first so one project can route to a different provider
	// without affecting the rest of the server. Empty = SDK default.
	BaseURL string
}

const (
	envOpenAIAPIKey  = "OPENAI_API_KEY"
	envCodexAPIKey   = "CODEX_API_KEY"
	envOpenAIBaseURL = "OPENAI_BASE_URL"
)

// Resolve returns the OpenAI API key and base-URL override for the given
// project, applying the precedence: per-project env (OPENAI_API_KEY, then
// CODEX_API_KEY) -> server env (same order). OPENAI_BASE_URL resolves with the
// same ladder, independently of which source supplied the key, so one project
// can point at an OpenAI-compatible proxy (e.g. OpenRouter) without rerouting
// the rest of the server. Returns a descriptive error listing all tried
// sources when no key resolves.
func Resolve(_ context.Context, envRepo ProjectEnvVarRepo, projectID string) (Credentials, error) {
	tried := []string{}
	baseURL := resolveBaseURL(envRepo, projectID)

	// 1. Per-project env vars.
	if projectID != "" && envRepo != nil {
		for _, name := range []string{envOpenAIAPIKey, envCodexAPIKey} {
			v, ok, err := envRepo.Get(projectID, name)
			if err != nil {
				return Credentials{}, fmt.Errorf("per-project env %s: %w", name, err)
			}
			if ok && v != "" {
				return Credentials{Value: v, BaseURL: baseURL}, nil
			}
		}
		tried = append(tried, "per-project env")
	}

	// 2. Server-process env fallback.
	if v := os.Getenv(envOpenAIAPIKey); v != "" {
		return Credentials{Value: v, BaseURL: baseURL}, nil
	}
	tried = append(tried, envOpenAIAPIKey+" env")
	if v := os.Getenv(envCodexAPIKey); v != "" {
		return Credentials{Value: v, BaseURL: baseURL}, nil
	}
	tried = append(tried, envCodexAPIKey+" env")

	return Credentials{}, fmt.Errorf("no OpenAI API key found (tried: %s)", strings.Join(tried, ", "))
}

// resolveBaseURL returns the endpoint override: per-project env -> server env -> "".
func resolveBaseURL(envRepo ProjectEnvVarRepo, projectID string) string {
	if projectID != "" && envRepo != nil {
		if v, ok, err := envRepo.Get(projectID, envOpenAIBaseURL); err == nil && ok && v != "" {
			return v
		}
	}
	return os.Getenv(envOpenAIBaseURL)
}
