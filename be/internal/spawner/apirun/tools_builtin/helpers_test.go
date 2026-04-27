package tools_builtin

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

// fakeHub captures Broadcast calls for assertion.
type fakeHub struct {
	events []*ws.Event
}

func (h *fakeHub) Broadcast(e *ws.Event) {
	h.events = append(h.events, e)
}

// builtinTestEnv assembles a real DB pool, services, and a fake hub bound to
// a seeded project / ticket / workflow_instance / agent_session row so the
// builtin handlers can run their full service path end-to-end.
type builtinTestEnv struct {
	pool *db.Pool
	hub  *fakeHub
	env  apirun.ToolEnv
	clk  *clock.TestClock
}

const (
	testProjectID  = "proj-bt"
	testTicketID   = "T-bt"
	testWorkflow   = "test"
	testWFIID      = "wfi-bt"
	testSessionID  = "sess-bt"
	testAgentType  = "implementor"
	testModelID    = "claude:opus_4_7"
	testInstanceID = testWFIID
)

func newBuiltinTestEnv(t *testing.T) *builtinTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		testProjectID, "Test", now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, created_at, updated_at, scope_type, groups) VALUES (?, ?, '', ?, ?, 'ticket', '["frontend"]')`,
		testWorkflow, testProjectID, now, now)
	mustExec(t, pool, `INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES (?, ?, ?, ?, ?, 'test')`,
		testTicketID, testProjectID, "Test ticket", now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', '{}', 0, ?, ?)`,
		testWFIID, testProjectID, testTicketID, testWorkflow, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'phase1', ?, ?, 'running', ?, ?)`,
		testSessionID, testProjectID, testTicketID, testWFIID, testAgentType, testModelID, now, now)

	hub := &fakeHub{}
	findingsSvc := service.NewFindingsService(pool, clk)
	projectFindingsSvc := service.NewProjectFindingsService(pool, clk)
	agentSvc := service.NewAgentService(pool, clk)
	workflowSvc := service.NewWorkflowService(pool, clk)

	env := apirun.ToolEnv{
		Pool:               pool,
		WSHub:              hub,
		Clock:              clk,
		SessionID:          testSessionID,
		AgentType:          testAgentType,
		ProjectID:          testProjectID,
		TicketID:           testTicketID,
		WorkflowName:       testWorkflow,
		WorkflowInstanceID: testWFIID,
		Findings:           findingsSvc,
		ProjectFindings:    projectFindingsSvc,
		Agent:              agentSvc,
		Workflow:           workflowSvc,
	}
	return &builtinTestEnv{pool: pool, hub: hub, env: env, clk: clk}
}

func mustExec(t *testing.T, pool *db.Pool, query string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\n  query: %s", err, query)
	}
}

// readSessionFindings returns the raw findings JSON for the seeded session.
func (e *builtinTestEnv) readSessionFindings(t *testing.T) string {
	t.Helper()
	var raw string
	err := e.pool.QueryRow(`SELECT IFNULL(findings, '') FROM agent_sessions WHERE id = ?`, testSessionID).Scan(&raw)
	if err != nil {
		t.Fatalf("read findings: %v", err)
	}
	return raw
}

// readSessionResult returns the result column for the seeded session.
func (e *builtinTestEnv) readSessionResult(t *testing.T) string {
	t.Helper()
	var raw *string
	err := e.pool.QueryRow(`SELECT result FROM agent_sessions WHERE id = ?`, testSessionID).Scan(&raw)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if raw == nil {
		return ""
	}
	return *raw
}

// readSessionContextLeft returns the context_left column for the seeded session.
func (e *builtinTestEnv) readSessionContextLeft(t *testing.T) int {
	t.Helper()
	var v *int
	err := e.pool.QueryRow(`SELECT context_left FROM agent_sessions WHERE id = ?`, testSessionID).Scan(&v)
	if err != nil {
		t.Fatalf("read context_left: %v", err)
	}
	if v == nil {
		return -1
	}
	return *v
}

// readSkipTags returns the skip_tags JSON for the seeded workflow instance.
func (e *builtinTestEnv) readSkipTags(t *testing.T) string {
	t.Helper()
	var raw *string
	err := e.pool.QueryRow(`SELECT skip_tags FROM workflow_instances WHERE id = ?`, testWFIID).Scan(&raw)
	if err != nil {
		t.Fatalf("read skip_tags: %v", err)
	}
	if raw == nil {
		return ""
	}
	return *raw
}
