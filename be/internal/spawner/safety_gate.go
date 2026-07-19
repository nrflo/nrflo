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

const safetyScriptTimeout = 5 * time.Second

// safetyCheckToolName is the fixed tool_name field of the shared Claude Code
// PreToolUse stdin contract — the bridge gates only bash, so it is always
// "Bash" (also used by CheckSafetyHook).
const safetyCheckToolName = "Bash"

// resolveSafetyCheck returns the apirun.ToolEnv.SafetyCheck closure for a
// project's bash tool: project tool_safety_script > global tool_safety_script
// > the project's claude_safety_hook config (dry-run via CheckSafetyHook) >
// allow. This is a script check only, NOT a permission system — there is no
// interactive approval step. Config is read fresh on every call (not cached
// at spawn time) so an admin change takes effect on the next bash invocation.
// A nil pool (tests without a DB) always allows.
func (s *Spawner) resolveSafetyCheck(projectID string) func(command string) (bool, string, error) {
	return func(command string) (bool, string, error) {
		pool := s.pool()
		if pool == nil {
			return true, "", nil
		}
		if script, _ := pool.GetProjectConfig(projectID, "tool_safety_script"); script != "" {
			return runSafetyScript(script, command)
		}
		if script, _ := pool.GetConfig("tool_safety_script"); script != "" {
			return runSafetyScript(script, command)
		}
		if raw, _ := pool.GetProjectConfig(projectID, "claude_safety_hook"); raw != "" {
			var cfg SafetyHookConfig
			if err := json.Unmarshal([]byte(raw), &cfg); err == nil && cfg.Enabled {
				return CheckSafetyHook(cfg, command)
			}
		}
		return true, "", nil
	}
}

// runSafetyScript execs the external safety-check script with the shared
// Claude Code PreToolUse stdin contract ({"tool_name":"Bash","tool_input":
// {"command"}}, identical to CheckSafetyHook's) — exit 0 = allow, exit 2 = block with the
// reason on stderr; any other exit or exec failure is a Go error (the bash
// handler surfaces it as a blocking isError tool result, never turn-fatal).
// 5s timeout.
func runSafetyScript(scriptPath, command string) (bool, string, error) {
	stdin, err := json.Marshal(map[string]interface{}{
		"tool_name": safetyCheckToolName,
		"tool_input": map[string]interface{}{
			"command": command,
		},
	})
	if err != nil {
		return false, "", fmt.Errorf("marshal safety check input: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), safetyScriptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, scriptPath)
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr == nil {
		return true, "", nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return false, "", fmt.Errorf("safety check script timed out after %s", safetyScriptTimeout)
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 2 {
			return false, strings.TrimSpace(stderr.String()), nil
		}
		return false, "", fmt.Errorf("safety check script exited with code %d: %s", exitErr.ExitCode(), strings.TrimSpace(stderr.String()))
	}
	return false, "", fmt.Errorf("failed to execute safety check script: %w", runErr)
}
