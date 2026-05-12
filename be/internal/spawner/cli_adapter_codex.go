package spawner

import (
	"fmt"
	"os"
	"os/exec"
)

// CodexAdapter implements CLIAdapter for OpenAI Codex CLI
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	model := opts.MappedModel
	if model == "" {
		model = a.MapModel(opts.Model)
	}
	reasoningEffort := opts.ReasoningEffort
	if reasoningEffort == "" {
		reasoningEffort = a.GetReasoningEffort(opts.Model)
	}

	args := []string{
		"exec",
		"--json",
		"--model", model,
		"-c", fmt.Sprintf("model_reasoning_effort=\"%s\"", reasoningEffort),
		"-c", "check_for_update_on_startup=false",
		"--dangerously-bypass-approvals-and-sandbox",
	}

	// Prompt is piped via stdin (UsesStdinPrompt=true), no positional arg

	cmd := exec.Command("codex", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

func (a *CodexAdapter) MapModel(model string) string {
	modelMap := map[string]string{
		"codex_gpt_normal":   "gpt-5.3-codex",
		"codex_gpt_high":     "gpt-5.3-codex",
		"codex_gpt54_normal": "gpt-5.4",
		"codex_gpt54_high":   "gpt-5.4",
	}
	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model // pass through custom model names
}

// GetReasoningEffort returns the reasoning effort level for a model alias
func (a *CodexAdapter) GetReasoningEffort(model string) string {
	switch model {
	case "codex_gpt_normal", "codex_gpt_high":
		return "high"
	case "codex_gpt54_normal":
		return "medium"
	case "codex_gpt54_high":
		return "high"
	default:
		return "high"
	}
}

func (a *CodexAdapter) SupportsSessionID() bool {
	return false // Codex generates its own session IDs
}

func (a *CodexAdapter) SupportsSystemPromptFile() bool {
	return false // Prompt piped via stdin
}

func (a *CodexAdapter) SupportsResume() bool {
	return false
}

func (a *CodexAdapter) UsesStdinPrompt() bool {
	return true
}

func (a *CodexAdapter) SupportsInteractive() bool { return true }

func (a *CodexAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		"--model", opts.Model,
		"-c", "check_for_update_on_startup=false",
		"--dangerously-bypass-approvals-and-sandbox",
	}
	// Codex's TUI in PTY contexts ignores `<CODEX_HOME>/config.toml` hooks
	// entirely. The `-c` flag is documented as a session-layer override
	// applied last (after user/managed config layers), so registering hooks
	// inline via `-c hooks.<event>=[…]` is the only path that survives the
	// TUI's hook-config bypass. One `-c` per event.
	for _, h := range opts.Hooks {
		args = append(args,
			"-c",
			fmt.Sprintf(`hooks.%s=[{matcher="*",hooks=[{type="command",command=%q,timeout=%d}]}]`,
				h.Event, h.Command, h.TimeoutSec),
		)
	}
	// Codex's TUI input box has a wrapping bug (`tui/src/wrapping.rs:52`,
	// usize subtraction underflow) that panics on multi-KB pasted bodies. We
	// pass the prompt as an argv positional instead so codex pre-loads it as
	// the first user message and never tries to wrap it interactively.
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	cmd := exec.Command("codex", args...)
	cmd.Dir = opts.WorkDir
	env := opts.Env
	if opts.CodexHome != "" {
		// Strip any inherited CODEX_HOME (e.g. from the user's shell env) so
		// our value isn't shadowed. macOS getenv typically returns the FIRST
		// match in environ, and a plain `append` puts ours at the end where
		// it loses to anything inherited via the shell.
		env = removeEnvKey(env, "CODEX_HOME=")
		env = append(env, "CODEX_HOME="+opts.CodexHome)
	}
	// Codex's TUI sends DSR/DA terminal capability queries (\x1b[6n, \x1b[c,
	// \x1b[?u) on startup and bails out when no replies arrive. Our creack/pty
	// PTY has no terminal emulator on the master side to answer them. Force
	// TERM to a known value so codex skips those probes and proceeds to the
	// interactive loop (where SessionStart fires and our hooks register).
	if !envHasKey(env, "TERM=") {
		env = append(env, "TERM=xterm-256color")
	}
	cmd.Env = env
	return cmd
}

