package spawner

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
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
	cmd.Env = opts.Env
	return cmd
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
// Invocation form: opencode <workdir> --port <port> --hostname 127.0.0.1 --model <model>
// The port is pre-allocated by PrepareInteractive.
func (a *OpencodeAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		opts.WorkDir,
		"--port", strconv.Itoa(opts.Port),
		"--hostname", "127.0.0.1",
		"--model", opts.Model,
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--variant", opts.ReasoningEffort)
	}
	cmd := exec.Command("opencode", args...)
	cmd.Dir = opts.WorkDir
	env := opts.Env
	if !envHasKey(env, "TERM=") {
		env = append(env, "TERM=xterm-256color")
	}
	cmd.Env = env
	return cmd
}

// PrepareInteractive picks a free localhost port for the opencode TUI.
// opencode owns the port lifecycle; cleanup is a no-op.
func (a *OpencodeAdapter) PrepareInteractive(_ InteractivePrepOptions) (InteractiveExtras, func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return InteractiveExtras{}, func() {}, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return InteractiveExtras{Port: port}, func() {}, nil
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

func (a *OpencodeAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}

