package spawner

import (
	"os/exec"
	"time"
)

// ClaudeAdapter implements CLIAdapter for Claude Code CLI
type ClaudeAdapter struct{}

// claudeDisallowedNativeTools denies the CLI's own multi-agent orchestration
// tools so a managed session cannot spawn children invisible to nrflo.
//
// The delegation tool is "Agent" in both the Docker-pinned 2.1.178 and 2.1.207;
// "Task" is its pre-rename name, kept for older CLIs. Deny names are matched
// exactly and unknown ones are silently ignored, so listing "Task" neither
// fails the spawn nor prefix-matches the unrelated Task* background-task tools.
// mcp__nrflo__* and ordinary coding tools (Bash/Edit/Read/Write/...) survive.
// Drift alarm for these names: TestNativeOrchestrationCLI (-tags clitools).
const claudeDisallowedNativeTools = "Agent Task Workflow SendMessage"

func (a *ClaudeAdapter) Name() string {
	return "claude"
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
	case "opus_4_8":
		return "claude-opus-4-8"
	case "opus_4_8_1m":
		return "claude-opus-4-8[1m]"
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

func (a *ClaudeAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		"--session-id", opts.SessionID,
		"--model", opts.Model,
		"--dangerously-skip-permissions",
	}
	if opts.ResumeSessionID != "" {
		args = append([]string{"--resume", opts.ResumeSessionID}, args...)
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--effort", opts.ReasoningEffort)
	}
	if opts.FallbackModels != "" {
		args = append(args, "--fallback-model", opts.FallbackModels)
	}
	if opts.SettingsJSON != "" {
		args = append(args, "--settings", opts.SettingsJSON)
	}
	if opts.SystemPromptOverrideFile != "" {
		args = append(args, "--system-prompt-file", opts.SystemPromptOverrideFile)
	}
	if opts.SystemPromptFile != "" {
		args = append(args, "--append-system-prompt-file", opts.SystemPromptFile)
	}
	if opts.NativeToolsCSV != "" {
		args = append(args, "--tools", opts.NativeToolsCSV)
	}
	if opts.MCPConfigJSON != "" {
		args = append(args, "--mcp-config", opts.MCPConfigJSON, "--strict-mcp-config")
	}
	if opts.AllowedToolsCSV != "" {
		args = append(args, "--allowedTools", opts.AllowedToolsCSV)
	}
	// Must stay the last flag pair: --disallowedTools is variadic and greedily
	// consumes any following positional argv (it stops at the next `-`-prefixed
	// flag). Claude never receives a positional prompt (DeliversPromptInline()
	// is false), but keep this trailing to avoid ever swallowing one.
	args = append(args, "--disallowedTools", claudeDisallowedNativeTools)
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

// BumpsOnPTYBytes returns false — PreToolUse/PostToolUse/Stop hooks drive
// heartbeat via record_event → BumpLastMessage, so PTY bytes must not reset
// the stall timer or stall detection becomes unreachable during idle redraws.
func (a *ClaudeAdapter) BumpsOnPTYBytes() bool { return false }

// NaturalExitGrace returns 2s — uniform default. Claude doesn't strictly
// need it (hook events fire on every tool call), but waiting on doneCh
// is harmless: if claude exits naturally first, the wait returns
// immediately. Keeping the grace consistent across adapters avoids
// surprises when adapters' telemetry-flush timing changes upstream.
func (a *ClaudeAdapter) NaturalExitGrace() time.Duration { return 2 * time.Second }

// ClassifyExit inspects recent output to classify an abnormal exit.
// Rate-limit patterns are checked before error patterns; user-supplied extras
// are merged with defaults so site-level overrides extend, not replace, them.
func (a *ClaudeAdapter) ClassifyExit(recentText, stderrTail string, exitCode int, extraLimitPatterns, extraErrorPatterns []string) (RetryClass, string) {
	limitPatterns := append([]string{
		"You've hit your limit",
		"You've hit your org's monthly usage limit",
		"Your usage allocation has been disabled by your admin",
		// Server-side overload (HTTP 529). Anthropic's overloaded_error surfaces
		// in the CLI as "API Error: 529 Overloaded"; treat it as a rate limit so
		// it gets backoff+retry instead of being a terminal error.
		"Overloaded",
	}, extraLimitPatterns...)
	errorPatterns := append([]string{
		"API Error:",
		"cannot be launched inside another Claude Code session",
		"Not logged in",
	}, extraErrorPatterns...)
	combined := recentText + "\n" + stderrTail
	if p, ok := matchAnyCaseInsensitive(combined, limitPatterns); ok {
		return RetryClassRateLimit, p
	}
	if p, ok := matchAnyCaseInsensitive(combined, errorPatterns); ok {
		return RetryClassError, p
	}
	return RetryClassNone, ""
}
