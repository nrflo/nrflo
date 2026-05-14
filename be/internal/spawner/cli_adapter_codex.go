package spawner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// CodexAdapter implements CLIAdapter for OpenAI Codex CLI
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) MapModel(model string) string {
	modelMap := map[string]string{
		"codex_gpt_normal":     "gpt-5.3-codex",
		"codex_gpt_high":       "gpt-5.3-codex",
		"codex_gpt54_normal":   "gpt-5.4",
		"codex_gpt54_high":     "gpt-5.4",
		"codex_gpt54_mini_low": "gpt-5.4-mini",
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
	case "codex_gpt54_mini_low":
		return "low"
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
	return true
}

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
	// Trust is persisted to `~/.codex/config.toml` from PrepareInteractive
	// via ensureCodexUserTrust — codex 0.130 ignores `-c projects."<path>".trust_level=...`
	// overrides (nested quoted-key keys are silently dropped by its parser).
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
// model/personality + [features] codex_hooks=true) and builds the hook event
// list the backend must inject via repeated `-c hooks.<event>=…` flags. Also
// idempotently appends a workDir trust entry to the user-level
// `~/.codex/config.toml` — codex 0.130 reads trust ONLY from that file (the
// per-session CODEX_HOME and the `-c projects."<path>".trust_level=...`
// override are both silently ignored), and the trust prompt blocks even under
// `--dangerously-bypass-approvals-and-sandbox`. Path is symlink-resolved so we
// match codex's own canonicalization (e.g. `/var/folders` → `/private/var/folders`).
// Returns a cleanup func that removes the temp dir (best-effort). The trust
// entry is intentionally left in `~/.codex/config.toml` — codex itself writes
// the same entry when a user clicks "yes", and self-pollution is normal.
func (a *CodexAdapter) PrepareInteractive(opts InteractivePrepOptions) (InteractiveExtras, func(), error) {
	dir, err := os.MkdirTemp("", "nrflo-codex-"+opts.SessionID+"-*")
	if err != nil {
		return InteractiveExtras{}, func() {}, err
	}
	if err := writeCodexProfileForSession(dir, resolvedNrfloPath(), opts.SessionID, opts.WorkflowInstanceID, opts.ProjectID, opts.WorkDir); err != nil {
		_ = os.RemoveAll(dir)
		return InteractiveExtras{}, func() {}, fmt.Errorf("write codex profile: %w", err)
	}
	var trustAdded bool
	var trustResolved string
	if opts.WorkDir != "" {
		added, resolved, err := ensureCodexUserTrust(opts.WorkDir)
		if err != nil {
			// Non-fatal: codex will prompt for trust at TUI startup and the
			// session will hang waiting for an answer. The failure is loud
			// (timeout at the harness/orchestrator deadline) so we don't
			// fail the spawn here.
			_ = err
		}
		trustAdded = added
		trustResolved = resolved
	}

	cmd := buildCodexHookCommand(resolvedNrfloPath(), opts.SessionID, opts.WorkflowInstanceID, opts.ProjectID)
	hooks := make([]HookEvent, 0, len(codexHookEvents))
	for _, ev := range codexHookEvents {
		hooks = append(hooks, HookEvent{Event: ev, Command: cmd, TimeoutSec: 5})
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
		if trustAdded && trustResolved != "" {
			// Only remove entries WE added — pre-existing user/codex-written
			// entries (real projects where the user clicked "yes" once) stay
			// put. Best-effort; failure leaves at most one stale entry.
			_ = removeCodexUserTrust(trustResolved)
		}
	}
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

// BumpsOnPTYBytes returns false — the rollout JSONL tailer (started in
// PostStart) calls Sink.BumpLastMessage on every real agent event
// (agent_message, function_call, function_call_output, token_count). PTY-byte
// bumps are no longer the heartbeat; stall detection is reachable for
// codex/cli_interactive at parity with Claude.
func (a *CodexAdapter) BumpsOnPTYBytes() bool { return false }

// NaturalExitGrace returns 2s — uniform default. Codex's JSONL tailer
// emits records as they happen so a SIGTERM wouldn't strictly drop
// telemetry, but the wait is bounded by doneCh — codex exits naturally
// in well under 2s after its last function_call_output.
func (a *CodexAdapter) NaturalExitGrace() time.Duration { return 2 * time.Second }

// PostStart launches the codex rollout JSONL tailer goroutine. Called by
// cliInteractiveBackend.Start — the rollout JSONL signal is produced in all
// interactive sessions (verified on codex 0.130.0).
//
// codex 0.130 has an upstream regression (openai/codex#21639) where hooks
// never fire in TUI/PTY sessions, so we read agent activity straight from
// codex's own rollout JSONL.
//
// Codex writes rollouts under $HOME/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl
// regardless of CODEX_HOME (verified empirically 2026-05-12 on 0.130). The
// tailer identifies OUR rollout by matching session_meta.payload.cwd against
// our resolved workdir; that's deterministic per scenario because each
// harness/agent run has a unique workdir.
//
// Returns a cleanup func that cancels the tailer goroutine (called by the
// backend when the session ends).
func (a *CodexAdapter) PostStart(ctx context.Context, opts PostStartOptions) (func(), error) {
	if opts.WorkDir == "" {
		return func() {}, fmt.Errorf("codex PostStart: empty WorkDir")
	}
	cancel := startCodexJSONLTail(ctx, opts.SessionID, opts.WorkDir, opts.Sink)
	return func() { cancel() }, nil
}

