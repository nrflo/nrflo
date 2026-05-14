package spawner

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// TestAgentConfigPrecedence_AgentDefWinsOverConfigAgents verifies that when an
// agent_definitions row exists with execution_mode="api", it wins over a
// conflicting Config.Agents entry with execution_mode="cli". The test uses the
// presence of "api mode" in the spawn error to distinguish the two paths:
// api-mode spawn fails at API key resolution ("api mode: ...") whereas cli-mode
// fails later (binary not found, no "api mode" in error).
func TestAgentConfigPrecedence_AgentDefWinsOverConfigAgents(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "") // No API key forces api-mode to fail early

	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Seed an agent_definition with execution_mode="api" for the "feature" workflow.
	// Config.Agents below will specify execution_mode="cli" — agentDef must win.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
		VALUES ('implementor', ?, 'feature', 'sonnet', 20, '# Implement the feature', 'api', ?, ?)`,
		env.projectID, now, now,
	)
	if err != nil {
		t.Fatalf("insert agent_definition: %v", err)
	}

	// Spawner with Config.Agents conflicting (cli) — agentDef api should win.
	// APIMode=true bypasses the early api_mode_disabled gate so precedence logic
	// can run to completion (failing at API key resolution with "api mode" prefix).
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true, // allow api-mode to reach the API key check
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{{ID: "implementor", Agent: "implementor", Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			"implementor": {
				Model:         "sonnet",
				ExecutionMode: "cli", // conflicts with agentDef execution_mode="api"
			},
		},
	})

	spawnErr := sp.Spawn(context.Background(), SpawnRequest{
		AgentType:          "implementor",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	})

	if spawnErr == nil {
		t.Fatal("Spawn() returned nil error; expected failure in test environment")
	}
	// If agentDef wins (api mode): error contains "api mode" (from ResolveAPIKey failure)
	// If Config.Agents wins (cli mode): error does NOT contain "api mode"
	if !strings.Contains(spawnErr.Error(), "api mode") {
		t.Errorf("Spawn() error = %q; want error containing \"api mode\" "+
			"(agentDef execution_mode=api should override Config.Agents execution_mode=cli)",
			spawnErr.Error())
	}
}

// TestAgentConfigPrecedence_FallsBackToConfigWhenNoAgentDef verifies that when
// no agent_definitions row exists, prepareSpawn falls back to Config.Agents for
// the execution mode. This is the normal path for synthetic system agents like
// the context-saver (workflow "_context_save" has no agent_definitions rows).
func TestAgentConfigPrecedence_FallsBackToConfigWhenNoAgentDef(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "") // No API key forces api-mode to fail early

	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Seed a system_agent_definitions row so loadPromptContent("my-agent") can
	// find a prompt. The execution_mode here is cli_interactive but prepareSpawn sets
	// executionMode from Config.Agents (not from system_agent_definitions) when
	// the project-scoped agent_definitions row is absent (agentDef==nil).
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO system_agent_definitions
			(id, role, model, timeout, prompt, tools, execution_mode, created_at, updated_at)
		VALUES ('my-agent', 'my-agent', 'sonnet', 20, '# Test prompt', '', 'cli_interactive', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert system_agent_definitions: %v", err)
	}

	// No agent_definition row — Config.Agents["my-agent"].ExecutionMode="api" must be used.
	// APIMode=true bypasses the early api_mode_disabled gate so the fallback logic
	// can run to completion (failing at API key resolution with "api mode" prefix).
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true, // allow api-mode to reach the API key check
		Workflows: map[string]WorkflowDef{
			"_test_wf": {
				Phases: []PhaseDef{{ID: "my-agent", Agent: "my-agent", Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			"my-agent": {
				Model:         "sonnet",
				ExecutionMode: "api", // no agentDef → this should be used
			},
		},
	})

	spawnErr := sp.Spawn(context.Background(), SpawnRequest{
		AgentType:          "my-agent",
		ProjectID:          env.projectID,
		WorkflowName:       "_test_wf",
		WorkflowInstanceID: env.wfiID,
	})

	if spawnErr == nil {
		t.Fatal("Spawn() returned nil error; expected failure in test environment")
	}
	// Config.Agents api mode → fails at API key with "api mode" prefix
	if !strings.Contains(spawnErr.Error(), "api mode") {
		t.Errorf("Spawn() error = %q; want error containing \"api mode\" "+
			"(Config.Agents execution_mode=api should apply when no agentDef found)",
			spawnErr.Error())
	}
}
