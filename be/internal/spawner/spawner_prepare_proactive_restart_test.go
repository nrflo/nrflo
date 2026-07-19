package spawner

// Tests for prepareSpawn's proactive_restart_threshold_tokens resolution:
// proc.proactiveRestartThreshold (and prep.proactiveRestartThreshold) must
// reflect resolveProactiveRestartThreshold(agentDef, config
// proactive_restart_threshold_default) for EVERY execution mode, not just
// api — mirrors spawner_api_prepspawn_context_budget_test.go's shape for the
// context_budget_tokens column.

import (
	"context"
	"testing"
	"time"
)

// insertCLIAgentDefWithProactiveThreshold inserts a cli_interactive
// agent_definition row with an explicit proactive_restart_threshold_tokens
// value.
func insertCLIAgentDefWithProactiveThreshold(t *testing.T, env *contextSaveTestEnv, agentID string, threshold int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, proactive_restart_threshold_tokens, created_at, updated_at)
		VALUES (?, ?, 'feature', 'claude:sonnet-5', 20, '# prompt', 'cli_interactive', 'agent_finished', ?, ?, ?)`,
		agentID, env.projectID, threshold, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

// insertCLIAgentDefNoProactiveThreshold inserts a cli_interactive
// agent_definition row leaving proactive_restart_threshold_tokens NULL.
func insertCLIAgentDefNoProactiveThreshold(t *testing.T, env *contextSaveTestEnv, agentID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, created_at, updated_at)
		VALUES (?, ?, 'feature', 'claude:sonnet-5', 20, '# prompt', 'cli_interactive', 'agent_finished', ?, ?)`,
		agentID, env.projectID, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

func newCLIPrepSpawner(env *contextSaveTestEnv) *Spawner {
	return New(Config{
		DataPath: env.dbPath,
		Pool:     env.spawner.pool(),
		Clock:    env.spawner.config.Clock,
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})
}

// TestPrepareSpawn_CLIMode_ProactiveThreshold_DefOverridePropagates verifies
// a positive per-def override lands in proc.proactiveRestartThreshold (and
// prep.proactiveRestartThreshold) verbatim, for the cli_interactive
// execution mode (not just api).
func TestPrepareSpawn_CLIMode_ProactiveThreshold_DefOverridePropagates(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertCLIAgentDefWithProactiveThreshold(t, env, "impl", 90000)

	sp := newCLIPrepSpawner(env)
	proc, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if proc.proactiveRestartThreshold != 90000 {
		t.Errorf("proc.proactiveRestartThreshold = %d, want 90000", proc.proactiveRestartThreshold)
	}
	if prep.proactiveRestartThreshold != 90000 {
		t.Errorf("prep.proactiveRestartThreshold = %d, want 90000", prep.proactiveRestartThreshold)
	}
}

// TestPrepareSpawn_CLIMode_ProactiveThreshold_ZeroDisables verifies an
// explicit per-def 0 stays 0 (disabled) rather than falling through to the
// global default.
func TestPrepareSpawn_CLIMode_ProactiveThreshold_ZeroDisables(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertCLIAgentDefWithProactiveThreshold(t, env, "impl", 0)
	if err := env.database.SetConfig("proactive_restart_threshold_default", "40000"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	sp := newCLIPrepSpawner(env)
	proc, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if proc.proactiveRestartThreshold != 0 {
		t.Errorf("proc.proactiveRestartThreshold = %d, want 0 (explicit disable wins over global default)", proc.proactiveRestartThreshold)
	}
}

// TestPrepareSpawn_CLIMode_ProactiveThreshold_NilDefFallsThroughToGlobal
// verifies a def with no proactive_restart_threshold_tokens column value
// inherits the global proactive_restart_threshold_default config.
func TestPrepareSpawn_CLIMode_ProactiveThreshold_NilDefFallsThroughToGlobal(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertCLIAgentDefNoProactiveThreshold(t, env, "impl")
	if err := env.database.SetConfig("proactive_restart_threshold_default", "30000"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	sp := newCLIPrepSpawner(env)
	proc, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if proc.proactiveRestartThreshold != 30000 {
		t.Errorf("proc.proactiveRestartThreshold = %d, want 30000 (global default)", proc.proactiveRestartThreshold)
	}
}

// A nil agentDef falling through to the hardcoded default is covered as a
// pure unit test: TestResolveProactiveRestartThreshold_NilDefFallsThroughToDefault
// (context_restart_policy_test.go) — prepareSpawn itself always requires a
// matching agent_definitions row to load a template from, so it cannot
// exercise the nil-def branch end to end without a real DB error.
