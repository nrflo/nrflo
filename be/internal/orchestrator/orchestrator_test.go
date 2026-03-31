package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
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

	hub := ws.NewHub(clock.Real())
	go hub.Run()

	orch := New(dbPath, hub, clock.Real())

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
		// Cancel all running orchestrations and wait for goroutines to exit
		// before TempDir cleanup removes DB files out from under them.
		orch.mu.Lock()
		var doneChans []chan struct{}
		for _, rs := range orch.runs {
			rs.cancel()
			if rs.done != nil {
				doneChans = append(doneChans, rs.done)
			}
		}
		orch.mu.Unlock()
		for _, ch := range doneChans {
			<-ch
		}
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
	ticketSvc := service.NewTicketService(e.pool, clock.Real())
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
	workflowSvc := service.NewWorkflowService(e.pool, clock.Real())
	_, err := workflowSvc.Init(e.project, ticketID, &types.WorkflowInitRequest{
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

	workflowSvc := service.NewWorkflowService(e.pool, clock.Real())
	wi, err := workflowSvc.InitProjectWorkflow(e.project, &types.ProjectWorkflowRunRequest{
		Workflow: workflowID,
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}
	return wi.ID
}

// getTicket retrieves a ticket from the DB.
func (e *testEnv) getTicket(t *testing.T, ticketID string) *model.Ticket {
	t.Helper()
	ticketSvc := service.NewTicketService(e.pool, clock.Real())
	ticket, err := ticketSvc.Get(e.project, ticketID)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	return ticket
}

// getWorkflowInstance retrieves a workflow instance from the DB.
func (e *testEnv) getWorkflowInstance(t *testing.T, wfiID string) *model.WorkflowInstance {
	t.Helper()
	wfiRepo := repo.NewWorkflowInstanceRepo(e.pool, clock.Real())
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
	e.hub.Subscribe(client, e.project, ticketID)
	return ch
}

// stopAndWaitRun cancels a running orchestration and waits for its goroutine to exit.
func (e *testEnv) stopAndWaitRun(t *testing.T, wfiID string) {
	t.Helper()
	e.orch.mu.Lock()
	rs, ok := e.orch.runs[wfiID]
	e.orch.mu.Unlock()
	if !ok {
		return
	}
	rs.cancel()
	if rs.done != nil {
		select {
		case <-rs.done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for orchestration goroutine to exit")
		}
	}
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
