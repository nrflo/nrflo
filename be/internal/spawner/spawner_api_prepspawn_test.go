package spawner

// Tests for api-mode prepareSpawn: provider selection, missing model, credential
// errors, and reasoning effort propagation. Each case exercises the path from
// Config.APIModelConfigs → BuildAPIProvider → prepResult fields.

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

// TestPrepareSpawn_APIMode_AnthropicProvider verifies that when an api_models row
// has provider="anthropic", BuildAPIProvider is called with providerName="anthropic".
func TestPrepareSpawn_APIMode_AnthropicProvider(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet")

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
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6", ContextLength: 200000},
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
	}, "claude:sonnet", "impl", env.wfiID)
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

// TestPrepareSpawn_APIMode_OpenAIProvider verifies that when an api_models row
// has provider="openai", BuildAPIProvider is called with providerName="openai".
func TestPrepareSpawn_APIMode_OpenAIProvider(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "codex-impl", "gpt53_codex_high")

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
		APIModelConfigs: map[string]APIModelConfig{
			"gpt53_codex_high": {Provider: "openai", MappedModel: "gpt-5.3-codex", ReasoningEffort: "high", ContextLength: 200000, SupportedEfforts: []string{"low", "medium", "high"}},
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
	}, "claude:gpt53_codex_high", "codex-impl", env.wfiID)
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
// is absent from APIModelConfigs, prepareSpawn returns a descriptive error and
// does NOT fall through to CLI mode.
func TestPrepareSpawn_APIMode_MissingModelInConfigs(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		// APIModelConfigs deliberately empty — "sonnet" not present.
		APIModelConfigs: map[string]APIModelConfig{},
		AgentSvc:        &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet", "impl", env.wfiID)
	if err == nil {
		t.Fatal("prepareSpawn() returned nil error; want error for missing model in APIModelConfigs")
	}
	if !strings.Contains(err.Error(), "not found in api_models") {
		t.Errorf("error = %q; want contains 'not found in api_models'", err.Error())
	}
}

// TestPrepareSpawn_APIMode_BuildProviderError verifies that a credential error from
// BuildAPIProvider propagates immediately without fallback.
func TestPrepareSpawn_APIMode_BuildProviderError(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet")

	credErr := errors.New("ANTHROPIC_API_KEY not found in env or project vars")
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return nil, credErr
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6", ContextLength: 200000},
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
	}, "claude:sonnet", "impl", env.wfiID)
	if err == nil {
		t.Fatal("prepareSpawn() returned nil error; want credential error from BuildAPIProvider")
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("error = %q; want error mentioning ANTHROPIC_API_KEY", err.Error())
	}
}

// TestPrepareSpawn_APIMode_ReasoningEffortPropagates verifies that the ReasoningEffort
// field from the APIModelConfig row is stored in prep.apiReasoningEffort.
func TestPrepareSpawn_APIMode_ReasoningEffortPropagates(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {
				Provider:         "anthropic",
				MappedModel:      "claude-sonnet-4-6",
				ContextLength:    200000,
				ReasoningEffort:  "high",
				SupportedEfforts: []string{"low", "medium", "high", "xhigh"},
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
	}, "claude:sonnet", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}

	if prep.apiReasoningEffort != "high" {
		t.Errorf("prep.apiReasoningEffort = %q, want %q", prep.apiReasoningEffort, "high")
	}
}

// TestPrepareSpawn_APIMode_MappedModelFromConfig verifies that prep.apiModelID
// is set to APIModelConfig.MappedModel (not the raw nrflo model ID).
func TestPrepareSpawn_APIMode_MappedModelFromConfig(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6-20251001", ContextLength: 200000},
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
	}, "claude:sonnet", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}

	if prep.apiModelID != "claude-sonnet-4-6-20251001" {
		t.Errorf("prep.apiModelID = %q, want %q", prep.apiModelID, "claude-sonnet-4-6-20251001")
	}
}
