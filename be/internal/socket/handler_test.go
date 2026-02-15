package socket

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

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

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	hub := ws.NewHub(clock.Real())
	go hub.Run()

	handler := NewHandler(pool, hub, clock.Real())

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
	err = workflowSvc.Init(e.project, ticketID, &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}
}

// TestAgentCompleteEventPayload verifies agent.complete broadcasts include required fields.
func TestAgentCompleteEventPayload(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")

	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "TEST-1", "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	// Create an active agent session directly in DB
	sessionID := "sess-test-complete"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TEST-1", wfiID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Subscribe a test client to receive broadcasts
	client, sendCh := ws.NewTestClient(env.hub, "test-client")
	env.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	env.hub.Subscribe(client, env.project, "TEST-1")

	// Call agent.complete
	params := struct {
		TicketID string `json:"ticket_id"`
		types.AgentCompleteRequest
	}{
		TicketID: "TEST-1",
		AgentCompleteRequest: types.AgentCompleteRequest{
			Workflow:  "test",
			AgentType: "analyzer",
			Model:     "claude-opus-4",
		},
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-1",
		Method:  "agent.complete",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify broadcast event
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}

		if event.Type != ws.EventAgentCompleted {
			t.Errorf("expected event type %s, got %s", ws.EventAgentCompleted, event.Type)
		}

		// Verify required payload fields
		sessionID, ok := event.Data["session_id"].(string)
		if !ok || sessionID == "" {
			t.Errorf("session_id must be present in payload, got: %v", event.Data["session_id"])
		}

		modelID, ok := event.Data["model_id"].(string)
		if !ok || modelID != "claude-opus-4" {
			t.Errorf("expected model_id=claude-opus-4, got: %v", event.Data["model_id"])
		}

		result, ok := event.Data["result"].(string)
		if !ok || result != "pass" {
			t.Errorf("expected result=pass, got: %v", event.Data["result"])
		}

		agentType, ok := event.Data["agent_type"].(string)
		if !ok || agentType != "analyzer" {
			t.Errorf("expected agent_type=analyzer, got: %v", event.Data["agent_type"])
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.completed broadcast")
	}
}

// TestAgentFailEventPayload verifies agent.fail broadcasts include required fields.
func TestAgentFailEventPayload(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")

	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "TEST-1", "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	// Create an active agent session directly in DB
	sessionID := "sess-test-fail"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TEST-1", wfiID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Subscribe a test client to receive broadcasts
	client, sendCh := ws.NewTestClient(env.hub, "test-client")
	env.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	env.hub.Subscribe(client, env.project, "TEST-1")

	// Call agent.fail
	params := struct {
		TicketID string `json:"ticket_id"`
		types.AgentCompleteRequest
	}{
		TicketID: "TEST-1",
		AgentCompleteRequest: types.AgentCompleteRequest{
			Workflow:  "test",
			AgentType: "analyzer",
			Model:     "claude-sonnet-4",
		},
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-1",
		Method:  "agent.fail",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify broadcast event
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}

		if event.Type != ws.EventAgentCompleted {
			t.Errorf("expected event type %s, got %s", ws.EventAgentCompleted, event.Type)
		}

		// Verify required payload fields
		sessionID, ok := event.Data["session_id"].(string)
		if !ok || sessionID == "" {
			t.Errorf("session_id must be present in payload, got: %v", event.Data["session_id"])
		}

		modelID, ok := event.Data["model_id"].(string)
		if !ok || modelID != "claude-sonnet-4" {
			t.Errorf("expected model_id=claude-sonnet-4, got: %v", event.Data["model_id"])
		}

		result, ok := event.Data["result"].(string)
		if !ok || result != "fail" {
			t.Errorf("expected result=fail, got: %v", event.Data["result"])
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.completed broadcast")
	}
}

