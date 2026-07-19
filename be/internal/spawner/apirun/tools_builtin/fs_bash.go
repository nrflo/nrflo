package tools_builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const (
	bashDefaultTimeoutMS = 60000
	bashMaxTimeoutMS     = 600000 // 10 minutes
	bashOutputCap        = 32 << 10
)

// bashHandler implements bash: one-shot `sh -c` in the session workdir. No
// persistent shell state across calls, unless run_in_background is set, in
// which case the command is started detached and its shell_id is returned
// for bash_output/kill_shell to drive.
type bashHandler struct{}

func (bashHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "bash",
		Description: "Run a shell command (sh -c) in the working directory. Combined stdout+stderr is returned (capped); non-zero exit is reported, not an error. Set run_in_background=true to launch a long-running command (dev server, watcher, anything that does not exit on its own) and monitor it with bash_output / stop it with kill_shell instead of blocking; do not use run_in_background for quick, short-lived commands.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"command":{"type":"string","description":"The shell command to run"},
"timeout_ms":{"type":"integer","description":"Timeout in milliseconds (default 60000, max 600000)"},
"run_in_background":{"type":"boolean","description":"Run this command in the background and monitor it with bash_output/kill_shell instead of blocking"}
},
"required":["command"],
"additionalProperties":false
}`),
	}
}

func (bashHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Command         string `json:"command"`
		TimeoutMS       int    `json:"timeout_ms"`
		RunInBackground bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return "command is required", true, nil
	}
	if env.WorkDir == "" {
		return "no working directory configured for this session", true, nil
	}

	if env.SafetyCheck != nil {
		allowed, reason, err := env.SafetyCheck(args.Command)
		if err != nil {
			return err.Error(), true, nil
		}
		if !allowed {
			return reason, true, nil
		}
	}

	timeout := time.Duration(bashDefaultTimeoutMS) * time.Millisecond
	if args.TimeoutMS > 0 {
		ms := args.TimeoutMS
		if ms > bashMaxTimeoutMS {
			ms = bashMaxTimeoutMS
		}
		timeout = time.Duration(ms) * time.Millisecond
	}

	if env.Heartbeat != nil {
		env.Heartbeat()
	}

	if args.RunInBackground {
		id, err := startBackground(env, args.Command, timeout)
		if err != nil {
			return err.Error(), true, nil
		}
		return fmt.Sprintf("started background shell %s", id), false, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := buildBashCmd(runCtx, env, args.Command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()

	text := out.String()
	if len(text) > bashOutputCap {
		text = text[:bashOutputCap] + "\n… output truncated"
	}
	switch {
	case runCtx.Err() == context.DeadlineExceeded:
		return fmt.Sprintf("command timed out after %s\n%s", timeout, text), true, nil
	case runErr != nil:
		return fmt.Sprintf("exit error: %v\n%s", runErr, text), true, nil
	}
	if text == "" {
		text = "(no output)"
	}
	return text, false, nil
}

// buildBashCmd builds the jailed `sh -c command` invocation shared by the
// foreground run and startBackground.
func buildBashCmd(ctx context.Context, env apirun.ToolEnv, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = env.WorkDir
	cmd.Env = bashEnv()
	return cmd
}

// bashEnv is the server env minus nested-Claude markers (same rule as
// spawner.HostEnvWithoutClaudeMarkers, inlined to avoid an import cycle) and
// the agent bearer token — a shell command must not inherit credentials it
// did not need.
func bashEnv() []string {
	base := os.Environ()
	out := make([]string, 0, len(base))
	for _, kv := range base {
		if strings.HasPrefix(kv, "CLAUDECODE=") || strings.HasPrefix(kv, "CLAUDE_CODE_") ||
			strings.HasPrefix(kv, "NRFLO_AGENT_TOKEN=") || strings.HasPrefix(kv, "NRFLO_CONSOLE_TOKEN=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
