package console

import (
	"fmt"
	"os"

	"be/internal/spawner"
)

// claudeDriver launches the native `claude` CLI as a human console session.
//
// This is a HUMAN session, not a managed spawner one: the cc96eed6
// managed-session boundary (--dangerously-skip-permissions,
// --disallowedTools denying native delegation, a safety-hook --settings
// injection) deliberately does NOT apply here. The user is at a real
// terminal driving their own CLI; native delegation (Agent/Task) is allowed
// and permission prompts stay in force. --strict-mcp-config is also never
// passed, so the user's own configured MCP servers survive alongside nrflo's.
type claudeDriver struct{}

func (d *claudeDriver) Name() string { return "claude" }

func (d *claudeDriver) Probe() error {
	if _, err := lookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not found on PATH — install it from https://docs.claude.com/claude-code: %w", err)
	}
	return nil
}

func (d *claudeDriver) Prepare(in LaunchInput) (LaunchSpec, func(), error) {
	dir, err := os.MkdirTemp("", "nrflo-console-claude-*")
	if err != nil {
		return LaunchSpec{}, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	cfgPath, err := spawner.WriteConsoleClaudeMCPConfig(dir, in.NrfloPath, []string{"agent", "mcp-external"}, bridgeEnv(in))
	if err != nil {
		cleanup()
		return LaunchSpec{}, func() {}, err
	}

	model := in.MappedModel
	if model == "" && in.RawModel != "" {
		model = (&spawner.ClaudeAdapter{}).MapModel(in.RawModel)
	}
	argv := []string{"claude", "--mcp-config", cfgPath}
	if model != "" {
		argv = append(argv, "--model", model)
	}
	// Same registry-sourced flags the managed path passes
	// (cli_adapter_claude.go:69-73). ClaudeAdapter has no GetReasoningEffort
	// alias table — for claude, effort exists only as a cli_models row value —
	// so there is nothing to fall back to when the registry has no row.
	if in.ReasoningEffort != "" {
		argv = append(argv, "--effort", in.ReasoningEffort)
	}
	if in.FallbackModels != "" {
		argv = append(argv, "--fallback-model", in.FallbackModels)
	}

	return LaunchSpec{
		Argv: argv,
		Env:  os.Environ(),
		Dir:  in.WorkDir,
	}, cleanup, nil
}
