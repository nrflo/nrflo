package spawner

import (
	"os"
	"strings"
)

// HostEnvWithoutClaudeMarkers removes nested-Claude markers while preserving
// authentication and the Docker sandbox marker.
func HostEnvWithoutClaudeMarkers() []string {
	hostEnv := os.Environ()
	out := make([]string, 0, len(hostEnv))
	for _, entry := range hostEnv {
		if strings.HasPrefix(entry, "CLAUDE_CODE_OAUTH_TOKEN=") ||
			strings.HasPrefix(entry, "CLAUDE_CODE_SANDBOXED=") {
			out = append(out, entry)
			continue
		}
		if strings.HasPrefix(entry, "CLAUDECODE=") || strings.HasPrefix(entry, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, entry)
	}
	return out
}
