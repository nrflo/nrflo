package spawner

import (
	"os/exec"
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

// GetReasoningEffort returns the reasoning effort variant for a model alias
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
	return false // Prompt piped via stdin
}

func (a *OpencodeAdapter) SupportsResume() bool {
	return false
}

func (a *OpencodeAdapter) UsesStdinPrompt() bool {
	return false // opencode reads message from positional args
}

// SupportsInteractive returns false for Opencode: the CLI has no hook system,
// so a PTY-attached run gives us only ANSI-laden TUI bytes with no structured
// telemetry. Opencode agents fall back to cliBackend (non-interactive
// `opencode run`) which produces clean JSON-derived messages.
func (a *OpencodeAdapter) SupportsInteractive() bool { return false }

// BuildInteractiveCommand is unreachable via the spawner selector
// (SupportsInteractive=false) but kept here for the CLIAdapter interface
// contract and direct unit tests. It mirrors BuildCommand's flag layout
// minus the batch-mode `run --format json` arguments.
func (a *OpencodeAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{"--model", opts.Model}
	if opts.ReasoningEffort != "" {
		args = append(args, "--variant", opts.ReasoningEffort)
	}
	cmd := exec.Command("opencode", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

// PrepareInteractive returns zero extras — Opencode never reaches the
// interactive backend (SupportsInteractive=false), so this is unreachable in
// production. Implemented only to satisfy the CLIAdapter contract.
func (a *OpencodeAdapter) PrepareInteractive(_ InteractivePrepOptions) (InteractiveExtras, func(), error) {
	return InteractiveExtras{}, func() {}, nil
}

// DeliversPromptInline returns false (unreachable; opencode falls back to
// cliBackend).
func (a *OpencodeAdapter) DeliversPromptInline() bool { return false }

// NeedsTerminalQueryReplies returns false (unreachable; opencode falls back to
// cliBackend).
func (a *OpencodeAdapter) NeedsTerminalQueryReplies() bool { return false }

func (a *OpencodeAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}
