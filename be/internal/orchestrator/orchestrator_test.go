package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// testEnv holds test infrastructure for orchestrator tests.
type testEnv struct {
	pool    *db.Pool
	hub     *ws.Hub
	orch    *Orchestrator
	dbPath  string
	project string
}

// newTestEnv creates an isolated test environment with a fresh DB and WS hub.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	hub := ws.NewHub()
	go hub.Run()

	orch := New(dbPath, hub)

	projectID := "test-project"

	// Seed project
	projectSvc := service.NewProjectService(pool)
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "Test Project",
		RootPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Seed test workflow definition
	workflowSvc := service.NewWorkflowService(pool)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "builder", "layer": 1},
	})
	_, err = workflowSvc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{
		ID:          "test",
		Description: "Test workflow",
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to seed workflow: %v", err)
	}

	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})

	return &testEnv{
		pool:    pool,
		hub:     hub,
		orch:    orch,
		dbPath:  dbPath,
		project: projectID,
	}
}

// createTicket creates a ticket in the test DB.
func (e *testEnv) createTicket(t *testing.T, ticketID, title string) {
	t.Helper()
	ticketSvc := service.NewTicketService(e.pool)
	_, err := ticketSvc.Create(e.project, &types.TicketCreateRequest{
		ID:    ticketID,
		Title: title,
	})
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}
}

// initWorkflow initializes the "test" workflow on a ticket and returns the instance ID.
func (e *testEnv) initWorkflow(t *testing.T, ticketID string) string {
	t.Helper()
	workflowSvc := service.NewWorkflowService(e.pool)
	err := workflowSvc.Init(e.project, ticketID, &types.WorkflowInitRequest{
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

// initProjectWorkflow initializes a project-scoped workflow and returns the instance ID.
func (e *testEnv) initProjectWorkflow(t *testing.T, workflowID string) string {
	t.Helper()

	// First, update the existing workflow to be project-scoped
	_, err := e.pool.Exec(`UPDATE workflows SET scope_type = 'project' WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		e.project, workflowID)
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	workflowSvc := service.NewWorkflowService(e.pool)
	err = workflowSvc.InitProjectWorkflow(e.project, &types.ProjectWorkflowRunRequest{
		Workflow: workflowID,
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	var id string
	err = e.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND scope_type = 'project'`,
		e.project, workflowID).Scan(&id)
	if err != nil {
		t.Fatalf("failed to get project workflow instance ID: %v", err)
	}
	return id
}

// getTicket retrieves a ticket from the DB.
func (e *testEnv) getTicket(t *testing.T, ticketID string) *model.Ticket {
	t.Helper()
	ticketSvc := service.NewTicketService(e.pool)
	ticket, err := ticketSvc.Get(e.project, ticketID)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	return ticket
}

// getWorkflowInstance retrieves a workflow instance from the DB.
func (e *testEnv) getWorkflowInstance(t *testing.T, wfiID string) *model.WorkflowInstance {
	t.Helper()
	wfiRepo := repo.NewWorkflowInstanceRepo(e.pool)
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	return wi
}

// subscribeWSClient creates a test WS client subscribed to the given ticket.
func (e *testEnv) subscribeWSClient(t *testing.T, id, ticketID string) chan []byte {
	t.Helper()
	client, ch := ws.NewTestClient(e.hub, id)
	e.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	e.hub.Subscribe(client, e.project, ticketID)
	time.Sleep(50 * time.Millisecond)
	return ch
}

// expectEvent waits for a specific WS event type on the channel.
func expectEvent(t *testing.T, ch chan []byte, eventType string, timeout time.Duration) ws.Event {
	t.Helper()
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				t.Fatalf("failed to unmarshal event: %v", err)
			}
			if event.Type == eventType {
				return event
			}
		case <-time.After(timeout):
			t.Fatalf("timeout waiting for event type '%s'", eventType)
		}
	}
}
