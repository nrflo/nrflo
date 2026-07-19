package spawner

// prepareSpawn tests for codex CLI delivery (prompt-body prepend, since codex
// has no --system-prompt-file flag). Shared helpers live in
// template_system_template_id_test.go.

import (
	"context"
	"os"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// TestPrepareSpawn_CodexSystemTemplateID_PrependedToPromptBody verifies a
// codex cli_interactive spawn with system_template_id prepends the rendered
// template ahead of the system-prompt-suffix injectable, both ahead of the
// main prompt body — since codex has no --system-prompt-file flag.
func TestPrepareSpawn_CodexSystemTemplateID_PrependedToPromptBody(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAgentDefWithSystemTemplate(t, env, "impl", "gpt-5-codex", "cli_interactive", "tier-t2-extractor")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
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
	}, "codex:gpt-5-codex", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	_ = proc

	override := mustRenderInjectable(t, env, "tier-t2-extractor")
	suffix := mustRenderInjectable(t, env, "system-prompt-suffix")
	want := override + "\n\n" + suffix + "\n\n" + "# prompt"
	if prep.prompt != want {
		t.Errorf("prep.prompt = %q, want override+suffix+body prefix chain %q", prep.prompt, want)
	}
}

// TestPrepareSpawn_CodexEmptySystemTemplateID_ByteIdenticalToSuffixOnly
// verifies a codex spawn with an empty system_template_id produces exactly
// the pre-feature prompt body: suffix + "\n\n" + prompt, with no override
// text prepended.
func TestPrepareSpawn_CodexEmptySystemTemplateID_ByteIdenticalToSuffixOnly(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAgentDefWithSystemTemplate(t, env, "impl", "gpt-5-codex", "cli_interactive", "")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
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
	}, "codex:gpt-5-codex", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	_ = proc

	suffix := mustRenderInjectable(t, env, "system-prompt-suffix")
	want := suffix + "\n\n" + "# prompt"
	if prep.prompt != want {
		t.Errorf("prep.prompt = %q, want byte-identical suffix-only prefix %q", prep.prompt, want)
	}
}
