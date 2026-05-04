package spawner

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// TestSpawner_RejectsAPIModeAgent_WhenConfigAPIModeFalse verifies that prepareSpawn
// returns "api_mode_disabled" before any provider call when an agent_definition row
// has execution_mode="api" but Config.APIMode is false.
//
// No Provider is set on Config (it would be nil). If the rejection happened AFTER
// provider access the spawner would panic or return a different error.
func TestSpawner_RejectsAPIModeAgent_WhenConfigAPIModeFalse(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
		VALUES ('implementor', ?, 'feature', 'sonnet', 20, '# prompt', 'api', ?, ?)`,
		env.projectID, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		// APIMode deliberately omitted (defaults to false)
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{{ID: "implementor", Agent: "implementor", Layer: 0}},
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
		t.Fatal("Spawn() returned nil error; expected api_mode_disabled")
	}
	if !strings.Contains(spawnErr.Error(), "api_mode_disabled") {
		t.Errorf("Spawn() error = %q; want error containing \"api_mode_disabled\"", spawnErr.Error())
	}
}

// TestSpawner_RejectsAPIModeAgent_ViaConfigAgents verifies that the "api_mode_disabled"
// rejection also fires when the api mode comes from Config.Agents (no agentDef row).
func TestSpawner_RejectsAPIModeAgent_ViaConfigAgents(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// No agent_definitions row — execution mode comes from Config.Agents
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		// APIMode deliberately omitted (defaults to false)
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{{ID: "implementor", Agent: "implementor", Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			"implementor": {
				Model:         "sonnet",
				ExecutionMode: "api", // Config.Agents says api, but APIMode=false
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
		t.Fatal("Spawn() returned nil error; expected api_mode_disabled")
	}
	if !strings.Contains(spawnErr.Error(), "api_mode_disabled") {
		t.Errorf("Spawn() error = %q; want error containing \"api_mode_disabled\"", spawnErr.Error())
	}
}

