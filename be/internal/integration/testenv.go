package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"be/internal/client"
	"be/internal/db"
	"be/internal/service"
	"be/internal/socket"
	"be/internal/types"
	"be/internal/ws"
)

// TestEnv provides an isolated server stack for integration tests.
type TestEnv struct {
	Pool       *db.Pool
	Hub        *ws.Hub
	Server     *socket.Server
	Client     *client.Client
	SocketPath string
	ProjectDir string
	ProjectID  string

	// Services for direct data seeding (socket no longer handles project/ticket/workflow)
	ProjectSvc  *service.ProjectService
	TicketSvc   *service.TicketService
	WorkflowSvc *service.WorkflowService
	AgentSvc    *service.AgentService
	FindingsSvc *service.FindingsService
}

// NewTestEnv creates a fully isolated test environment:
// fresh DB, dedicated socket, WS hub, config dir, and seeded project + workflow def.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	// 1. DB in temp dir (t.TempDir is fine for DB paths)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// 2. Short socket path (macOS 104-char limit)
	socketPath := fmt.Sprintf("/tmp/nrwf-it-%d-%d.sock", os.Getpid(), time.Now().UnixNano()%100000)
	t.Cleanup(func() { os.Remove(socketPath) })
	t.Setenv("NRWORKFLOW_SOCKET", socketPath)

	// 3. DB pool
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	// 4. WS Hub
	hub := ws.NewHub()
	go hub.Run()

	// 5. Socket server
	srv := socket.NewServerWithHub(pool, hub)
	if err := srv.Start(); err != nil {
		pool.Close()
		hub.Stop()
		t.Fatalf("failed to start server: %v", err)
	}

	// 6. Project dir
	projectDir := t.TempDir()

	// 7. Services
	projectSvc := service.NewProjectService(pool)
	ticketSvc := service.NewTicketService(pool)
	workflowSvc := service.NewWorkflowService(pool)
	agentSvc := service.NewAgentService(pool)
	findingsSvc := service.NewFindingsService(pool)

	// 8. Client (for agent/findings socket tests)
	projectID := "test-project"
	c := client.NewWithSocket(socketPath, projectID)

	// 9. Seed project via service
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "Test Project",
		RootPath: projectDir,
	})
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// 10. Seed test workflow definition via service
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

	env := &TestEnv{
		Pool:        pool,
		Hub:         hub,
		Server:      srv,
		Client:      c,
		SocketPath:  socketPath,
		ProjectDir:  projectDir,
		ProjectID:   projectID,
		ProjectSvc:  projectSvc,
		TicketSvc:   ticketSvc,
		WorkflowSvc: workflowSvc,
		AgentSvc:    agentSvc,
		FindingsSvc: findingsSvc,
	}

	// 11. Cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Stop(ctx)
		hub.Stop()
		pool.Close()
	})

	return env
}

// MustExecute calls client.Execute and fatals on error or error response.
// Use for agent/findings socket methods only.
func (e *TestEnv) MustExecute(t *testing.T, method string, params interface{}, result interface{}) {
	t.Helper()
	resp, err := e.Client.Execute(method, params)
	if err != nil {
		t.Fatalf("MustExecute(%s): connection error: %v", method, err)
	}
	if resp.Error != nil {
		t.Fatalf("MustExecute(%s): server error: code=%d msg=%s", method, resp.Error.Code, resp.Error.Message)
	}
	if result != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			t.Fatalf("MustExecute(%s): unmarshal error: %v (raw: %s)", method, err, string(resp.Result))
		}
	}
}

// ExpectError calls client.Execute and asserts the response has an error with the expected code.
func (e *TestEnv) ExpectError(t *testing.T, method string, params interface{}, expectedCode int) {
	t.Helper()
	resp, err := e.Client.Execute(method, params)
	if err != nil {
		t.Fatalf("ExpectError(%s): connection error: %v", method, err)
	}
	if resp.Error == nil {
		t.Fatalf("ExpectError(%s): expected error code %d but got success (result: %s)", method, expectedCode, string(resp.Result))
	}
	if resp.Error.Code != expectedCode {
		t.Fatalf("ExpectError(%s): expected code %d, got %d (%s)", method, expectedCode, resp.Error.Code, resp.Error.Message)
	}
}

// NewWSClient creates a test WS client subscribed to the given project+ticket.
func (e *TestEnv) NewWSClient(t *testing.T, id, ticketID string) (*ws.Client, chan []byte) {
	t.Helper()
	c, ch := ws.NewTestClient(e.Hub, id)
	e.Hub.Register(c)
	time.Sleep(50 * time.Millisecond) // let registration propagate
	e.Hub.Subscribe(c, e.ProjectID, ticketID)
	time.Sleep(50 * time.Millisecond) // let subscription propagate
	return c, ch
}

// CreateTicket creates a ticket via service layer.
func (e *TestEnv) CreateTicket(t *testing.T, ticketID, title string) {
	t.Helper()
	_, err := e.TicketSvc.Create(e.ProjectID, &types.TicketCreateRequest{
		ID:    ticketID,
		Title: title,
	})
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}
}

// InitWorkflow initializes the "test" workflow on the given ticket.
// Auto-creates the ticket if it doesn't exist.
func (e *TestEnv) InitWorkflow(t *testing.T, ticketID string) {
	t.Helper()
	err := e.WorkflowSvc.Init(e.ProjectID, ticketID, &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}
}

// StartPhase starts a phase on the given ticket's workflow.
func (e *TestEnv) StartPhase(t *testing.T, ticketID, phase string) {
	t.Helper()
	err := e.WorkflowSvc.StartPhase(e.ProjectID, ticketID, &types.PhaseUpdateRequest{
		Workflow: "test",
		Phase:   phase,
	})
	if err != nil {
		t.Fatalf("failed to start phase %s: %v", phase, err)
	}
}

// CompletePhase completes a phase on the given ticket's workflow.
func (e *TestEnv) CompletePhase(t *testing.T, ticketID, phase, result string) {
	t.Helper()
	err := e.WorkflowSvc.CompletePhase(e.ProjectID, ticketID, &types.PhaseUpdateRequest{
		Workflow: "test",
		Phase:   phase,
		Result:  result,
	})
	if err != nil {
		t.Fatalf("failed to complete phase %s: %v", phase, err)
	}
}

// GetWorkflowInstanceID returns the workflow instance ID for a ticket+workflow.
func (e *TestEnv) GetWorkflowInstanceID(t *testing.T, ticketID, workflow string) string {
	t.Helper()
	var id string
	err := e.Pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		e.ProjectID, ticketID, workflow).Scan(&id)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}
	return id
}

// InsertAgentSession inserts an agent session row into the DB for testing.
func (e *TestEnv) InsertAgentSession(t *testing.T, id, ticketID, wfiID, phase, agentType, modelID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := e.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'running', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		id, e.ProjectID, ticketID, wfiID, phase, agentType,
		nullStr(modelID),
		now, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert session %s: %v", id, err)
	}
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// getAgentDefServiceInternal returns the AgentDefinitionService for testing.
func (e *TestEnv) getAgentDefServiceInternal(t *testing.T) *service.AgentDefinitionService {
	return service.NewAgentDefinitionService(e.Pool)
}
