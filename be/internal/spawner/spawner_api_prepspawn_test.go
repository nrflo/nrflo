package spawner

// Tests for api-mode prepareSpawn: provider selection, missing model, credential
// errors, and reasoning effort propagation. Each case exercises the path from
// Config.ModelConfigs → BuildAPIProvider → prepResult fields.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// insertAPIAgentDef inserts an agent_definition row with execution_mode=api and
// the given model name into the test DB.
func insertAPIAgentDef(t *testing.T, env *contextSaveTestEnv, agentID, model string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', 'api', 'agent_finished', ?, ?)`,
		agentID, env.projectID, model, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

// TestPrepareSpawn_APIMode_AnthropicProvider verifies that when a model row
// has provider="anthropic", BuildAPIProvider is called with providerName="anthropic".
func TestPrepareSpawn_APIMode_AnthropicProvider(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	var capturedProvider string
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, providerName, _ string) (provider.Provider, error) {
			capturedProvider = providerName
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

	if capturedProvider != "anthropic" {
		t.Errorf("BuildAPIProvider called with providerName=%q, want %q", capturedProvider, "anthropic")
	}
	if prep.apiProvider == nil {
		t.Error("prep.apiProvider is nil; want non-nil resolved provider")
	}
}

// TestPrepareSpawn_APIMode_OpenAIProvider verifies that when a model row
// has provider="openai", BuildAPIProvider is called with providerName="openai".
func TestPrepareSpawn_APIMode_OpenAIProvider(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "codex-impl", "gpt-5.3-codex")

	var capturedProvider string
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, providerName, _ string) (provider.Provider, error) {
			capturedProvider = providerName
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"gpt-5.3-codex": {Provider: "openai", APIModel: "gpt-5.3-codex", DefaultEffort: "high", APIContext: 200000, APIEfforts: []string{"low", "medium", "high"}},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "codex-impl", Agent: "codex-impl", Layer: 0}}},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "codex-impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "codex:gpt-5.3-codex", "codex-impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}

	if capturedProvider != "openai" {
		t.Errorf("BuildAPIProvider called with providerName=%q, want %q", capturedProvider, "openai")
	}
	if prep.apiProvider == nil {
		t.Error("prep.apiProvider is nil; want non-nil resolved provider")
	}
}

// TestPrepareSpawn_APIMode_MissingModelInConfigs verifies that when the model ID
// is absent from ModelConfigs, prepareSpawn returns a descriptive error and
// does NOT fall through to CLI mode.
func TestPrepareSpawn_APIMode_MissingModelInConfigs(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		// ModelConfigs deliberately empty — "sonnet-5" not present.
		ModelConfigs: map[string]ModelConfig{},
		AgentSvc:     &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err == nil {
		t.Fatal("prepareSpawn() returned nil error; want error for missing model in ModelConfigs")
	}
	if !strings.Contains(err.Error(), "not found in models") {
		t.Errorf("error = %q; want contains 'not found in models'", err.Error())
	}
}

// TestPrepareSpawn_APIMode_BuildProviderError verifies that a credential error from
// BuildAPIProvider propagates immediately without fallback.
func TestPrepareSpawn_APIMode_BuildProviderError(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	credErr := errors.New("ANTHROPIC_API_KEY not found in env or project vars")
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return nil, credErr
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", APIModel: "claude-sonnet-4-6", APIContext: 200000},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err == nil {
		t.Fatal("prepareSpawn() returned nil error; want credential error from BuildAPIProvider")
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("error = %q; want error mentioning ANTHROPIC_API_KEY", err.Error())
	}
}

// TestPrepareSpawn_APIMode_ReasoningEffortPropagates verifies that the ReasoningEffort
// field from the ModelConfig row is stored in prep.apiReasoningEffort.
func TestPrepareSpawn_APIMode_ReasoningEffortPropagates(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {
				Provider:      "anthropic",
				APIModel:      "claude-sonnet-4-6",
				APIContext:    200000,
				DefaultEffort: "high",
				APIEfforts:    []string{"low", "medium", "high", "xhigh"},
			},
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

	if prep.apiReasoningEffort != "high" {
		t.Errorf("prep.apiReasoningEffort = %q, want %q", prep.apiReasoningEffort, "high")
	}
}

// TestPrepareSpawn_APIMode_APIModelFromConfig verifies that prep.apiModelID
// is set to ModelConfig.APIModel (not the raw nrflo model ID).
func TestPrepareSpawn_APIMode_APIModelFromConfig(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", APIModel: "claude-sonnet-4-6-20251001", APIContext: 200000},
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

	if prep.apiModelID != "claude-sonnet-4-6-20251001" {
		t.Errorf("prep.apiModelID = %q, want %q", prep.apiModelID, "claude-sonnet-4-6-20251001")
	}
}
