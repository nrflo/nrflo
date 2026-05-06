package spawner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// setupScriptSpawnEnv creates an isolated DB with migrations applied, inserts a
// project and a python_scripts row, and returns a spawner configured with
// PythonScriptRepo pointing at that DB.
type scriptSpawnEnv struct {
	database  *db.DB
	dbPath    string
	projectID string
	scriptID  string
	spawner   *Spawner
	cleanup   func()
}

func setupScriptSpawnEnv(t *testing.T) *scriptSpawnEnv {
	t.Helper()
	dbPath := "/tmp/test_script_spawn_" + uuid.New().String() + ".db"
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}

	projectID := "proj-script-spawn"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, "Test Project", now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	scriptID := "ps-spawn-test-1"
	if _, err := database.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, file_path, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		scriptID, projectID, "Test Script", "", "print('from code')", "", now, now,
	); err != nil {
		t.Fatalf("seed python_script: %v", err)
	}

	pool := db.WrapAsPool(database)
	scriptRepo := repo.NewPythonScriptRepo(pool, clock.Real())

	sp := New(Config{
		DataPath:         dbPath,
		Pool:             pool,
		Clock:            clock.NewTest(time.Now()),
		PythonScriptRepo: scriptRepo,
	})

	return &scriptSpawnEnv{
		database:  database,
		dbPath:    dbPath,
		projectID: projectID,
		scriptID:  scriptID,
		spawner:   sp,
		cleanup: func() {
			database.Close()
			os.Remove(dbPath)
		},
	}
}

func makeMinimalAgentDef(scriptID string) *model.AgentDefinition {
	return &model.AgentDefinition{
		ID:             "agent-def-1",
		ExecutionMode:  "script",
		PythonScriptID: &scriptID,
	}
}

// TestPrepareScriptSpawn_UsesCodeField verifies that when file_path is empty,
// scriptCode is taken from the script.Code DB column.
func TestPrepareScriptSpawn_UsesCodeField(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, prep, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	if prep.scriptCode != "print('from code')" {
		t.Errorf("scriptCode = %q, want %q", prep.scriptCode, "print('from code')")
	}
}

// TestPrepareScriptSpawn_FilePathOverridesCode verifies that when file_path is set,
// the script code is read from the file instead of the DB code column.
func TestPrepareScriptSpawn_FilePathOverridesCode(t *testing.T) {
	t.Parallel()
	if _, err := os.LookupEnv("SKIP_FILE_TESTS"); false && err {
		t.Skip()
	}
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	dir := t.TempDir()
	pyFile := filepath.Join(dir, "override.py")
	fileContent := "print('from file override')"
	if err := os.WriteFile(pyFile, []byte(fileContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Update the python_script row to have file_path set.
	if _, err := env.database.Exec(
		`UPDATE python_scripts SET file_path = ? WHERE id = ?`,
		pyFile, env.scriptID,
	); err != nil {
		t.Fatalf("update file_path: %v", err)
	}

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, prep, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	if prep.scriptCode != fileContent {
		t.Errorf("scriptCode = %q, want %q (from file)", prep.scriptCode, fileContent)
	}
}

// TestPrepareScriptSpawn_FilePathMissing verifies that a spawn-time error is
// returned when file_path points to a non-existent file.
func TestPrepareScriptSpawn_FilePathMissing(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	if _, err := env.database.Exec(
		`UPDATE python_scripts SET file_path = ? WHERE id = ?`,
		"/nonexistent/script.py", env.scriptID,
	); err != nil {
		t.Fatalf("update: %v", err)
	}

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, _, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err == nil {
		t.Fatal("prepareScriptSpawn() expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_file_path_invalid") {
		t.Errorf("error = %q, want to contain \"python_script_file_path_invalid\"", err.Error())
	}
}

// TestPrepareScriptSpawn_FilePathNotPy verifies spawn-time rejection when
// file_path does not end in .py.
func TestPrepareScriptSpawn_FilePathNotPy(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	dir := t.TempDir()
	f := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(f, []byte("#!/bin/sh"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := env.database.Exec(
		`UPDATE python_scripts SET file_path = ? WHERE id = ?`,
		f, env.scriptID,
	); err != nil {
		t.Fatalf("update: %v", err)
	}

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, _, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err == nil {
		t.Fatal("prepareScriptSpawn() expected error for .sh file, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_file_path_invalid") {
		t.Errorf("error = %q, want to contain \"python_script_file_path_invalid\"", err.Error())
	}
}

// TestPrepareScriptSpawn_NilRepo verifies that a nil PythonScriptRepo returns
// an appropriate error.
func TestPrepareScriptSpawn_NilRepo(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.NewTest(time.Now())})
	scriptID := "ps-1"
	agentDef := &model.AgentDefinition{
		ID:             "agent-def-1",
		ExecutionMode:  "script",
		PythonScriptID: &scriptID,
	}
	_, _, err := sp.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: "proj", AgentType: "agent"},
		"L0", "wfi-1", "agent-1", "sess-1", "tok",
		agentDef,
	)
	if err == nil {
		t.Fatal("prepareScriptSpawn() expected error for nil repo, got nil")
	}
	if !strings.Contains(err.Error(), "PythonScriptRepo not configured") {
		t.Errorf("error = %q, want to contain \"PythonScriptRepo not configured\"", err.Error())
	}
}

// TestPrepareScriptSpawn_NilAgentDef verifies that a nil agentDef returns error.
func TestPrepareScriptSpawn_NilAgentDef(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	_, _, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "agent"},
		"L0", "wfi-1", "agent-1", "sess-1", "tok",
		nil,
	)
	if err == nil {
		t.Fatal("prepareScriptSpawn() expected error for nil agentDef, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_id_required") {
		t.Errorf("error = %q, want to contain \"python_script_id_required\"", err.Error())
	}
}
