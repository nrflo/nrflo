package artifact

import (
	"fmt"
	"os"
	"strings"
)

// ResolveSecretRef resolves a secret reference of the form "env:NAME",
// "literal:VALUE", or "file:/path" to its plaintext value.
func ResolveSecretRef(ref string) (string, error) {
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

	case strings.HasPrefix(ref, "literal:"):
		v := strings.TrimPrefix(ref, "literal:")
		if v == "" {
			return "", fmt.Errorf("invalid secret_ref %q: literal value empty", ref)
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
	}

	return "", fmt.Errorf("unsupported secret_ref scheme: %q", ref)
}

// RedactSecretRef returns a redacted form safe for logging. Literal values are
// replaced by "literal:***"; env: and file: references are returned as-is.
func RedactSecretRef(ref string) string {
	if strings.HasPrefix(ref, "literal:") {
		return "literal:***"
	}
	return ref
}
