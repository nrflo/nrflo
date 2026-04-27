package apirun

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"be/internal/logger"
)

var literalSecretWarned sync.Once

// DereferenceSecretRef resolves a secret_ref value of the form
// "env:NAME", "file:/path", or "literal:VALUE". Shared by the Anthropic
// credentials path and the HTTP tool dispatcher's bearer_secret_ref.
func DereferenceSecretRef(ctx context.Context, ref string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "env:"):
		name := strings.TrimPrefix(ref, "env:")
		if name == "" {
			return "", fmt.Errorf("invalid secret_ref %q: env name empty", ref)
		}
		v := os.Getenv(name)
		if v == "" {
			return "", fmt.Errorf("env var %s referenced by secret_ref is empty", name)
		}
		return v, nil

	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimPrefix(ref, "file:")
		if path == "" {
			return "", fmt.Errorf("invalid secret_ref %q: file path empty", ref)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read secret_ref file %s: %w", path, err)
		}
		v := strings.TrimSpace(string(data))
		if v == "" {
			return "", fmt.Errorf("secret_ref file %s is empty", path)
		}
		return v, nil

	case strings.HasPrefix(ref, "literal:"):
		v := strings.TrimPrefix(ref, "literal:")
		if v == "" {
			return "", fmt.Errorf("invalid secret_ref %q: literal value empty", ref)
		}
		literalSecretWarned.Do(func() {
			logger.Warn(ctx, "literal secret_ref in use")
		})
		return v, nil
	}

	return "", fmt.Errorf("unsupported secret_ref scheme: %q", ref)
}