// envHasKey reports whether the env slice already contains an assignment for
// the given prefix (e.g. "TERM=").
func envHasKey(env []string, prefix string) bool {
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// removeEnvKey returns env with all entries matching prefix removed. Used so
// our explicit value isn't shadowed by a duplicate inherited from the parent
// process (macOS getenv typically returns the first match in environ).
func removeEnvKey(env []string, prefix string) []string {
	out := env[:0:0]
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			continue
		}
		out = append(out, e)
	}
	return out
}

// PrepareInteractive creates a per-session CODEX_HOME profile dir (auth +
// model/personality + workDir trust + [features] codex_hooks=true) and builds
// the hook event list the backend must inject via repeated `-c hooks.<event>=…`
// flags. Returns a cleanup func that removes the temp dir (best-effort).
func (a *CodexAdapter) PrepareInteractive(opts InteractivePrepOptions) (InteractiveExtras, func(), error) {
	dir, err := os.MkdirTemp("", "nrflo-codex-"+opts.SessionID+"-*")
	if err != nil {
		return InteractiveExtras{}, func() {}, err
	}
	if err := writeCodexProfileForSession(dir, resolvedNrfloPath(), opts.SessionID, opts.WorkflowInstanceID, opts.ProjectID, opts.WorkDir); err != nil {
		_ = os.RemoveAll(dir)
		return InteractiveExtras{}, func() {}, fmt.Errorf("write codex profile: %w", err)
	}

	cmd := buildCodexHookCommand(resolvedNrfloPath(), opts.SessionID, opts.WorkflowInstanceID, opts.ProjectID)
	hooks := make([]HookEvent, 0, len(codexHookEvents))
	for _, ev := range codexHookEvents {
		hooks = append(hooks, HookEvent{Event: ev, Command: cmd, TimeoutSec: 5})
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	return InteractiveExtras{CodexHome: dir, Hooks: hooks}, cleanup, nil
}

// DeliversPromptInline returns true — codex receives the prompt as the final
// argv positional (avoids the TUI input-box wrapping panic at
// `tui/src/wrapping.rs:52` on multi-KB pasted bodies). The backend skips PTY
// stdin prompt delivery for codex.
func (a *CodexAdapter) DeliversPromptInline() bool { return true }

// NeedsTerminalQueryReplies returns true — codex's TUI sends DSR/DA/kitty/OSC
// capability queries during init and bails when no replies arrive. The backend
// PTY ferry must auto-answer them.
func (a *CodexAdapter) NeedsTerminalQueryReplies() bool { return true }

// CapturesTUIBytes returns true — codex hooks no longer fire in PTY/TUI
// sessions due to upstream regression openai/codex#21639. Raw PTY bytes are
// captured, ANSI-stripped, and emitted as agent_messages lines as a temporary
// workaround until upstream ships a fix.
//
// Cleanup: once openai/codex#21639 is fixed and a patched codex version is
// available, flip both CapturesTUIBytes and BumpsOnPTYBytes to false together.
// After one release, delete backend_interactive_tui_capture.go, the tuiLineBuf
// field on processInfo, the captureTUI param from ferryPTYOutput, and the
// CapturesTUIBytes and BumpsOnPTYBytes methods from the CLIAdapter interface.
func (a *CodexAdapter) CapturesTUIBytes() bool { return true }

// BumpsOnPTYBytes returns true — while openai/codex#21639 keeps hooks unfired
// in PTY/TUI sessions, PTY bytes are the only heartbeat signal available, so
// they must bump lastMessageTime to prevent false stall detection. Flip this
// to false together with CapturesTUIBytes once the upstream fix ships.
func (a *CodexAdapter) BumpsOnPTYBytes() bool { return true }

func (a *CodexAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}
