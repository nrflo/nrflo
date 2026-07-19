package spawner

// prepareSpawn tests for claude CLI delivery (--system-prompt-file). Shared
// helpers (insertAgentDefWithSystemTemplate, mustRenderInjectable) live in
// template_system_template_id_test.go.

import (
	"context"
	"os"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// TestPrepareSpawn_ClaudeSystemTemplateID_WritesOverrideFile verifies a claude
// cli_interactive spawn with system_template_id writes the rendered template
// (not the "system-prompt" gate injectable) to systemPromptOverrideFile.
func TestPrepareSpawn_ClaudeSystemTemplateID_WritesOverrideFile(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAgentDefWithSystemTemplate(t, env, "impl", "sonnet-5", "cli_interactive", "tier-t2-extractor")

	// Gate on, to prove def wins over it too.
	pool := db.WrapAsPool(env.database)
	if err := pool.SetConfig("claude_system_prompt_override_enabled", "true"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     pool,
		Clock:    clock.Real(),
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	proc, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile != "" {
		t.Cleanup(func() { os.Remove(prep.suffixFile) })
	}
	if prep.systemPromptOverrideFile != "" {
		t.Cleanup(func() { os.Remove(prep.systemPromptOverrideFile) })
	}
	_ = proc

	if prep.systemPromptOverrideFile == "" {
		t.Fatal("systemPromptOverrideFile is empty; want a file containing the rendered def template")
	}
	got, readErr := os.ReadFile(prep.systemPromptOverrideFile)
	if readErr != nil {
		t.Fatalf("ReadFile(systemPromptOverrideFile): %v", readErr)
	}
	want := mustRenderInjectable(t, env, "tier-t2-extractor")
	if string(got) != want {
		t.Errorf("systemPromptOverrideFile content = %q, want rendered tier-t2-extractor %q", string(got), want)
	}
}

// TestPrepareSpawn_ClaudeEmptySystemTemplateID_ByteIdenticalToGate verifies
// that with an empty system_template_id, the override file (when the gate is
// on) contains exactly the same content systemPromptOverrideFor would
// produce, and no file is written when the gate is off — i.e. unchanged from
// pre-feature behavior.
func TestPrepareSpawn_ClaudeEmptySystemTemplateID_ByteIdenticalToGate(t *testing.T) {
	t.Parallel()

	for _, gateOn := range []bool{true, false} {
		t.Run(map[bool]string{true: "gate=on", false: "gate=off"}[gateOn], func(t *testing.T) {
			t.Parallel()
			ensureTmpNrfloDir(t)
			env := setupContextSaveTestEnv(t)
			defer env.cleanup()
			insertAgentDefWithSystemTemplate(t, env, "impl", "sonnet-5", "cli_interactive", "")

			pool := db.WrapAsPool(env.database)
			if err := pool.SetConfig("claude_system_prompt_override_enabled", map[bool]string{true: "true", false: "false"}[gateOn]); err != nil {
				t.Fatalf("SetConfig: %v", err)
			}

			modelConfigs := map[string]ModelConfig{"sonnet-5": {Provider: "anthropic", CLIModel: "raw"}}
			sp := New(Config{
				DataPath:     env.dbPath,
				Pool:         pool,
				Clock:        clock.Real(),
				ModelConfigs: modelConfigs,
				AgentSvc:     &noopAgentSvc{},
				Workflows: map[string]WorkflowDef{
					"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
				},
			})
			want := sp.systemPromptOverrideFor("sonnet-5", nil)

			proc, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
				AgentType:          "impl",
				ProjectID:          env.projectID,
				WorkflowName:       "feature",
				WorkflowInstanceID: env.wfiID,
			}, "claude:sonnet-5", "impl", env.wfiID)
			if err != nil {
				t.Fatalf("prepareSpawn() error: %v", err)
			}
			if prep.promptFile != "" {
				t.Cleanup(func() { os.Remove(prep.promptFile) })
			}
			if prep.suffixFile != "" {
				t.Cleanup(func() { os.Remove(prep.suffixFile) })
			}
			_ = proc

			if gateOn {
				if prep.systemPromptOverrideFile == "" {
					t.Fatal("gate on: want a non-empty systemPromptOverrideFile")
				}
				t.Cleanup(func() { os.Remove(prep.systemPromptOverrideFile) })
				got, readErr := os.ReadFile(prep.systemPromptOverrideFile)
				if readErr != nil {
					t.Fatalf("ReadFile: %v", readErr)
				}
				if string(got) != want {
					t.Errorf("override file content = %q, want byte-identical gate content %q", string(got), want)
				}
			} else if prep.systemPromptOverrideFile != "" {
				t.Errorf("gate off: systemPromptOverrideFile = %q, want empty", prep.systemPromptOverrideFile)
			}
		})
	}
}
