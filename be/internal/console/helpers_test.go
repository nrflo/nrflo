package console

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

const (
	testProjectID      = "proj-console-t"
	testOtherProjectID = "proj-console-other-t"
	testTicketID       = "T-console-t"
)

// fakeHub captures Broadcast calls for assertion.
type fakeHub struct {
	events []*ws.Event
}

func (h *fakeHub) Broadcast(e *ws.Event) { h.events = append(h.events, e) }

// consoleTestEnv assembles a real DB pool + console.Deps against two seeded
// projects (testProjectID / testOtherProjectID), so cross-project rejection
// can be exercised without mocking the repo layer.
type consoleTestEnv struct {
	pool *db.Pool
	deps Deps
	hub  *fakeHub
	clk  *clock.TestClock
	home string
}

func newConsoleTestEnv(t *testing.T) *consoleTestEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)
	dbPath := filepath.Join(home, "test.db")
	if err := copyConsoleTemplateDB(dbPath); err != nil {
		t.Fatalf("copyConsoleTemplateDB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	for _, p := range []string{testProjectID, testOtherProjectID} {
		mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			p, p, now, now)
	}
	mustExec(t, pool, `INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES (?, ?, ?, ?, ?, 'test')`,
		testTicketID, testProjectID, "Test ticket", now, now)

	hub := &fakeHub{}
	deps := Deps{
		Pool:               pool,
		Clock:              clk,
		WSHub:              hub,
		DataPath:           dbPath,
		WorkflowSvc:        service.NewWorkflowService(pool, clk),
		TicketSvc:          service.NewTicketService(pool, clk),
		ProjectFindingsSvc: service.NewProjectFindingsService(pool, clk),
		ArtifactSvc:        service.NewArtifactService(pool, clk, hub, dbPath),
		WaitBroker:         NewWaitBroker(),
	}
	return &consoleTestEnv{pool: pool, deps: deps, hub: hub, clk: clk, home: home}
}

func mustExec(t *testing.T, pool *db.Pool, query string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\n  query: %s", err, query)
	}
}

// seedWorkflowInstance inserts a minimal workflow + workflow_instance row
// owned by projectID and returns the instance id.
func (e *consoleTestEnv) seedWorkflowInstance(t *testing.T, projectID, instanceID string) string {
	t.Helper()
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	wfName := "wf-" + instanceID
	mustExec(t, e.pool, `INSERT INTO workflows (id, project_id, description, created_at, updated_at, scope_type, groups)
		VALUES (?, ?, '', ?, ?, 'project', '[]')`, wfName, projectID, now, now)
	mustExec(t, e.pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, retry_count, created_at, updated_at)
		VALUES (?, ?, '', ?, 'active', 'project', 0, ?, ?)`, instanceID, projectID, wfName, now, now)
	return instanceID
}

// invoke runs a registered handler by name and returns the (output, isErr, err) tuple.
func invoke(t *testing.T, reg apirun.Registry, env apirun.ToolEnv, name string, input string) (string, bool, error) {
	t.Helper()
	h, ok := reg[name]
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	return h.Invoke(context.Background(), env, json.RawMessage(input))
}

// fakeOrchestrator implements console.Orchestrator, recording calls.
type fakeOrchestrator struct {
	startProjectID        string
	startTicketID         string
	startWorkflow         string
	startInstructions     string
	startScopeType        string
	startInstanceID       string
	startErr              error
	startConsoleSessionID string
	startPlanManifest     json.RawMessage

	stopProjectProjectID  string
	stopProjectWorkflow   string
	stopProjectInstanceID string
	stopProjectErr        error
	stopProjectCalled     int

	retryFailedErr        error
	retryFailedProjectErr error

	runPlannerInstanceID string
	runPlannerInput      service.PlannerInput
	runPlannerSessionID  string
	runPlannerErr        error

	resumeInstanceID string
	resumeErr        error

	claimInstanceID string
	claimResult     bool
}

func (f *fakeOrchestrator) StartWorkflow(ctx context.Context, projectID, ticketID, workflowName, instructions, scopeType string) (string, error) {
	f.startProjectID = projectID
	f.startTicketID = ticketID
	f.startWorkflow = workflowName
	f.startInstructions = instructions
	f.startScopeType = scopeType
	if f.startErr != nil {
		return "", f.startErr
	}
	if f.startInstanceID == "" {
		f.startInstanceID = "wfi-fake"
	}
	return f.startInstanceID, nil
}

func (f *fakeOrchestrator) StartConsoleWorkflow(ctx context.Context, projectID, ticketID, workflowName, instructions, scopeType, consoleSessionID string, planManifest json.RawMessage) (string, error) {
	f.startConsoleSessionID = consoleSessionID
	f.startPlanManifest = planManifest
	return f.StartWorkflow(ctx, projectID, ticketID, workflowName, instructions, scopeType)
}

func (f *fakeOrchestrator) StopByProject(projectID, workflowName, instanceID string) error {
	f.stopProjectCalled++
	f.stopProjectProjectID = projectID
	f.stopProjectWorkflow = workflowName
	f.stopProjectInstanceID = instanceID
	return f.stopProjectErr
}

func (f *fakeOrchestrator) RetryFailed(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error {
	return f.retryFailedErr
}

func (f *fakeOrchestrator) RetryFailedProject(ctx context.Context, projectID, workflowName, sessionID, instanceID string) error {
	return f.retryFailedProjectErr
}

func (f *fakeOrchestrator) RunPlanner(ctx context.Context, instanceID string, in service.PlannerInput) (string, error) {
	f.runPlannerInstanceID = instanceID
	f.runPlannerInput = in
	if f.runPlannerErr != nil {
		return "", f.runPlannerErr
	}
	if f.runPlannerSessionID == "" {
		f.runPlannerSessionID = "sess-planner-fake"
	}
	return f.runPlannerSessionID, nil
}

func (f *fakeOrchestrator) ResumeAfterPlanApproval(ctx context.Context, instanceID string) error {
	f.resumeInstanceID = instanceID
	return f.resumeErr
}

func (f *fakeOrchestrator) ClaimPlanApprovalAtBoundary(instanceID string) bool {
	f.claimInstanceID = instanceID
	return f.claimResult
}

var _ Orchestrator = (*fakeOrchestrator)(nil)
