package spawner

import (
	"context"
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

// SupportsInteractive returns true for Opencode: the embedded HTTP server
// (started via --port / --hostname flags) exposes an SSE /event bus that
// replaces hook telemetry, giving the same structured visibility as Claude's
// --settings hooks or Codex's -c hook injection.
func (a *OpencodeAdapter) SupportsInteractive() bool { return true }

// BuildInteractiveCommand builds the PTY command for an opencode TUI session
// with the embedded HTTP server enabled. The first positional arg is the
// working directory; --port and --hostname start the embedded event server.
// Prompt delivery is via PTY stdin (DeliversPromptInline=false).
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

	// Ensure TERM is set so the TUI can initialize inside the PTY.
	hasTERM := false
	for _, e := range opts.Env {
		if strings.HasPrefix(e, "TERM=") {
			hasTERM = true
			break
		}
	}
	if hasTERM {
		cmd.Env = opts.Env
	} else {
		cmd.Env = append(opts.Env, "TERM=xterm-256color")
	}
	return cmd
}

// PrepareInteractive allocates a free localhost port for the embedded HTTP
// server by binding and immediately releasing a TCP listener. The port is
// returned in InteractiveExtras.Port; opencode will bind it on startup.
func (a *OpencodeAdapter) PrepareInteractive(_ InteractivePrepOptions) (InteractiveExtras, func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return InteractiveExtras{}, func() {}, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return InteractiveExtras{Port: port}, func() {}, nil
}

// PostInteractiveStart launches the in-process SSE consumer goroutine that
// subscribes to opencode's embedded HTTP /event bus and dispatches events
// into the spawner's message/context tracking pipelines. Returns a cleanup
// func that stops the consumer goroutine.
func (a *OpencodeAdapter) PostInteractiveStart(ctx context.Context, opts PostInteractiveStartOptions) (func(), error) {
	cancel := startOpencodeEventStream(ctx, opts.Port, opts.SessionID, opts.WorkDir, opts.Sink)
	return cancel, nil
}

// DeliversPromptInline returns false: prompt is delivered via PTY stdin Write
// after the readiness delay, identical to Claude's interactive path.
func (a *OpencodeAdapter) DeliversPromptInline() bool { return false }

// NeedsTerminalQueryReplies returns false: opencode's TUI does not send
// DSR/DA/kitty/OSC capability queries that require auto-replies.
func (a *OpencodeAdapter) NeedsTerminalQueryReplies() bool { return false }

// CapturesTUIBytes returns false — opencode delivers messages via its SSE
// event bus; raw PTY byte capture is not needed.
func (a *OpencodeAdapter) CapturesTUIBytes() bool { return false }

// BumpsOnPTYBytes returns false — SSE bus message.part.updated /
// session.idle events already call BumpLastMessage, so PTY bytes must not
// reset the stall timer or stall detection becomes unreachable during redraws.
func (a *OpencodeAdapter) BumpsOnPTYBytes() bool { return false }

func (a *OpencodeAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}

