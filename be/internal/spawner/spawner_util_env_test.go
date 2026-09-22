package spawner

import "testing"

func TestHostEnvWithoutClaudeMarkers_PreservesOAuthAndSkipsOnboarding(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-test-token")
	t.Setenv("CLAUDE_CODE_SKIP_ONBOARDING", "0")
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "child")
	t.Setenv("CLAUDECODE", "parent")

	env := HostEnvWithoutClaudeMarkers()

	assertEnvValue(t, env, "CLAUDE_CODE_OAUTH_TOKEN", "oauth-test-token")
	assertEnvValue(t, env, "CLAUDE_CODE_SKIP_ONBOARDING", "1")
	assertEnvMissing(t, env, "CLAUDE_CODE_CHILD_SESSION")
	assertEnvMissing(t, env, "CLAUDECODE")
}

func assertEnvValue(t *testing.T, env []string, key, want string) {
	t.Helper()
	prefix := key + "="
	found := ""
	count := 0
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			found = entry[len(prefix):]
			count++
		}
	}
	if count != 1 || found != want {
		t.Fatalf("%s entries = %d, value = %q; want one entry with value %q", key, count, found, want)
	}
}

func assertEnvMissing(t *testing.T, env []string, key string) {
	t.Helper()
	prefix := key + "="
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			t.Fatalf("environment unexpectedly contains %s", key)
		}
	}
}
