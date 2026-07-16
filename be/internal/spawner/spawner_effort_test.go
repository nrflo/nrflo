package spawner

// Tests for the reasoning_effort override precedence (def override >
// AgentConfig-carried override > model row) and spawn-time re-validation
// implemented by resolveReasoningEffort (spawner_prepare.go), CLI-mode
// prepareSpawn cases. Mirrors the structure of spawner_agentconfig_test.go's
// precedence tests. API-mode cases and direct resolveReasoningEffort
// precedence cases live in spawner_effort_api_test.go (300-line file cap).

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// insertCLIAgentDefWithEffort inserts a cli_interactive agent_definitions row
// with an optional reasoning_effort override (empty string = no override,
// i.e. column left NULL).
func insertCLIAgentDefWithEffort(t *testing.T, env *contextSaveTestEnv, agentID, model, effort string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var err error
	if effort == "" {
		_, err = env.database.Exec(
			`INSERT INTO agent_definitions
				(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, created_at, updated_at)
			VALUES (?, ?, 'feature', ?, 20, '# prompt', 'cli_interactive', 'agent_finished', ?, ?)`,
			agentID, env.projectID, model, now, now,
		)
	} else {
		_, err = env.database.Exec(
			`INSERT INTO agent_definitions
				(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, reasoning_effort, created_at, updated_at)
			VALUES (?, ?, 'feature', ?, 20, '# prompt', 'cli_interactive', 'agent_finished', ?, ?, ?)`,
			agentID, env.projectID, model, effort, now, now,
		)
	}
	if err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

func effortSpawner(env *contextSaveTestEnv, modelConfigs map[string]ModelConfig, agents map[string]AgentConfig) *Spawner {
	return New(Config{
		DataPath:     env.dbPath,
		Pool:         db.WrapAsPool(env.database),
		Clock:        clock.Real(),
		AgentSvc:     &noopAgentSvc{},
		ModelConfigs: modelConfigs,
		Agents:       agents,
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})
}

// TestPrepareSpawn_CLI_ReasoningEffort_DefOverrideReachesSpawnOptions
// verifies a legal def-level override reaches SpawnOptions.ReasoningEffort —
// the field the claude adapter renders as `--effort <level>`
// (cli_adapter_claude.go:65-67) and the codex app-server backend reads for
// its turn/start `effort` param (codex_appserver_backend.go:74-76).
func TestPrepareSpawn_CLI_ReasoningEffort_DefOverrideReachesSpawnOptions(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertCLIAgentDefWithEffort(t, env, "impl", "opus48", "xhigh")

	sp := effortSpawner(env, map[string]ModelConfig{
		"opus48": {CLIType: "claude", MappedModel: "claude-opus-4-8", SupportedEfforts: []string{"low", "medium", "high", "xhigh", "max"}},
	}, nil)

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:opus48", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.opts.ReasoningEffort != "xhigh" {
		t.Errorf("SpawnOptions.ReasoningEffort = %q, want %q (def override)", prep.opts.ReasoningEffort, "xhigh")
	}
}

// TestPrepareSpawn_CLI_ReasoningEffort_CodexDefOverrideReachesSpawnOptions
// is the codex counterpart: a legal "ultra" override on a codex model
// reaches SpawnOptions.ReasoningEffort via the same CLI-mode tail (codex and
// claude share prepareSpawn's cli branch — only the MCP wiring diverges by
// adapter). This is the field codexAppServerBackend.Start reads into
// turnStartParams' `effort` instead of falling back to
// CodexAdapter.GetReasoningEffort's default.
func TestPrepareSpawn_CLI_ReasoningEffort_CodexDefOverrideReachesSpawnOptions(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertCLIAgentDefWithEffort(t, env, "impl", "codexsol", "ultra")

	sp := effortSpawner(env, map[string]ModelConfig{
		"codexsol": {CLIType: "codex", MappedModel: "gpt-5.6-sol", SupportedEfforts: []string{"low", "medium", "high", "ultra"}},
	}, nil)

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "codex:codexsol", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.opts.ReasoningEffort != "ultra" {
		t.Errorf("SpawnOptions.ReasoningEffort = %q, want %q (codex def override)", prep.opts.ReasoningEffort, "ultra")
	}
}

// TestPrepareSpawn_CLI_ReasoningEffort_RowFallbackWhenNoOverride verifies
// that with no def-level override and no AgentConfig override, the model
// row's own ReasoningEffort is used.
func TestPrepareSpawn_CLI_ReasoningEffort_RowFallbackWhenNoOverride(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertCLIAgentDefWithEffort(t, env, "impl", "sonnet", "")

	sp := effortSpawner(env, map[string]ModelConfig{
		"sonnet": {CLIType: "claude", MappedModel: "claude-sonnet-5", ReasoningEffort: "medium", SupportedEfforts: []string{"low", "medium", "high", "xhigh"}},
	}, nil)

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.opts.ReasoningEffort != "medium" {
		t.Errorf("SpawnOptions.ReasoningEffort = %q, want %q (model row default)", prep.opts.ReasoningEffort, "medium")
	}
}

// TestPrepareSpawn_CLI_ReasoningEffort_IllegalOverrideFailsSpawn verifies a
// def-level override that has become illegal for the current model row (e.g.
// the model was swapped underneath a stale def) fails the spawn outright
// rather than silently dropping the override or spawning with an invalid
// flag.
func TestPrepareSpawn_CLI_ReasoningEffort_IllegalOverrideFailsSpawn(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// "xhigh" is not in this model row's supported_efforts, so the override is illegal.
	insertCLIAgentDefWithEffort(t, env, "impl", "haiku", "xhigh")

	sp := effortSpawner(env, map[string]ModelConfig{
		"haiku": {CLIType: "claude", MappedModel: "haiku", SupportedEfforts: []string{"low", "medium", "high"}},
	}, nil)

	_, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:haiku", "impl", env.wfiID)
	if err == nil {
		t.Fatal("prepareSpawn() = nil error; want error for an xhigh override on a haiku model row")
	}
	if !strings.Contains(err.Error(), "xhigh") {
		t.Errorf("error = %q, want it to name the invalid effort", err.Error())
	}
}
