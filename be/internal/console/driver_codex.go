package console

import (
	"fmt"
	"os"

	"be/internal/spawner"
)

// codexDriver launches the native `codex` CLI as a human console session.
//
// NO --dangerously-bypass-approvals-and-sandbox is passed: codex's own
// approval/sandbox prompts stay in force for a human sitting at a real TTY —
// unlike a managed spawner session, there is no reason to bypass them here.
type codexDriver struct{}

func (d *codexDriver) Name() string { return "codex" }

func (d *codexDriver) Probe() error {
	if _, err := lookPath("codex"); err != nil {
		return fmt.Errorf("codex CLI not found on PATH — install it from https://github.com/openai/codex: %w", err)
	}
	return nil
}

func (d *codexDriver) Prepare(in LaunchInput) (LaunchSpec, func(), error) {
	dir, err := os.MkdirTemp("", "nrflo-console-codex-*")
	if err != nil {
		return LaunchSpec{}, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	if err := spawner.WriteConsoleCodexProfile(dir, in.WorkDir, in.NrfloPath, bridgeEnv(in)); err != nil {
		cleanup()
		return LaunchSpec{}, func() {}, fmt.Errorf("write codex console profile: %w", err)
	}

	model := in.MappedModel
	if model == "" && in.RawModel != "" {
		model = (&spawner.CodexAdapter{}).MapModel(in.RawModel)
	}
	argv := []string{"codex"}
	if model != "" {
		argv = append(argv, "--model", model)
	}

	// Reasoning effort, registry row first, else the adapter's own alias table
	// — mirrors codex_appserver_backend.go:74-76. Without this,
	// codex_gpt55_high and codex_gpt55_normal produce identical launches (both
	// MapModel to gpt-5.5) and the user silently gets medium effort.
	//
	// The codex TUI has no --effort flag; effort is the `model_reasoning_effort`
	// config key, delivered as a `-c key=value` override (same mechanism as
	// `-c check_for_update_on_startup=false` in cli_adapter_codex.go). It cannot
	// be appended to the profile config.toml: writeCodexProfileForSession ends
	// the file with a [projects."<dir>"] table, and a bare key after a table
	// header would land inside that table.
	effort := in.ReasoningEffort
	if effort == "" && in.RawModel != "" {
		effort = (&spawner.CodexAdapter{}).GetReasoningEffort(in.RawModel)
	}
	if effort != "" {
		argv = append(argv, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
	}
	// No opening ticket hint here (unlike claudeDriver): the codex TUI has no
	// system-prompt-append, and its only extra-instruction channel is a project
	// doc in the cwd (project_doc_fallback_filenames), which we must not write
	// into the user's repo. The model gets the current ticket from the
	// ticket_current tool instead (in.CurrentTicket is still surfaced on stderr
	// by the console command).

	return LaunchSpec{
		Argv: argv,
		Env:  withCodexHome(os.Environ(), dir),
		Dir:  in.WorkDir,
	}, cleanup, nil
}

// withCodexHome returns env with any existing CODEX_HOME entry removed and
// CODEX_HOME=dir appended — filter-then-append, not a blind append, so a value
// inherited from the user's shell does not shadow ours (macOS getenv returns
// the first match in environ; same problem handled by spawner.removeEnvKey,
// cli_adapter_codex.go:134).
func withCodexHome(env []string, dir string) []string {
	const prefix = "CODEX_HOME="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			continue
		}
		out = append(out, e)
	}
	return append(out, prefix+dir)
}
