package spawner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// OpencodeAdapter implements CLIAdapter for Opencode CLI
type OpencodeAdapter struct{}

func (a *OpencodeAdapter) Name() string {
	return "opencode"
}

// overridePWD replaces or appends PWD=<workDir> in env. opencode resolves
// its project_root via $PWD (Bun runtime), not os.Getwd(); without this
// override the child inherits the server's PWD and registers all sessions
// against the wrong project, which breaks the SQLite tailer's
// `project.worktree = ?` join.
func overridePWD(env []string, workDir string) []string {
	if workDir == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, e := range env {
		if strings.HasPrefix(e, "PWD=") {
			out = append(out, "PWD="+workDir)
			replaced = true
		} else {
			out = append(out, e)
		}
	}
	if !replaced {
		out = append(out, "PWD="+workDir)
	}
	return out
}

func (a *OpencodeAdapter) MapModel(model string) string {
	// If already in provider/model format, return as-is
	if strings.Contains(model, "/") {
		return model
	}

	modelMap := map[string]string{
		"opencode_minimax_m25_free": "opencode/minimax-m2.5-free",
		"opencode_qwen36_plus_free": "opencode/qwen3.6-plus-free",
		"opencode_gpt54":            "openai/gpt-5.4",
		"opencode_gpt54_mini_low":   "openai/gpt-5.4-mini",
	}

	if mapped, ok := modelMap[model]; ok {
		return mapped
	}

	// Default: assume anthropic provider
	return "anthropic/" + model
}

// GetReasoningEffort returns the reasoning effort variant for a model alias.
// Opencode uses --variant flag with values: max, high, medium, low, minimal
func (a *OpencodeAdapter) GetReasoningEffort(model string) string {
	switch model {
	case "opencode_gpt54":
		return "high"
	case "opencode_gpt54_mini_low":
		return "low"
	default:
		return ""
	}
}

func (a *OpencodeAdapter) SupportsSessionID() bool {
	return false // Opencode generates its own session IDs
}

func (a *OpencodeAdapter) SupportsSystemPromptFile() bool {
	return false // Suffix prepended to prompt body in deliverPrompt
}

func (a *OpencodeAdapter) SupportsResume() bool {
	return false
}

// PostStart launches the opencode SQLite DB tailer goroutine for context
// tracking. Called by cliInteractiveBackend.Start.
func (a *OpencodeAdapter) PostStart(ctx context.Context, opts PostStartOptions) (func(), error) {
	if opts.WorkDir == "" {
		return func() {}, fmt.Errorf("opencode PostStart: empty WorkDir")
	}
	cancel := startOpencodeSQLiteTail(ctx, opts.SessionID, opts.WorkDir, opts.StartedAt, opts.MaxContext, opts.Sink)
	return func() { cancel() }, nil
}

// BuildInteractiveCommand builds the PTY command for opencode TUI mode.
// Invocation form: opencode <workdir> --port 0 --hostname 127.0.0.1 --model <model>
//
// `--port 0` lets opencode pick its own free port (race-free).
//
// `--variant` is not passed: the TUI subcommand does not accept it.
// Reasoning-effort selection happens via model alias resolution.
//
// ResumeSessionID is not supported by opencode; context carryover uses the
// agent-saver path instead.
func (a *OpencodeAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		opts.WorkDir,
		"--port", "0",
		"--hostname", "127.0.0.1",
		"--model", opts.Model,
	}
	cmd := exec.Command("opencode", args...)
	cmd.Dir = opts.WorkDir
	env := overridePWD(opts.Env, opts.WorkDir)
	if !envHasKey(env, "TERM=") {
		env = append(env, "TERM=xterm-256color")
	}
	cmd.Env = env
	return cmd
}

// PrepareInteractive is a no-op for opencode: `--port 0` lets opencode
// self-allocate, so nrflo-side prep is empty. Kept on the interface for
// symmetry with adapters that do need pre-spawn prep (codex profile + hooks).
func (a *OpencodeAdapter) PrepareInteractive(_ InteractivePrepOptions) (InteractiveExtras, func(), error) {
	return InteractiveExtras{}, func() {}, nil
}

// DeliversPromptInline returns false: prompt is delivered via PTY stdin after
// readiness.
func (a *OpencodeAdapter) DeliversPromptInline() bool { return false }

// NeedsTerminalQueryReplies returns false: the opencode TUI does not probe
// terminal capabilities.
func (a *OpencodeAdapter) NeedsTerminalQueryReplies() bool { return false }

// BumpsOnPTYBytes returns false: context tracking flows through the SQLite
// tailer and ferry heartbeat, not PTY bytes.
func (a *OpencodeAdapter) BumpsOnPTYBytes() bool { return false }

// NaturalExitGrace returns 5s. opencode writes its `step-finish` part
// (carrying the turn's token usage) AFTER the agent's final tool call
// returns and the model emits its closing assistant chunk; the SQLite
// commit happens via an async write pool. Under high parallel load,
// opencode sometimes needs 3-4 seconds to flush. If opencode exits
// earlier the kill-loop's doneCh check exits the wait immediately,
// so the ceiling is harmless in the common case.
func (a *OpencodeAdapter) NaturalExitGrace() time.Duration { return 5 * time.Second }

// ClassifyExit has empty defaults; all patterns are user-extensible via
// config keys (opencode_limit_patterns, opencode_error_patterns).
func (a *OpencodeAdapter) ClassifyExit(recentText, stderrTail string, exitCode int, extraLimitPatterns, extraErrorPatterns []string) (RetryClass, string) {
	combined := recentText + "\n" + stderrTail
	if len(extraLimitPatterns) > 0 {
		if p, ok := matchAnyCaseInsensitive(combined, extraLimitPatterns); ok {
			return RetryClassRateLimit, p
		}
	}
	if len(extraErrorPatterns) > 0 {
		if p, ok := matchAnyCaseInsensitive(combined, extraErrorPatterns); ok {
			return RetryClassError, p
		}
	}
	return RetryClassNone, ""
}
