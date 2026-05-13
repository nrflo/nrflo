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

func (a *OpencodeAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	// Opencode uses provider/model format
	model := opts.MappedModel
	if model == "" {
		model = a.MapModel(opts.Model)
	}
	reasoningEffort := opts.ReasoningEffort
	if reasoningEffort == "" {
		reasoningEffort = a.GetReasoningEffort(opts.Model)
	}

	args := []string{
		"run",
		"--format", "json",
		"--model", model,
	}

	// Add reasoning effort variant if specified
	if reasoningEffort != "" {
		args = append(args, "--variant", reasoningEffort)
	}

	// Opencode reads message from positional args, not stdin
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}

	cmd := exec.Command("opencode", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = overridePWD(opts.Env, opts.WorkDir)
	return cmd
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

func (a *OpencodeAdapter) UsesStdinPrompt() bool {
	return false // opencode reads message from positional args
}

// SupportsInteractive returns true. Interactive runs launch the opencode TUI
// against a free localhost port; agent activity is observed via the SQLite
// tailer started by PostStart.
func (a *OpencodeAdapter) SupportsInteractive() bool { return true }

// PostStart launches the opencode SQLite DB tailer goroutine for context
// tracking. Called by both cliBackend.Start (cli batch) and
// cliInteractiveBackend.Start (cli_interactive).
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
// `--port 0` lets opencode pick its own free port. We used to pre-allocate
// in PrepareInteractive and pass the bound port here, but that introduced
// a TOCTOU race: under parallel spawning, the just-closed listener's port
// could be grabbed by another process before opencode binds, and opencode
// would exit with code 1 within a second. Nothing nrflo-side needs to
// know the port (the SQLite tailer reads the local DB, not the HTTP
// server), so handing port selection to opencode is both simpler and
// race-free.
//
// `--variant` is intentionally NOT passed here: the TUI subcommand
// (`opencode [project]`) does not accept it — that flag is exclusive to
// `opencode run` (batch mode). Passing it makes opencode print its help
// and exit 1 before any prompt is delivered. Reasoning-effort selection
// in TUI mode happens via the model alias resolution (e.g.
// `openai/gpt-5.4-mini` already carries the "low" profile in opencode's
// own catalogue).
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

func (a *OpencodeAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}

