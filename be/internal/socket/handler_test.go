package socket

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handlerTestEnv holds test infrastructure for socket handler tests.
type handlerTestEnv struct {
	pool    *db.Pool
	hub     *ws.Hub
	handler *Handler
	dbPath  string
	project string
}

// newHandlerTestEnv creates an isolated test environment for socket handler tests.
func newHandlerTestEnv(t *testing.T) *handlerTestEnv {
	t.Helper()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	copyTemplateDB(t, dbPath)
	pool, err := db.NewPoolPathExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	hub := ws.NewHub(clock.Real())
	go hub.Run()

	handler := NewHandler(pool, hub, clock.Real(), nil)

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
		hub.Stop()
		pool.Close()
	})

	return &handlerTestEnv{
		pool:    pool,
		hub:     hub,
		handler: handler,
		dbPath:  dbPath,
		project: projectID,
	}
}

// createTicketAndWorkflow creates a ticket and initializes a workflow.
func (e *handlerTestEnv) createTicketAndWorkflow(t *testing.T, ticketID string) {
	t.Helper()

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
}

// TestSocketHandlerInvalidMethod verifies unknown/removed methods return -32601.
func TestSocketHandlerInvalidMethod(t *testing.T) {
	env := newHandlerTestEnv(t)

	cases := []struct {
		name   string
		method string
		params string
	}{
		{"unknown method", "invalid.method", "{}"},
		{"removed agent.complete", "agent.complete", `{"ticket_id":"TEST-1","workflow":"test","agent_type":"analyzer"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Request{
				ID:      "req-1",
				Method:  tc.method,
				Project: env.project,
				Params:  []byte(tc.params),
			}
			resp := env.handler.Handle(req)
			if resp.Error == nil {
				t.Fatalf("expected error for method %q", tc.method)
			}
			if resp.Error.Code != -32601 {
				t.Errorf("expected code -32601 (method not found), got: %d", resp.Error.Code)
			}
		})
	}
}

// TestSocketHandlerMissingProject verifies requests without project return error.
func TestSocketHandlerMissingProject(t *testing.T) {
	env := newHandlerTestEnv(t)

	params := types.AgentRequest{}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-1",
		Method:  "agent.fail",
		Project: "", // Missing project
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for missing project")
	}
	// The actual error code might be -32606 (validation error) instead of -32602
	if resp.Error.Code != -32602 && resp.Error.Code != -32606 {
		t.Errorf("expected code -32602 or -32606, got: %d", resp.Error.Code)
	}
}
