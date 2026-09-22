package spawner

import (
	"os"
	"strings"
)

// HostEnvWithoutClaudeMarkers removes nested-Claude markers while preserving
// authentication and disabling first-run onboarding for spawned sessions.
func HostEnvWithoutClaudeMarkers() []string {
	hostEnv := os.Environ()
	out := make([]string, 0, len(hostEnv)+1)
	for _, entry := range hostEnv {
		if strings.HasPrefix(entry, "CLAUDE_CODE_OAUTH_TOKEN=") {
			out = append(out, entry)
			continue
		}
		if strings.HasPrefix(entry, "CLAUDECODE=") || strings.HasPrefix(entry, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, entry)
	}
	return append(out, "CLAUDE_CODE_SKIP_ONBOARDING=1")
}
