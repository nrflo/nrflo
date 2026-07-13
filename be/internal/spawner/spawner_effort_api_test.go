package spawner

// API-mode reasoning_effort prep-result cases and direct
// resolveReasoningEffort precedence cases. Split out of
// spawner_effort_test.go to stay under the 300-line file cap; see that
// file's doc comment for the shared helpers (effortSpawner) used here.

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestPrepareSpawn_API_ReasoningEffort_DefOverrideReachesPrepResult verifies
// a legal def-level override reaches prep.apiReasoningEffort for api-mode
// spawns.
func TestPrepareSpawn_API_ReasoningEffort_DefOverrideReachesPrepResult(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, reasoning_effort, created_at, updated_at)
		VALUES ('impl', ?, 'feature', 'opus48', 20, '# prompt', 'api', 'agent_finished', 'xhigh', ?, ?)`,
		env.projectID, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		AgentSvc: &noopAgentSvc{},
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"opus48": {Provider: "anthropic", MappedModel: "claude-opus-4-8", ContextLength: 200000},
		},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:opus48", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.apiReasoningEffort != "xhigh" {
		t.Errorf("prep.apiReasoningEffort = %q, want %q (def override)", prep.apiReasoningEffort, "xhigh")
	}
}

// TestPrepareSpawn_API_ReasoningEffort_IllegalOverrideFailsSpawn verifies an
// api-mode def override illegal for its provider (xhigh is anthropic-only)
// fails the spawn.
func TestPrepareSpawn_API_ReasoningEffort_IllegalOverrideFailsSpawn(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, reasoning_effort, created_at, updated_at)
		VALUES ('impl', ?, 'feature', 'gpt56', 20, '# prompt', 'api', 'agent_finished', 'xhigh', ?, ?)`,
		env.projectID, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		AgentSvc: &noopAgentSvc{},
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"gpt56": {Provider: "openai", MappedModel: "gpt-5.6-sol", ContextLength: 372000},
		},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:gpt56", "impl", env.wfiID)
	if err == nil {
		t.Fatal("prepareSpawn() = nil error; want error for an xhigh override on an openai provider")
	}
	if !strings.Contains(err.Error(), "api mode") {
		t.Errorf("error = %q, want the api-mode-prefixed error", err.Error())
	}
}

// TestResolveReasoningEffort_Precedence_DefOverrideWinsOverAgentConfig
// verifies resolveReasoningEffort directly: when both an agentDef override
// and a Config.Agents override are present, the def wins.
func TestResolveReasoningEffort_Precedence_DefOverrideWinsOverAgentConfig(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	defEffort := "xhigh"
	cfgEffort := "medium"
	sp := effortSpawner(env, nil, map[string]AgentConfig{
		"impl": {ReasoningEffort: &cfgEffort},
	})

	got := sp.resolveReasoningEffort(&model.AgentDefinition{ReasoningEffort: &defEffort}, "impl", "low")
	if got != "xhigh" {
		t.Errorf("resolveReasoningEffort() = %q, want %q (def override wins)", got, "xhigh")
	}
}

// TestResolveReasoningEffort_Precedence_AgentConfigWinsWhenDefNil verifies
// the load-bearing case (orchestrator/plan_boundary.go, spawner/CLAUDE.md):
// a global workflow's agent def is invisible to loadAgentDefinition
// (agentDef == nil), so a materialized plan node's override must travel via
// Config.Agents[agentType].ReasoningEffort and win over the model row.
func TestResolveReasoningEffort_Precedence_AgentConfigWinsWhenDefNil(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	cfgEffort := "medium"
	sp := effortSpawner(env, nil, map[string]AgentConfig{
		"impl": {ReasoningEffort: &cfgEffort},
	})

	got := sp.resolveReasoningEffort(nil, "impl", "low")
	if got != "medium" {
		t.Errorf("resolveReasoningEffort() = %q, want %q (AgentConfig override, agentDef nil)", got, "medium")
	}
}

// TestResolveReasoningEffort_Precedence_RowWinsWhenNoOverridesPresent
// verifies the row effort is the final fallback when neither the def nor
// Config.Agents carries an override.
func TestResolveReasoningEffort_Precedence_RowWinsWhenNoOverridesPresent(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	sp := effortSpawner(env, nil, map[string]AgentConfig{
		"impl": {},
	})

	got := sp.resolveReasoningEffort(nil, "impl", "low")
	if got != "low" {
		t.Errorf("resolveReasoningEffort() = %q, want %q (model row fallback)", got, "low")
	}
}
