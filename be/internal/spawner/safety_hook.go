package spawner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SafetyHookConfig defines the configuration for safety hook generation.
type SafetyHookConfig struct {
	Enabled           bool     `json:"enabled"`
	AllowGit          bool     `json:"allow_git"`
	RmRfAllowedPaths  []string `json:"rm_rf_allowed_paths"`
	DangerousPatterns []string `json:"dangerous_patterns"`
}

// CheckSafetyHook dry-runs a command against the safety hook built from cfg.
// Returns (allowed, reason, error). Exit 0 = allowed, exit 2 = blocked with reason on stderr.
func CheckSafetyHook(cfg SafetyHookConfig, command string) (bool, string, error) {
	if command == "" {
		return false, "", fmt.Errorf("command is required")
	}

	script := buildSafetyCommand(cfg)

	// Build mock tool-call JSON that the script expects on stdin.
	toolInput := map[string]interface{}{
		"tool_input": map[string]interface{}{
			"command": command,
		},
	}
	inputJSON, err := json.Marshal(toolInput)
	if err != nil {
		return false, "", fmt.Errorf("failed to marshal tool input: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		// Exit 0 — allowed.
		return true, "", nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 2 {
			reason := strings.TrimSpace(stderr.String())
			return false, reason, nil
		}
		return false, "", fmt.Errorf("script exited with code %d: %s", exitErr.ExitCode(), strings.TrimSpace(stderr.String()))
	}

	return false, "", fmt.Errorf("failed to execute safety hook: %w", err)
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

	// User-defined dangerous patterns (quoted to handle pipes and special chars)
	if len(cfg.DangerousPatterns) > 0 {
		for _, pat := range cfg.DangerousPatterns {
			escaped := shellEscape(pat)
			b.WriteString(fmt.Sprintf(`case "$CMD" in *"%s"*) echo "Blocked dangerous pattern: %s" >&2; exit 2;; esac; `, escaped, escaped))
		}
	}

	// rm -rf check: extract target path, match against allowed paths
	// Use [[:space:]] instead of \s for POSIX compatibility (BSD sed/grep on macOS)
	b.WriteString(`if echo "$CMD" | grep -qE "rm[[:space:]]+(-[a-zA-Z]*r[a-zA-Z]*f|(-[a-zA-Z]*f[a-zA-Z]*r))"; then `)
	b.WriteString(`TARGET=$(echo "$CMD" | sed -E "s/.*rm[[:space:]]+(-[a-zA-Z]*r[a-zA-Z]*f|(-[a-zA-Z]*f[a-zA-Z]*r))[[:space:]]+//"); `)

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
