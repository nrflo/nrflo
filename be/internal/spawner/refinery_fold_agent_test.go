package spawner

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"

	"github.com/google/uuid"
)

// refineryFoldTestEnv is a minimal harness for Spawner.RunRefineryFold:
// a migrated template DB copy plus a seeded project, mirroring
// consultTestEnv/contextSaveTestEnv's shape but scoped to the fields
// RunRefineryFold's child-spawn path actually touches.
type refineryFoldTestEnv struct {
	database  *db.DB
	dbPath    string
	pool      *db.Pool
	projectID string
	spawner   *Spawner
	cleanup   func()
}

func setupRefineryFoldTestEnv(t *testing.T) *refineryFoldTestEnv {
	t.Helper()
	dbPath := "/tmp/test_refinery_fold_" + uuid.New().String() + ".db"
	copyTemplateDB(t, dbPath)
	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	pool := db.WrapAsPool(database)

	projectID := "test-project"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		projectID, "Test Project", "/tmp", now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	clk := clock.Real()
	sp := New(Config{
		DataPath:           dbPath,
		Pool:               pool,
		Clock:              clk,
		WSHub:              ws.NewHub(clk),
		AgentSvc:           &noopAgentSvc{},
		FindingsSvc:        service.NewFindingsService(pool, clk),
		ProjectFindingsSvc: service.NewProjectFindingsService(pool, clk),
		AgentSvcReal:       service.NewAgentService(pool, clk),
		WorkflowSvc:        service.NewWorkflowService(pool, clk),
	})

	return &refineryFoldTestEnv{
		database:  database,
		dbPath:    dbPath,
		pool:      pool,
		projectID: projectID,
		spawner:   sp,
		cleanup: func() {
			database.Close()
			os.Remove(dbPath)
		},
	}
}

// seedWorkflowInstance inserts a minimal active workflow_instances row the
// one-off `_refinery_fold` child Spawn() can resolve by id — a required
// precondition for Spawn (spawner.go's "project workflow not initialized"
// gate), unrelated to the fold logic under test.
func (env *refineryFoldTestEnv) seedWorkflowInstance(t *testing.T) string {
	t.Helper()
	wfiID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		 VALUES (?, ?, '', '_refinery_fold', 'project', 'active', ?, ?)`,
		wfiID, env.projectID, now, now,
	); err != nil {
		t.Fatalf("seed workflow instance: %v", err)
	}
	return wfiID
}

func TestRunRefineryFold_MissingProjectID_ReturnsValidationError(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()

	_, err := env.spawner.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ModelID:  "haiku-4-5",
		UserText: "digest this",
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want validation error for missing project_id")
	}
}

func TestRunRefineryFold_MissingModelID_ReturnsValidationError(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()

	_, err := env.spawner.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ProjectID: env.projectID,
		UserText:  "digest this",
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want validation error for missing model_id")
	}
}

func TestRunRefineryFold_MissingUserText_ReturnsValidationError(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()

	_, err := env.spawner.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ProjectID: env.projectID,
		ModelID:   "haiku-4-5",
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want validation error for missing user_text")
	}
}

func TestRunRefineryFold_NoPool_ReturnsError(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	_, err := sp.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ProjectID: "proj", ModelID: "haiku-4-5", UserText: "text",
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want error for no database pool")
	}
}

func TestRunRefineryFold_MissingRefineryCLIDef_ReturnsError(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()
	if _, err := env.database.Exec(`DELETE FROM system_agent_definitions WHERE id = '_refinery-cli'`); err != nil {
		t.Fatalf("delete _refinery-cli def: %v", err)
	}

	_, err := env.spawner.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ProjectID: env.projectID,
		ModelID:   "haiku-4-5",
		UserText:  "digest this",
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want error for missing _refinery-cli def")
	}
}

func TestRunRefineryFold_ProjectNotFound_ReturnsError(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()

	_, err := env.spawner.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ProjectID: "does-not-exist",
		ModelID:   "haiku-4-5",
		UserText:  "digest this",
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want error for a project lookup failure")
	}
}

// TestRunRefineryFold_UnsupportedCLIModel_WrapsProviderBuildError drives a
// real (fast, no CLI process) build-time failure: gpt-5.3-codex has an empty
// cli_model (migration 000170), so prepareSpawn's cli-mode branch rejects it
// before any process is spawned. RunRefineryFold must classify this via
// isProviderBuildError and wrap it in types.ErrRefineryFoldProviderBuild so
// the refinery chain walk advances past it.
func TestRunRefineryFold_UnsupportedCLIModel_WrapsProviderBuildError(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()
	wfiID := env.seedWorkflowInstance(t)

	_, err := env.spawner.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ProjectID:          env.projectID,
		ModelID:            "gpt-5.3-codex",
		Provider:           "openai",
		UserText:           "digest this",
		WorkflowInstanceID: wfiID,
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want a provider-build error")
	}
	if !errors.Is(err, types.ErrRefineryFoldProviderBuild) {
		t.Errorf("error = %v, want errors.Is(err, types.ErrRefineryFoldProviderBuild)", err)
	}
}
