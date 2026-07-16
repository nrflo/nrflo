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
	bashDefaultTimeout = 60 * time.Second
	bashMaxTimeout     = 300 * time.Second
	bashOutputCap      = 32 << 10
)

// bashHandler implements bash: one-shot `sh -c` in the session workdir. No
// persistent shell state across calls.
type bashHandler struct{}

func (bashHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "bash",
		Description: "Run a shell command (sh -c) in the working directory. One-shot: no state persists between calls. Combined stdout+stderr is returned (capped); non-zero exit is reported, not an error.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"command":{"type":"string","description":"The shell command to run"},
"timeout_sec":{"type":"integer","description":"Timeout in seconds (default 60, max 300)"}
},
"required":["command"],
"additionalProperties":false
}`),
	}
}

func (bashHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeout_sec"`
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

	timeout := bashDefaultTimeout
	if args.TimeoutSec > 0 {
		timeout = time.Duration(args.TimeoutSec) * time.Second
		if timeout > bashMaxTimeout {
			timeout = bashMaxTimeout
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if env.Heartbeat != nil {
		env.Heartbeat()
	}

	cmd := exec.CommandContext(runCtx, "sh", "-c", args.Command)
	cmd.Dir = env.WorkDir
	cmd.Env = bashEnv()
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
