package spawner

import (
	"os/exec"
	"time"
)

// ClaudeAdapter implements CLIAdapter for Claude Code CLI
type ClaudeAdapter struct{}

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
