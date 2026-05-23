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

// Credentials holds a resolved OpenAI API key.
type Credentials struct {
	Value string
}

const (
	envOpenAIAPIKey = "OPENAI_API_KEY"
	envCodexAPIKey  = "CODEX_API_KEY"
)

// Resolve returns the OpenAI API key for the given project, applying the
// precedence: per-project env (OPENAI_API_KEY, then CODEX_API_KEY) -> server
// env (same order). Returns a descriptive error listing all tried sources when
// none resolve.
func Resolve(_ context.Context, envRepo ProjectEnvVarRepo, projectID string) (Credentials, error) {
	tried := []string{}

	// 1. Per-project env vars.
	if projectID != "" && envRepo != nil {
		for _, name := range []string{envOpenAIAPIKey, envCodexAPIKey} {
			v, ok, err := envRepo.Get(projectID, name)
			if err != nil {
				return Credentials{}, fmt.Errorf("per-project env %s: %w", name, err)
			}
			if ok && v != "" {
				return Credentials{Value: v}, nil
			}
		}
		tried = append(tried, "per-project env")
	}

	// 2. Server-process env fallback.
	if v := os.Getenv(envOpenAIAPIKey); v != "" {
		return Credentials{Value: v}, nil
	}
	tried = append(tried, envOpenAIAPIKey+" env")
	if v := os.Getenv(envCodexAPIKey); v != "" {
		return Credentials{Value: v}, nil
	}
	tried = append(tried, envCodexAPIKey+" env")

	return Credentials{}, fmt.Errorf("no OpenAI API key found (tried: %s)", strings.Join(tried, ", "))
}
