package spawner

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// TestLoadProjectPythonTools_NilRepo verifies that a nil PythonScriptRepo returns
// nil handlers without error.
func TestLoadProjectPythonTools_NilRepo(t *testing.T) {
	t.Parallel()
	s := New(Config{PythonScriptRepo: nil})
	handlers, err := s.loadProjectPythonTools("proj-1", "sess-1")
	if err != nil {
		t.Fatalf("loadProjectPythonTools: %v", err)
	}
	if len(handlers) != 0 {
		t.Errorf("handlers len=%d, want 0", len(handlers))
	}
}

// TestLoadProjectPythonTools_NoRows verifies that a project with no python_scripts
// rows returns an empty slice.
func TestLoadProjectPythonTools_NoRows(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	scriptRepo := repo.NewPythonScriptRepo(env.database, clock.Real())
	s := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		PythonScriptRepo: scriptRepo,
	})

	handlers, err := s.loadProjectPythonTools(env.projectID, "sess-x")
	if err != nil {
		t.Fatalf("loadProjectPythonTools: %v", err)
	}
	if len(handlers) != 0 {
		t.Errorf("handlers len=%d, want 0 for empty project", len(handlers))
	}
}

// TestLoadProjectPythonTools_ToolRowsOnly verifies that only kind=tool rows produce
// handlers — kind=agent rows are excluded.
func TestLoadProjectPythonTools_ToolRowsOnly(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	scriptRepo := repo.NewPythonScriptRepo(env.database, clock.Real())

	toolRows := []*model.PythonScript{
		{ID: "tool-1", ProjectID: env.projectID, Name: "lookup_sku", Kind: "tool",
			Code: `print("ok")`, ToolDescription: "SKU lookup", InputSchema: `{}`, TimeoutSec: 30},
		{ID: "tool-2", ProjectID: env.projectID, Name: "search_db", Kind: "tool",
			Code: `print("ok")`, ToolDescription: "DB search", InputSchema: `{}`, TimeoutSec: 30},
	}
	agentRow := &model.PythonScript{
		ID: "agent-1", ProjectID: env.projectID, Name: "my_agent", Kind: "agent",
		Code: `print("agent")`,
	}
	for _, row := range toolRows {
		if err := scriptRepo.Create(row); err != nil {
			t.Fatalf("create tool row %s: %v", row.Name, err)
		}
	}
	if err := scriptRepo.Create(agentRow); err != nil {
		t.Fatalf("create agent row: %v", err)
	}

	s := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		PythonScriptRepo: scriptRepo,
	})

	handlers, err := s.loadProjectPythonTools(env.projectID, "sess-x")
	if err != nil {
		t.Fatalf("loadProjectPythonTools: %v", err)
	}
	if len(handlers) != 2 {
		t.Errorf("handlers len=%d, want 2 (tool rows only)", len(handlers))
	}

	names := map[string]bool{}
	for _, h := range handlers {
		names[h.Spec().Name] = true
	}
	if !names["lookup_sku"] {
		t.Errorf("lookup_sku not in handlers, got %v", names)
	}
	if !names["search_db"] {
		t.Errorf("search_db not in handlers, got %v", names)
	}
	if names["my_agent"] {
		t.Errorf("my_agent (kind=agent) should not appear in tool handlers")
	}
}

// TestPrepareSpawn_PythonBuiltinCollision_ReturnsError verifies that spawning an
// api-mode agent fails when a project python tool name collides with a builtin.
func TestPrepareSpawn_PythonBuiltinCollision_ReturnsError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-test-fake")

	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	scriptRepo := repo.NewPythonScriptRepo(env.database, clock.Real())
	if err := scriptRepo.Create(&model.PythonScript{
		ID:              "coll-1",
		ProjectID:       env.projectID,
		Name:            "findings_add",
		Kind:            "tool",
		Code:            `print("x")`,
		ToolDescription: "test",
		InputSchema:     `{}`,
		TimeoutSec:      30,
	}); err != nil {
		t.Fatalf("create python script: %v", err)
	}

	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, created_at, updated_at)
		VALUES ('impl', ?, 'feature', 'sonnet', 20, '# test', 'api', '*', ?, ?)`,
		env.projectID, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition: %v", err)
	}

	sp := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		APIMode:          true,
		PythonScriptRepo: scriptRepo,
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{{ID: "impl", Agent: "impl", Layer: 0}},
			},
		},
	})

	spawnErr := sp.Spawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	})

	if spawnErr == nil {
		t.Fatal("Spawn() returned nil; expected builtin collision error")
	}
	if !strings.Contains(spawnErr.Error(), "collides with builtin") {
		t.Errorf("Spawn() error = %q; want substring 'collides with builtin'", spawnErr.Error())
	}
}