// TestAgentContinueEventPayload verifies agent.continue broadcasts include required fields.
func TestAgentContinueEventPayload(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")

	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "TEST-1", "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	// Create an active agent session directly in DB
	sessionID := "sess-test-continue"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'gpt-5.3', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TEST-1", wfiID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Subscribe a test client to receive broadcasts
	client, sendCh := ws.NewTestClient(env.hub, "test-client")
	env.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	env.hub.Subscribe(client, env.project, "TEST-1")

	// Call agent.continue
	params := struct {
		TicketID string `json:"ticket_id"`
		types.AgentCompleteRequest
	}{
		TicketID: "TEST-1",
		AgentCompleteRequest: types.AgentCompleteRequest{
			Workflow:  "test",
			AgentType: "analyzer",
			Model:     "gpt-5.3",
		},
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-1",
		Method:  "agent.continue",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify broadcast event
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}

		if event.Type != ws.EventAgentContinued {
			t.Errorf("expected event type %s, got %s", ws.EventAgentContinued, event.Type)
		}

		// Verify required payload fields
		sessionID, ok := event.Data["session_id"].(string)
		if !ok || sessionID == "" {
			t.Errorf("session_id must be present in payload, got: %v", event.Data["session_id"])
		}

		modelID, ok := event.Data["model_id"].(string)
		if !ok || modelID != "gpt-5.3" {
			t.Errorf("expected model_id=gpt-5.3, got: %v", event.Data["model_id"])
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.continued broadcast")
	}
}

// TestAgentCallbackEventPayload verifies agent.callback broadcasts include required fields.
func TestAgentCallbackEventPayload(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")

	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "TEST-1", "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	// Create an active agent session directly in DB
	sessionID := "sess-test-callback"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TEST-1", wfiID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Subscribe a test client to receive broadcasts
	client, sendCh := ws.NewTestClient(env.hub, "test-client")
	env.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	env.hub.Subscribe(client, env.project, "TEST-1")

	// Call agent.callback
	params := struct {
		TicketID string `json:"ticket_id"`
		types.AgentCallbackRequest
	}{
		TicketID: "TEST-1",
		AgentCallbackRequest: types.AgentCallbackRequest{
			AgentCompleteRequest: types.AgentCompleteRequest{
				Workflow:  "test",
				AgentType: "analyzer",
				Model:     "claude-opus-4",
			},
			Level: 0,
		},
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-1",
		Method:  "agent.callback",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify broadcast event
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}

		if event.Type != ws.EventAgentCompleted {
			t.Errorf("expected event type %s, got %s", ws.EventAgentCompleted, event.Type)
		}

		// Verify required payload fields
		modelID, ok := event.Data["model_id"].(string)
		if !ok || modelID != "claude-opus-4" {
			t.Errorf("expected model_id=claude-opus-4, got: %v", event.Data["model_id"])
		}

		result, ok := event.Data["result"].(string)
		if !ok || result != "callback" {
			t.Errorf("expected result=callback, got: %v", event.Data["result"])
		}

		level, ok := event.Data["level"].(float64)
		if !ok || int(level) != 0 {
			t.Errorf("expected level=0, got: %v", event.Data["level"])
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.completed broadcast")
	}
}

// TestSocketHandlerInvalidMethod verifies unknown methods return error.
func TestSocketHandlerInvalidMethod(t *testing.T) {
	env := newHandlerTestEnv(t)

	req := Request{
		ID:      "req-1",
		Method:  "invalid.method",
		Project: env.project,
		Params:  []byte("{}"),
	}

	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for invalid method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601 (method not found), got: %d", resp.Error.Code)
	}
}

// TestSocketHandlerMissingProject verifies requests without project return error.
func TestSocketHandlerMissingProject(t *testing.T) {
	env := newHandlerTestEnv(t)

	params := struct {
		TicketID string `json:"ticket_id"`
		types.AgentCompleteRequest
	}{
		TicketID: "TEST-1",
		AgentCompleteRequest: types.AgentCompleteRequest{
			Workflow:  "test",
			AgentType: "analyzer",
		},
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-1",
		Method:  "agent.complete",
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
