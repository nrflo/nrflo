package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// startBackground launches command as a jailed background shell (same sh -c
// shape as the foreground bash handler), registers it on env.FS, and returns
// its shell_id. Requires env.FS != nil and env.WorkDir != "".
func startBackground(env apirun.ToolEnv, command string, timeout time.Duration) (string, error) {
	if env.FS == nil {
		return "", fmt.Errorf("background shells are not available for this session")
	}
	if env.WorkDir == "" {
		return "", fmt.Errorf("no working directory configured for this session")
	}

	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := buildBashCmd(runCtx, env, command)

	startedAt := time.Now()
	if env.Clock != nil {
		startedAt = env.Clock.Now()
	}
	id := env.FS.NewShellID()
	sh := apirun.NewBgShell(id, command, startedAt, cancel)
	cmd.Stdout = sh
	cmd.Stderr = sh

	if err := cmd.Start(); err != nil {
		cancel()
		return "", err
	}
	sh.Track(cmd)
	env.FS.AddShell(sh)
	return id, nil
}

// bashOutputHandler implements bash_output: poll a background shell started
// via bash's run_in_background for new output plus its status.
type bashOutputHandler struct{}

func (bashOutputHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "bash_output",
		Description: "Retrieve output from a running or completed background shell started with bash's run_in_background=true. Returns only new output since the last check, plus status (running/completed/failed) and exit code. Use this to poll a background command instead of blocking on it.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"shell_id":{"type":"string","description":"The background shell ID returned by bash when run_in_background was true"},
"filter":{"type":"string","description":"Optional regex; only output lines matching it are returned"}
},
"required":["shell_id"],
"additionalProperties":false
}`),
	}
}

func (bashOutputHandler) Invoke(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		ShellID string `json:"shell_id"`
		Filter  string `json:"filter"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.FS == nil {
		return "background shells are not available for this session", true, nil
	}
	sh, ok := env.FS.GetShell(args.ShellID)
	if !ok {
		return "unknown shell_id: " + args.ShellID, true, nil
	}

	snap := sh.Snapshot()
	out := snap.Output
	if args.Filter != "" {
		re, reErr := regexp.Compile(args.Filter)
		if reErr != nil {
			return "invalid filter regex: " + reErr.Error(), true, nil
		}
		out = filterLines(out, re)
	}
	if out == "" {
		out = "(no new output)"
	}
	return fmt.Sprintf("status: %s\nexit_code: %d\n%s", snap.Status, snap.ExitCode, out), false, nil
}

func filterLines(s string, re *regexp.Regexp) string {
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if re.MatchString(l) {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

// killShellHandler implements kill_shell: terminate a background shell
// started via bash's run_in_background.
type killShellHandler struct{}

func (killShellHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "kill_shell",
		Description: "Kill a running background shell started by bash with run_in_background=true, by its shell_id.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"shell_id":{"type":"string","description":"The background shell ID to kill"}
},
"required":["shell_id"],
"additionalProperties":false
}`),
	}
}

func (killShellHandler) Invoke(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		ShellID string `json:"shell_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.FS == nil {
		return "background shells are not available for this session", true, nil
	}
	sh, ok := env.FS.GetShell(args.ShellID)
	if !ok {
		return "unknown shell_id: " + args.ShellID, true, nil
	}
	sh.Kill()
	return "killed " + args.ShellID, false, nil
}
