package orchestrator

import (
	"strings"
	"testing"

	"be/internal/service"
	"be/internal/spawner"
)

// TestBuildInteractiveLaunch_SystemPromptOverride_EmitsBothFlags verifies that when
// the global claude_system_prompt_override_enabled setting is on and the L0 model is a
// claude model, buildInteractiveLaunch in interactive (non-plan) mode emits both
// --system-prompt-file and --append-system-prompt-file.
func TestBuildInteractiveLaunch_SystemPromptOverride_EmitsBothFlags(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PTA-OVR", "buildInteractiveLaunch override test")
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

	launch, _, _, cleanup, err := env.orch.buildInteractiveLaunch(
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
		t.Fatalf("buildInteractiveLaunch() error: %v", err)
	}
	t.Cleanup(cleanup)

	argsStr := strings.Join(launch.Args, " ")
	if !strings.Contains(argsStr, "--system-prompt-file") {
		t.Errorf("args should contain --system-prompt-file when the override setting is on; got: %v", launch.Args)
	}
	if !strings.Contains(argsStr, "--append-system-prompt-file") {
		t.Errorf("args should contain --append-system-prompt-file; got: %v", launch.Args)
	}
	// Override must precede the append flag
	overrideIdx := strings.Index(argsStr, "--system-prompt-file")
	appendIdx := strings.Index(argsStr, "--append-system-prompt-file")
	if overrideIdx >= appendIdx {
		t.Errorf("--system-prompt-file (%d) should precede --append-system-prompt-file (%d): %v",
			overrideIdx, appendIdx, launch.Args)
	}
}

// TestBuildInteractiveLaunch_SystemPromptOverride_ToggleFalse verifies that when
// the global claude_system_prompt_override_enabled setting is off (stored false),
// --system-prompt-file is NOT emitted but --append-system-prompt-file is still present.
func TestBuildInteractiveLaunch_SystemPromptOverride_ToggleFalse(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PTA-NOV", "buildInteractiveLaunch no-override test")
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

	launch, _, _, cleanup, err := env.orch.buildInteractiveLaunch(
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
		t.Fatalf("buildInteractiveLaunch() error: %v", err)
	}
	t.Cleanup(cleanup)

	argsStr := strings.Join(launch.Args, " ")
	// --system-prompt-file must NOT appear as a standalone flag
	// (--append-system-prompt-file contains the substring, so check for the bare flag)
	for i, p := range launch.Args {
		if p == "--system-prompt-file" {
			t.Errorf("args[%d]=%q: --system-prompt-file should not be emitted when the override setting is off; args: %v", i, p, launch.Args)
		}
	}
	if !strings.Contains(argsStr, "--append-system-prompt-file") {
		t.Errorf("args should still contain --append-system-prompt-file; got: %v", launch.Args)
	}
}

// TestBuildInteractiveLaunch_PlanMode_NoOverrideFile verifies that in plan mode,
// --system-prompt-file is never emitted (plan mode skips template loading entirely),
// even with the global claude_system_prompt_override_enabled setting on.
func TestBuildInteractiveLaunch_PlanMode_NoOverrideFile(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PTA-PM", "buildInteractiveLaunch plan mode test")
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

	launch, _, _, cleanup, err := env.orch.buildInteractiveLaunch(
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
		t.Fatalf("buildInteractiveLaunch() error: %v", err)
	}
	t.Cleanup(cleanup)

	for i, p := range launch.Args {
		if p == "--system-prompt-file" {
			t.Errorf("args[%d]=%q: plan mode should never emit --system-prompt-file; args: %v", i, p, launch.Args)
		}
	}
}
