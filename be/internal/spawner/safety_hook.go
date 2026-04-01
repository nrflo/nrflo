package spawner

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SafetyHookConfig defines the configuration for safety hook generation.
type SafetyHookConfig struct {
	Enabled           bool     `json:"enabled"`
	AllowGit          bool     `json:"allow_git"`
	RmRfAllowedPaths  []string `json:"rm_rf_allowed_paths"`
	DangerousPatterns []string `json:"dangerous_patterns"`
}

// BuildSafetySettingsJSON parses configJSON into SafetyHookConfig,
// returns the full --settings JSON string, or "" if disabled/empty/invalid.
func BuildSafetySettingsJSON(configJSON string) string {
	if configJSON == "" {
		return ""
	}

	var cfg SafetyHookConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return ""
	}
	if !cfg.Enabled {
		return ""
	}

	command := buildSafetyCommand(cfg)

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": command,
						},
					},
				},
			},
		},
	}

	out, err := json.Marshal(settings)
	if err != nil {
		return ""
	}
	return string(out)
}

// buildSafetyCommand generates the inline bash -c '...' command from rules.
func buildSafetyCommand(cfg SafetyHookConfig) string {
	var b strings.Builder

	b.WriteString("bash -c '")
	// Read stdin and extract .tool_input.command via jq
	b.WriteString(`CMD=$(cat | jq -r ".tool_input.command // empty"); `)
	b.WriteString(`if [ -z "$CMD" ]; then exit 0; fi; `)

	// Always-block hardcoded dangerous rm patterns
	b.WriteString(`case "$CMD" in `)
	for _, pattern := range []string{"rm -rf /", "rm -rf ~", "rm -rf .", "rm -rf .."} {
		b.WriteString(fmt.Sprintf(`*"%s"*) echo "Blocked: %s" >&2; exit 2;; `, pattern, pattern))
	}
	b.WriteString(`esac; `)

	// User-defined dangerous patterns
	if len(cfg.DangerousPatterns) > 0 {
		for _, pat := range cfg.DangerousPatterns {
			escaped := shellEscape(pat)
			b.WriteString(fmt.Sprintf(`case "$CMD" in *%s*) echo "Blocked dangerous pattern: %s" >&2; exit 2;; esac; `, escaped, escaped))
		}
	}

	// rm -rf check: extract target path, match against allowed paths
	b.WriteString(`if echo "$CMD" | grep -qE "rm\\s+(-[a-zA-Z]*r[a-zA-Z]*f|(-[a-zA-Z]*f[a-zA-Z]*r))"; then `)
	b.WriteString(`TARGET=$(echo "$CMD" | grep -oE "rm\\s+(-[a-zA-Z]*r[a-zA-Z]*f|(-[a-zA-Z]*f[a-zA-Z]*r))\\s+(.*)" | sed "s/rm\\s\\+[^ ]* //"); `)

	if len(cfg.RmRfAllowedPaths) > 0 {
		b.WriteString(`ALLOWED=0; `)
		for _, allowed := range cfg.RmRfAllowedPaths {
			escaped := shellEscape(allowed)
			if strings.HasPrefix(allowed, "/") {
				// Absolute path: prefix match
				b.WriteString(fmt.Sprintf(`case "$TARGET" in %s*) ALLOWED=1;; esac; `, escaped))
			} else {
				// Relative pattern: basename match
				b.WriteString(fmt.Sprintf(`BASENAME=$(basename "$TARGET"); case "$BASENAME" in %s) ALLOWED=1;; esac; `, escaped))
			}
		}
		b.WriteString(`if [ "$ALLOWED" -eq 0 ]; then echo "Blocked: rm -rf $TARGET not in allowed paths" >&2; exit 2; fi; `)
	} else {
		// No allowed paths — block all rm -rf
		b.WriteString(`echo "Blocked: rm -rf $TARGET not in allowed paths" >&2; exit 2; `)
	}
	b.WriteString(`fi; `)

	// Git check: if !allow_git, block git write ops
	if !cfg.AllowGit {
		gitWriteOps := []string{"commit", "push", "merge", "rebase", "reset", "branch -d", "branch -D", "clean", "stash drop", "add -f"}
		for _, op := range gitWriteOps {
			escaped := shellEscape(op)
			b.WriteString(fmt.Sprintf(`case "$CMD" in *"git %s"*) echo "Blocked: git %s" >&2; exit 2;; esac; `, escaped, escaped))
		}
	}

	// Default: exit 0
	b.WriteString(`exit 0`)
	b.WriteString("'")

	return b.String()
}

// shellEscape escapes single quotes for use inside a bash single-quoted string.
func shellEscape(s string) string {
	return strings.ReplaceAll(s, "'", "'\"'\"'")
}
