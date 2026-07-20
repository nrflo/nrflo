// Package openrouter provides API-key credential resolution for OpenRouter,
// consumed by the thin openai-wrapper provider in this package.
package openrouter

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

// Credentials holds a resolved OpenRouter API key and base-URL.
type Credentials struct {
	Value   string
	BaseURL string
}

// DefaultBaseURL is used when no OPENROUTER_BASE_URL override is configured.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

const (
	envOpenRouterAPIKey  = "OPENROUTER_API_KEY"
	envOpenRouterBaseURL = "OPENROUTER_BASE_URL"
)

// Resolve returns the OpenRouter API key and base URL for the given project,
// applying the precedence: per-project env (OPENROUTER_API_KEY) -> server env
// (same var). OPENROUTER_BASE_URL resolves independently with the same
// ladder, defaulting to DefaultBaseURL (never empty) when unset. Returns a
// descriptive error listing all tried sources when no key resolves.
func Resolve(_ context.Context, envRepo ProjectEnvVarRepo, projectID string) (Credentials, error) {
	tried := []string{}
	baseURL := resolveBaseURL(envRepo, projectID)

	if projectID != "" && envRepo != nil {
		v, ok, err := envRepo.Get(projectID, envOpenRouterAPIKey)
		if err != nil {
			return Credentials{}, fmt.Errorf("per-project env %s: %w", envOpenRouterAPIKey, err)
		}
		if ok && v != "" {
			return Credentials{Value: v, BaseURL: baseURL}, nil
		}
		tried = append(tried, "per-project env")
	}

	if v := os.Getenv(envOpenRouterAPIKey); v != "" {
		return Credentials{Value: v, BaseURL: baseURL}, nil
	}
	tried = append(tried, envOpenRouterAPIKey+" env")

	return Credentials{}, fmt.Errorf("no OpenRouter API key found (tried: %s)", strings.Join(tried, ", "))
}

// resolveBaseURL returns the endpoint override: per-project env -> server env
// -> DefaultBaseURL (never empty).
func resolveBaseURL(envRepo ProjectEnvVarRepo, projectID string) string {
	if projectID != "" && envRepo != nil {
		if v, ok, err := envRepo.Get(projectID, envOpenRouterBaseURL); err == nil && ok && v != "" {
			return v
		}
	}
	if v := os.Getenv(envOpenRouterBaseURL); v != "" {
		return v
	}
	return DefaultBaseURL
}
