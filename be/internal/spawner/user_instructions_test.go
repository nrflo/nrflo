package spawner

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"

	"github.com/google/uuid"
)

// spawnerTestEnv holds test infrastructure for spawner DB-backed tests.
type spawnerTestEnv struct {
	pool    *db.Pool
	dbPath  string
	project string
}

// newSpawnerTestEnv creates an isolated test environment with a fresh DB.
func newSpawnerTestEnv(t *testing.T) *spawnerTestEnv {
	t.Helper()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	projectID := "test-project"

	// Seed project
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "Test Project",
		RootPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Seed test workflow definition
	workflowSvc := service.NewWorkflowService(pool, clock.Real())
	_, err = workflowSvc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{
		ID:          "test",
		Description: "Test workflow",
	})
	if err != nil {
		t.Fatalf("failed to seed workflow: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return &spawnerTestEnv{
		pool:    pool,
		dbPath:  dbPath,
		project: projectID,
	}
}

// initWorkflow initializes the "test" workflow on a ticket and returns the instance ID.
func (e *spawnerTestEnv) initWorkflow(t *testing.T, ticketID string) string {
	t.Helper()

	// Create ticket
	ticketSvc := service.NewTicketService(e.pool, clock.Real())
	_, err := ticketSvc.Create(e.project, &types.TicketCreateRequest{
		ID:    ticketID,
		Title: "Test ticket",
	})
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}

	workflowSvc := service.NewWorkflowService(e.pool, clock.Real())
	_, err = workflowSvc.Init(e.project, ticketID, &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	var id string
	err = e.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		e.project, ticketID, "test").Scan(&id)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}
	return id
}

// setFindings directly sets the findings JSON on a workflow instance.
func (e *spawnerTestEnv) setFindings(t *testing.T, wfiID string, findings map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("failed to marshal findings: %v", err)
	}
	wfiRepo := repo.NewWorkflowInstanceRepo(e.pool, clock.Real())
	if err := wfiRepo.UpdateFindings(wfiID, string(data)); err != nil {
		t.Fatalf("failed to update findings: %v", err)
	}
}

// newSpawner creates a Spawner with the test DB pool.
func (e *spawnerTestEnv) newSpawner() *Spawner {
	return New(Config{
		DataPath: e.dbPath,
		Pool:     e.pool,
	})
}

func TestFetchUserInstructions_DirectString(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "UI-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// Store instructions as direct string (new format)
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"user_instructions": "Fix the login bug",
	})

	sp := env.newSpawner()
	got := sp.fetchUserInstructions(env.project, ticketID, "test", "")
	if got != "Fix the login bug" {
		t.Fatalf("expected 'Fix the login bug', got %q", got)
	}
}

func TestFetchUserInstructions_MissingReturnsPlaceholder(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "UI-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// No user_instructions in findings at all
	sp := env.newSpawner()
	got := sp.fetchUserInstructions(env.project, ticketID, "test", "")
	expected := "_No user instructions provided_"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestFetchUserInstructions_EmptyStringReturnsPlaceholder(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "UI-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// Store empty string instructions
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"user_instructions": "",
	})

	sp := env.newSpawner()
	got := sp.fetchUserInstructions(env.project, ticketID, "test", "")
	expected := "_No user instructions provided_"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestFetchUserInstructions_NoWorkflowReturnsPlaceholder(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Don't create any ticket or workflow - should return placeholder
	sp := env.newSpawner()
	got := sp.fetchUserInstructions(env.project, "NONEXISTENT", "test", "")
	expected := "_No user instructions provided_"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

// getWfiID retrieves the workflow instance ID for a ticket.
func (e *spawnerTestEnv) getWfiID(t *testing.T, ticketID string) string {
	t.Helper()
	var id string
	err := e.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		e.project, ticketID, "test").Scan(&id)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}
	return id
}
