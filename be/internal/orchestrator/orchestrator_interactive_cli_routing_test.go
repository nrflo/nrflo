package orchestrator

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/service"
	"be/internal/spawner"
)

// TestBuildInteractiveLaunch_RoutesToL0ModelCLI is the ticket's headline
// guarantee: interactive start and plan mode both resolve the CLI from the L0
// agent's model through spawner.GetCLIAdapter, so a codex model launches codex
// and a claude model launches claude — same code path, no name-checks.
func TestBuildInteractiveLaunch_RoutesToL0ModelCLI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		model       string
		modelConfig spawner.ModelConfig
		planMode    bool
		wantCLI     string
		wantArgs    []string
		notWantArgs []string
	}{
		{
			name:        "claude model, interactive",
			model:       "opus_4_7",
			modelConfig: spawner.ModelConfig{CLIType: "claude", MappedModel: "claude-opus-4-7"},
			wantCLI:     "claude",
			wantArgs:    []string{"--model claude-opus-4-7", "--dangerously-skip-permissions"},
		},
		{
			name:        "codex model, interactive",
			model:       "codex_gpt55_high",
			modelConfig: spawner.ModelConfig{CLIType: "codex", MappedModel: "gpt-5.5", ReasoningEffort: "high"},
			wantCLI:     "codex",
			wantArgs:    []string{"--model gpt-5.5", "--dangerously-bypass-approvals-and-sandbox"},
			// Claude flags must never leak into a codex launch.
			notWantArgs: []string{"--session-id", "--dangerously-skip-permissions"},
		},
		{
			name:        "claude model, plan mode",
			model:       "opus_4_7",
			modelConfig: spawner.ModelConfig{CLIType: "claude", MappedModel: "claude-opus-4-7"},
			planMode:    true,
			wantCLI:     "claude",
			wantArgs:    []string{"--permission-mode plan"},
		},
		{
			name:        "codex model, plan mode",
			model:       "codex_gpt55_high",
			modelConfig: spawner.ModelConfig{CLIType: "codex", MappedModel: "gpt-5.5", ReasoningEffort: "high"},
			planMode:    true,
			wantCLI:     "codex",
			// Codex has no native plan permission mode: read-only sandbox stands in.
			wantArgs:    []string{"--sandbox read-only", "--ask-for-approval on-request"},
			notWantArgs: []string{"--permission-mode"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			ticket := "TKT-CLI-" + strings.ReplaceAll(tc.name, " ", "")
			env.createTicket(t, ticket, "cli routing test")
			wfiID := env.initWorkflow(t, ticket)
			wi := env.getWorkflowInstance(t, wfiID)

			svcWf := service.SpawnerWorkflowDef{
				Phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}},
			}
			agents := map[string]spawner.AgentConfig{"analyzer": {Model: tc.model}}
			modelConfigs := map[string]spawner.ModelConfig{tc.model: tc.modelConfig}

			req := RunRequest{
				ProjectID:    env.project,
				TicketID:     ticket,
				WorkflowName: "test",
				Interactive:  !tc.planMode,
				PlanMode:     tc.planMode,
			}

			launch, adapter, planFile, cleanup, err := env.orch.buildInteractiveLaunch(
				req, wi, "sess-cli-routing", tc.model,
				svcWf,
				map[string]spawner.WorkflowDef{},
				agents,
				env.pool,
				t.TempDir(),
				modelConfigs,
				map[string]spawner.APIModelConfig{},
				"",
			)
			if err != nil {
				t.Fatalf("buildInteractiveLaunch() error: %v", err)
			}
			t.Cleanup(cleanup)

			// exec.Command resolves a PATH lookup to an absolute path.
			if got := filepath.Base(launch.Command); got != tc.wantCLI {
				t.Errorf("launch.Command = %q, want the %s CLI", launch.Command, tc.wantCLI)
			}
			if adapter.Name() != tc.wantCLI {
				t.Errorf("adapter.Name() = %q, want %q", adapter.Name(), tc.wantCLI)
			}

			args := strings.Join(launch.Args, " ")
			for _, want := range tc.wantArgs {
				if !strings.Contains(args, want) {
					t.Errorf("launch args missing %q: %s", want, args)
				}
			}
			for _, notWant := range tc.notWantArgs {
				if strings.Contains(args, notWant) {
					t.Errorf("launch args must not contain %q: %s", notWant, args)
				}
			}

			// Plan mode allocates a plan file the adapter can capture from; the
			// interactive path has no plan to read.
			if tc.planMode && planFile == "" {
				t.Error("plan mode must allocate a plan file")
			}
			if !tc.planMode && planFile != "" {
				t.Errorf("interactive mode must not allocate a plan file, got %q", planFile)
			}
		})
	}
}
