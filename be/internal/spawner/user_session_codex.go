package spawner

import (
	"fmt"
	"os"

	"be/internal/model"
	"be/internal/pty"
)

// PrepareUserSession builds the argv for a human-driven codex PTY session
// (orchestrator interactive/plan pre-step, take-control resume). Parity with
// Claude's managed-session boundary: interactive mode gets
// --dangerously-bypass-approvals-and-sandbox (Claude's
// --dangerously-skip-permissions equivalent). Plan mode has no native codex
// permission mode (confirmed against `codex --help` v0.144.1: -s/--sandbox
// {read-only,workspace-write,danger-full-access}, -a/--ask-for-approval
// {untrusted,on-request,never}), so it uses --sandbox read-only
// --ask-for-approval on-request — on-request lets the model still escalate to
// write its plan file when needed, without a standing write sandbox.
func (a *CodexAdapter) PrepareUserSession(opts UserSessionOptions) (pty.Launch, func(), error) {
	dir, err := os.MkdirTemp("", "nrflo-codex-user-"+opts.SessionID+"-*")
	if err != nil {
		return pty.Launch{}, func() {}, err
	}
	if err := writeCodexProfileForSession(dir, opts.WorkDir); err != nil {
		_ = os.RemoveAll(dir)
		return pty.Launch{}, func() {}, fmt.Errorf("write codex profile: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	args := []string{
		"--model", opts.Model,
		"-c", "check_for_update_on_startup=false",
	}
	// The codex TUI has no --effort flag; effort rides as a -c override (same
	// mechanism as check_for_update_on_startup above).
	if opts.ReasoningEffort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", opts.ReasoningEffort))
	}
	if opts.PlanMode {
		args = append(args, "--sandbox", model.SandboxReadOnly, "--ask-for-approval", "on-request")
	} else {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	// Prompt goes as a trailing argv positional, not PTY stdin: codex's TUI
	// input box panics on multi-KB pasted bodies (tui/src/wrapping.rs:52).
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}

	env := []string{"CODEX_HOME=" + dir}
	if os.Getenv("TERM") == "" {
		env = append(env, "TERM=xterm-256color")
	}

	return pty.Launch{Command: "codex", Args: args, Env: env, Dir: opts.WorkDir}, cleanup, nil
}

// PlanPromptSuffix tells the agent where to write its plan — codex has no
// native plan store, so ReadPlan has to read it back from a known path.
func (a *CodexAdapter) PlanPromptSuffix(opts PlanCaptureOptions) string {
	return fmt.Sprintf("\n\nWhen your plan is complete, write the final plan as markdown to %s and change no other file. Do not implement the plan.", opts.PlanFile)
}

// ReadPlan reads back the plan file codex was instructed to write via
// PlanPromptSuffix.
func (a *CodexAdapter) ReadPlan(opts PlanCaptureOptions) string {
	content, err := os.ReadFile(opts.PlanFile)
	if err != nil {
		return ""
	}
	return string(content)
}
