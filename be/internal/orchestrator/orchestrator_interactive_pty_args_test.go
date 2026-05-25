package orchestrator

import (
	"strings"
	"testing"

	"be/internal/service"
	"be/internal/spawner"
)

// TestBuildInteractivePtyArgs_SystemPromptOverride_EmitsBothFlags verifies that when
// the global claude_system_prompt_override_enabled setting is on and the L0 model is a
// claude model, buildInteractivePtyArgs in interactive (non-plan) mode emits both
// --system-prompt-file and --append-system-prompt-file.
func TestBuildInteractivePtyArgs_SystemPromptOverride_EmitsBothFlags(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PTA-OVR", "buildInteractivePtyArgs override test")
	wfiID := env.initWorkflow(t, "TKT-PTA-OVR")
	wi := env.getWorkflowInstance(t, wfiID)

	if err := env.pool.SetConfig("claude_system_prompt_override_enabled", "true"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}},
	}
	agents := map[string]spawner.AgentConfig{
		"analyzer": {Model: "opus_4_7"},
	}
	modelConfigs := map[string]spawner.ModelConfig{
		"opus_4_7": {CLIType: "claude", MappedModel: "claude-opus-4-7"},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-PTA-OVR",
		WorkflowName: "test",
		Interactive:  true,
	}

	args, err := env.orch.buildInteractivePtyArgs(
		req, wi, "test-session-ovr", "opus_4_7",
		svcWf,
		map[string]spawner.WorkflowDef{},
		agents,
		env.pool,
		"",
		modelConfigs,
		map[string]spawner.APIModelConfig{},
		"",
	)
	if err != nil {
		t.Fatalf("buildInteractivePtyArgs() error: %v", err)
	}

	argsStr := strings.Join(args, " ")
	if !strings.Contains(argsStr, "--system-prompt-file") {
		t.Errorf("args should contain --system-prompt-file when the override setting is on; got: %v", args)
	}
	if !strings.Contains(argsStr, "--append-system-prompt-file") {
		t.Errorf("args should contain --append-system-prompt-file; got: %v", args)
	}
	// Override must precede the append flag
	overrideIdx := strings.Index(argsStr, "--system-prompt-file")
	appendIdx := strings.Index(argsStr, "--append-system-prompt-file")
	if overrideIdx >= appendIdx {
		t.Errorf("--system-prompt-file (%d) should precede --append-system-prompt-file (%d): %v",
			overrideIdx, appendIdx, args)
	}
}

// TestBuildInteractivePtyArgs_SystemPromptOverride_ToggleFalse verifies that when
// the global claude_system_prompt_override_enabled setting is off (stored false),
// --system-prompt-file is NOT emitted but --append-system-prompt-file is still present.
func TestBuildInteractivePtyArgs_SystemPromptOverride_ToggleFalse(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PTA-NOV", "buildInteractivePtyArgs no-override test")
	wfiID := env.initWorkflow(t, "TKT-PTA-NOV")
	wi := env.getWorkflowInstance(t, wfiID)

	if err := env.pool.SetConfig("claude_system_prompt_override_enabled", "false"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}},
	}
	agents := map[string]spawner.AgentConfig{
		"analyzer": {Model: "opus_4_7"},
	}
	modelConfigs := map[string]spawner.ModelConfig{
		"opus_4_7": {CLIType: "claude", MappedModel: "claude-opus-4-7"},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-PTA-NOV",
		WorkflowName: "test",
		Interactive:  true,
	}

	args, err := env.orch.buildInteractivePtyArgs(
		req, wi, "test-session-nov", "opus_4_7",
		svcWf,
		map[string]spawner.WorkflowDef{},
		agents,
		env.pool,
		"",
		modelConfigs,
		map[string]spawner.APIModelConfig{},
		"",
	)
	if err != nil {
		t.Fatalf("buildInteractivePtyArgs() error: %v", err)
	}

	argsStr := strings.Join(args, " ")
	// --system-prompt-file must NOT appear as a standalone flag
	// (--append-system-prompt-file contains the substring, so check for the bare flag)
	parts := args
	for i, p := range parts {
		if p == "--system-prompt-file" {
			t.Errorf("args[%d]=%q: --system-prompt-file should not be emitted when the override setting is off; args: %v", i, p, args)
		}
	}
	if !strings.Contains(argsStr, "--append-system-prompt-file") {
		t.Errorf("args should still contain --append-system-prompt-file; got: %v", args)
	}
}

// TestBuildInteractivePtyArgs_PlanMode_NoOverrideFile verifies that in plan mode,
// --system-prompt-file is never emitted (plan mode skips template loading entirely),
// even with the global claude_system_prompt_override_enabled setting on.
func TestBuildInteractivePtyArgs_PlanMode_NoOverrideFile(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PTA-PM", "buildInteractivePtyArgs plan mode test")
	wfiID := env.initWorkflow(t, "TKT-PTA-PM")
	wi := env.getWorkflowInstance(t, wfiID)

	if err := env.pool.SetConfig("claude_system_prompt_override_enabled", "true"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}},
	}
	modelConfigs := map[string]spawner.ModelConfig{
		"opus_4_7": {CLIType: "claude", MappedModel: "claude-opus-4-7"},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-PTA-PM",
		WorkflowName: "test",
		PlanMode:     true,
	}

	args, err := env.orch.buildInteractivePtyArgs(
		req, wi, "test-session-pm", "opus_4_7",
		svcWf,
		map[string]spawner.WorkflowDef{},
		map[string]spawner.AgentConfig{},
		env.pool,
		"",
		modelConfigs,
		map[string]spawner.APIModelConfig{},
		"",
	)
	if err != nil {
		t.Fatalf("buildInteractivePtyArgs() error: %v", err)
	}

	for i, p := range args {
		if p == "--system-prompt-file" {
			t.Errorf("args[%d]=%q: plan mode should never emit --system-prompt-file; args: %v", i, p, args)
		}
	}
}
