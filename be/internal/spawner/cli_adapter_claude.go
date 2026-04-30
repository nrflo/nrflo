package spawner

import "os/exec"

// ClaudeAdapter implements CLIAdapter for Claude Code CLI
type ClaudeAdapter struct{}

func (a *ClaudeAdapter) Name() string {
	return "claude"
}

func (a *ClaudeAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	model := opts.MappedModel
	if model == "" {
		model = a.MapModel(opts.Model)
	}
	args := []string{
		"--print",
		"--verbose",
		"--dangerously-skip-permissions",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--disallowed-tools", "AskUserQuestion,EnterPlanMode,ExitPlanMode",
		"--model", model,
		"--session-id", opts.SessionID,
		// prompt piped via stdin — no PromptFile arg, no InitialPrompt
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--effort", opts.ReasoningEffort)
	}
	if opts.SettingsJSON != "" {
		args = append(args, "--settings", opts.SettingsJSON)
	}
	if opts.SystemPromptFile != "" {
		args = append(args, "--append-system-prompt-file", opts.SystemPromptFile)
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

func (a *ClaudeAdapter) MapModel(model string) string {
	switch model {
	case "opus_4_6":
		return "claude-opus-4-6"
	case "opus_4_6_1m":
		return "claude-opus-4-6[1m]"
	case "opus_4_7":
		return "claude-opus-4-7"
	case "opus_4_7_1m":
		return "claude-opus-4-7[1m]"
	}
	return model
}

func (a *ClaudeAdapter) SupportsSessionID() bool {
	return true
}

func (a *ClaudeAdapter) SupportsSystemPromptFile() bool {
	return true
}

func (a *ClaudeAdapter) SupportsResume() bool {
	return true
}

func (a *ClaudeAdapter) UsesStdinPrompt() bool {
	return true
}

func (a *ClaudeAdapter) SupportsInteractive() bool { return true }

func (a *ClaudeAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		"--session-id", opts.SessionID,
		"--model", opts.Model,
		"--dangerously-skip-permissions",
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--effort", opts.ReasoningEffort)
	}
	if opts.SettingsJSON != "" {
		args = append(args, "--settings", opts.SettingsJSON)
	}
	if opts.SystemPromptFile != "" {
		args = append(args, "--append-system-prompt-file", opts.SystemPromptFile)
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

// PrepareInteractive returns zero extras and a noop cleanup — Claude needs no
// per-session profile dir or out-of-band hook events; --settings JSON is set
// directly on InteractiveSpawnOptions by the backend.
func (a *ClaudeAdapter) PrepareInteractive(_ InteractivePrepOptions) (InteractiveExtras, func(), error) {
	return InteractiveExtras{}, func() {}, nil
}

// DeliversPromptInline returns false — Claude's prompt body is written to PTY
// stdin by the backend after the readiness delay.
func (a *ClaudeAdapter) DeliversPromptInline() bool { return false }

// NeedsTerminalQueryReplies returns false — Claude's TUI does not probe the
// host terminal during init, so the PTY ferry skips the canned-reply responder.
func (a *ClaudeAdapter) NeedsTerminalQueryReplies() bool { return false }

func (a *ClaudeAdapter) BuildResumeCommand(opts ResumeOptions) *exec.Cmd {
	args := []string{
		"--resume", opts.SessionID,
		"--print",
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--disallowed-tools", "AskUserQuestion,EnterPlanMode,ExitPlanMode",
		// prompt piped via stdin (same as BuildCommand)
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--effort", opts.ReasoningEffort)
	}
	if opts.SettingsJSON != "" {
		args = append(args, "--settings", opts.SettingsJSON)
	}
	if opts.SystemPromptFile != "" {
		args = append(args, "--append-system-prompt-file", opts.SystemPromptFile)
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}
