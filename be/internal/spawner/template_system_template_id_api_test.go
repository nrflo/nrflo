package spawner

// prepareSpawn tests for api mode (Conversation.System) and the api-via-cli
// hybrid (--system-prompt-file, api semantics). Shared helpers live in
// template_system_template_id_test.go.

import (
	"context"
	"os"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestPrepareSpawn_APIMode_SystemTemplateIDOverridesDefault verifies an api
// agent def with system_template_id wins over the api-system-prompt default.
func TestPrepareSpawn_APIMode_SystemTemplateIDOverridesDefault(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAgentDefWithSystemTemplate(t, env, "impl", "sonnet-5", "api", "tier-t1-executor")

	prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

	want := mustRenderInjectable(t, env, "tier-t1-executor")
	if !strings.HasPrefix(prep.apiSystem, want) {
		t.Errorf("prep.apiSystem = %q, want prefix %q", prep.apiSystem, want)
	}
	if strings.HasPrefix(prep.apiSystem, defaultAPISystemPrompt) {
		t.Errorf("prep.apiSystem still starts with the worker default %q; want the def template to win", defaultAPISystemPrompt)
	}
}

// TestPrepareSpawn_APIMode_EmptySystemTemplateID_ByteIdenticalToDefault is the
// explicit regression companion to TestPrepareSpawn_APIMode_SystemPromptSeedByteIdentity:
// an agent def row that carries the system_template_id column (as every row
// does post-migration) but leaves it empty must still resolve to the
// unmodified api-system-prompt default.
func TestPrepareSpawn_APIMode_EmptySystemTemplateID_ByteIdenticalToDefault(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAgentDefWithSystemTemplate(t, env, "impl", "sonnet-5", "api", "")

	prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

	if !strings.HasPrefix(prep.apiSystem, defaultAPISystemPrompt) {
		t.Errorf("prep.apiSystem = %q, want prefix %q (byte-identical fallback)", prep.apiSystem, defaultAPISystemPrompt)
	}
}

// TestPrepareSpawn_APIViaCLI_SystemTemplateIDOverridesDefault verifies the
// api-via-cli hybrid path writes the rendered def template to the
// --system-prompt-file temp file instead of defaultAPISystemPrompt.
func TestPrepareSpawn_APIViaCLI_SystemTemplateIDOverridesDefault(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAgentDefWithSystemTemplate(t, env, "impl", "sonnet-5", "api", "tier-t0-decider")

	sp := New(Config{
		DataPath:  env.dbPath,
		Pool:      db.WrapAsPool(env.database),
		Clock:     clock.Real(),
		APIMode:   true,
		APIViaCLI: true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", APIModel: "claude-sonnet-4-6", APIContext: 200000},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
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
	if prep.systemPromptOverrideFile != "" {
		t.Cleanup(func() { os.Remove(prep.systemPromptOverrideFile) })
	}

	if prep.systemPromptOverrideFile == "" {
		t.Fatal("systemPromptOverrideFile is empty; want the rendered def template written")
	}
	got, readErr := os.ReadFile(prep.systemPromptOverrideFile)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	want := mustRenderInjectable(t, env, "tier-t0-decider")
	if string(got) != want {
		t.Errorf("system prompt file content = %q, want rendered tier-t0-decider %q", string(got), want)
	}
}
