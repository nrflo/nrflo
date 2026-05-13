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

// SupportsInteractive returns false. opencode 1.14.48's TUI does not surface
// chat activity through any observable channel: /event and /global/event SSE
// streams emit only `server.connected`; /api/session/{id}/message returns 0
// items even for an actively-running TUI session; storage on disk is never
// populated for `--port`-launched TUI sessions. With no way to capture text
// or tool events, cli_interactive provides zero value over cli batch.
// Workflows requesting `execution_mode=cli_interactive` on an opencode agent
// fail at startBackend with a clear error — fall back to cli batch instead.
// See backlog.md for the full investigation.
func (a *OpencodeAdapter) SupportsInteractive() bool { return false }

// BuildInteractiveCommand is a no-op stub. SupportsInteractive()=false means
// this method is never reached at runtime; it exists only to satisfy the
// CLIAdapter interface contract.
func (a *OpencodeAdapter) BuildInteractiveCommand(_ InteractiveSpawnOptions) *exec.Cmd {
	return nil
}

// PrepareInteractive is a no-op stub. See BuildInteractiveCommand.
func (a *OpencodeAdapter) PrepareInteractive(_ InteractivePrepOptions) (InteractiveExtras, func(), error) {
	return InteractiveExtras{}, func() {}, nil
}

// DeliversPromptInline is a no-op stub. See BuildInteractiveCommand.
func (a *OpencodeAdapter) DeliversPromptInline() bool { return false }

// NeedsTerminalQueryReplies is a no-op stub. See BuildInteractiveCommand.
func (a *OpencodeAdapter) NeedsTerminalQueryReplies() bool { return false }

// BumpsOnPTYBytes is a no-op stub. See BuildInteractiveCommand.
func (a *OpencodeAdapter) BumpsOnPTYBytes() bool { return false }

func (a *OpencodeAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}

