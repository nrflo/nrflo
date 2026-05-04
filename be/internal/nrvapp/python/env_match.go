package python

import (
	"os"
	"path/filepath"
	"strings"
)

// MatchEnv returns a filtered subset of os.Environ() whose key matches any
// pattern in allowPatterns. Patterns use filepath.Match glob syntax.
// KEY=VALUE pairs where KEY doesn't match are excluded.
func MatchEnv(allowPatterns []string, environ []string) []string {
	if len(allowPatterns) == 0 {
		return nil
	}
	var result []string
	for _, kv := range environ {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if matchesAny(key, allowPatterns) {
			result = append(result, kv)
		}
	}
	return result
}

// FilterOSEnv calls MatchEnv against the current process environment.
func FilterOSEnv(allowPatterns []string) []string {
	return MatchEnv(allowPatterns, os.Environ())
}

func matchesAny(key string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, err := filepath.Match(pattern, key); err == nil && matched {
			return true
		}
		// Also support simple prefix match (e.g. "MY_APP_*" covers glob; this
		// handles plain prefixes like "PATH" as an exact match already via glob).
	}
	return false
}
