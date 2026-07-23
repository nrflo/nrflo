package spawner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"be/internal/logger"
)

// CodexAdapter implements CLIAdapter for OpenAI Codex CLI
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) MapModel(model string) string {
	return model
}

func (a *CodexAdapter) SupportsSessionID() bool {
	return false // Codex generates its own session IDs
}

func (a *CodexAdapter) SupportsSystemPromptFile() bool {
	return false // Prompt piped via stdin
}

func (a *CodexAdapter) SupportsResume() bool {
	return true
}

func (a *CodexAdapter) SupportsNativeDocRead() bool {
	return false // codex cannot turn a local path into vision input on its own
}

// BuildInteractiveCommand and the other PTY-path methods below back the
// human-session PTY paths (take-control resume, API resume launch); the
// autonomous codex/cli_interactive spawn is still routed to the codex
// app-server backend (codex_appserver_backend.go). CodexAdapter is also
// resolved for passthrough model handling and ClassifyExit.
func (a *CodexAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		"--model", opts.Model,
		"-c", "check_for_update_on_startup=false",
		"--dangerously-bypass-approvals-and-sandbox",
	}
	if opts.ResumeSessionID != "" {
		// Prepend `resume <id>` subcommand so codex resumes the existing session.
		args = append([]string{"resume", opts.ResumeSessionID}, args...)
	}
	// codex 0.145 fires hooks from the TUI under a PTY given a
	// CODEX_HOME/hooks.json + --dangerously-bypass-hook-trust (validated
	// locally; see hooks_settings_codex.go). Workdir trust is delivered
	// through the per-session CODEX_HOME profile (writeCodexProfileForSession)
	// via the `[projects."<path>"] trust_level="trusted"` entry in
	// CODEX_HOME/config.toml, which suppresses the otherwise-blocking
	// directory-trust dialog.
	if opts.CodexHome != "" {
		// Only bypass hook trust when nrflo owns CODEX_HOME (this profile
		// carries our hooks.json). A resume launch with no CodexHome
		// (registerTakeControlResumeLaunch, registerResumeLaunch) points at
		// the user's real ~/.codex — bypassing trust there would run the
		// user's own hooks.json unreviewed.
		args = append(args, "--dangerously-bypass-hook-trust")
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
	// \x1b[?u) on startup and bails out when no replies arrive. Force TERM to a
	// known value so codex skips those probes and proceeds to the interactive loop.
	if !envHasTERM(env) {
		env = append(env, "TERM=xterm-256color")
	}
	cmd.Env = env
	return cmd
}

// envHasTERM reports whether the env slice already sets TERM.
func envHasTERM(env []string) bool {
	const p = "TERM="
	for _, e := range env {
		if len(e) >= len(p) && e[:len(p)] == p {
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

// PrepareInteractive creates a per-session CODEX_HOME profile dir holding the
// user's auth + config (hook tables stripped) plus a `[projects."<workDir>"]
// trust_level="trusted"` entry, and a hooks.json lifecycle sidecar
// (writeCodexHooksForSession, non-fatal on error). codex 0.145 reads workdir
// trust from CODEX_HOME/config.toml; without it the TUI blocks on a
// directory-trust dialog even under `--dangerously-bypass-approvals-and-sandbox`.
// The workDir path is symlink-resolved inside writeCodexProfileForSession to
// match codex's own cwd canonicalization (e.g. `/var/folders` →
// `/private/var/folders` on macOS). Returns a cleanup func that removes the
// temp dir (best-effort).
func (a *CodexAdapter) PrepareInteractive(opts InteractivePrepOptions) (InteractiveExtras, func(), error) {
	dir, err := os.MkdirTemp("", "nrflo-codex-"+opts.SessionID+"-*")
	if err != nil {
		return InteractiveExtras{}, func() {}, err
	}
	if err := writeCodexProfileForSession(dir, opts.WorkDir); err != nil {
		_ = os.RemoveAll(dir)
		return InteractiveExtras{}, func() {}, fmt.Errorf("write codex profile: %w", err)
	}
	if err := writeCodexHooksForSession(dir); err != nil {
		logger.Warn(context.Background(), "codex interactive prep: write hooks.json failed", "session_id", opts.SessionID, "err", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	return InteractiveExtras{CodexHome: dir}, cleanup, nil
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

// BumpsOnPTYBytes returns true. codex PTY user sessions now emit hook events
// (see hooks_settings_codex.go), but PTY bytes stay the heartbeat floor for
// this path: the (currently app-server-routed) managed cli_interactive spawn
// never reaches cliInteractiveBackend, so flipping this only affects the
// human-session PTY paths, which have no proc to bump against a structured
// channel anyway. Kept true as a deliberate deviation — the TUI's continuous
// redraws remain the fallback liveness signal.
func (a *CodexAdapter) BumpsOnPTYBytes() bool { return true }

// NaturalExitGrace returns 2s — uniform default for the grace before SIGTERM.
func (a *CodexAdapter) NaturalExitGrace() time.Duration { return 2 * time.Second }

// TranscriptPath returns "" — codex writes rollout JSONL under CODEX_HOME,
// not a claude-shaped transcript; the context ledger is fed by the app-server
// event emitter instead (codex_appserver_events.go).
func (a *CodexAdapter) TranscriptPath(env []string, workDir, sessionID string) string {
	return ""
}

// ClassifyExit inspects recent output to classify an abnormal exit.
// Codex error patterns are empty by default; users extend via config keys.
func (a *CodexAdapter) ClassifyExit(recentText, stderrTail string, exitCode int, extraLimitPatterns, extraErrorPatterns []string) (RetryClass, string) {
	limitPatterns := append([]string{
		"Rate limit exceeded",
		"rate limit reached",
		"429 Too Many Requests",
		"quota exceeded",
		"insufficient_quota",
		"You've hit your usage limit",
	}, extraLimitPatterns...)
	combined := recentText + "\n" + stderrTail
	if p, ok := matchAnyCaseInsensitive(combined, limitPatterns); ok {
		return RetryClassRateLimit, p
	}
	if len(extraErrorPatterns) > 0 {
		if p, ok := matchAnyCaseInsensitive(combined, extraErrorPatterns); ok {
			return RetryClassError, p
		}
	}
	return RetryClassNone, ""
}
