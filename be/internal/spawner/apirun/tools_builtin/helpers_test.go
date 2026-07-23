package tools_builtin

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
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
	testModelID    = "claude:opus-4-7"
	testInstanceID = testWFIID
)

func newBuiltinTestEnv(t *testing.T) *builtinTestEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)
	dbPath := filepath.Join(home, "test.db")
	if err := copyBuiltinTemplateDB(dbPath); err != nil {
		t.Fatalf("copyBuiltinTemplateDB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
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
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', 0, ?, ?)`,
		testWFIID, testProjectID, testTicketID, testWorkflow, now, now)
	// phase (and thus node_id, which falls back to phase for legacy/unset
	// rows) matches testAgentType — mirroring real spawns, where a static
	// workflow's node_id equals its agent_definitions id.
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'running', ?, ?)`,
		testSessionID, testProjectID, testTicketID, testWFIID, testAgentType, testAgentType, testAgentType, testModelID, now, now)

	hub := &fakeHub{}
	findingsSvc := service.NewFindingsService(pool, clk)
	projectFindingsSvc := service.NewProjectFindingsService(pool, clk)
	agentSvc := service.NewAgentService(pool, clk)
	workflowSvc := service.NewWorkflowService(pool, clk)
	ticketSvc := service.NewTicketService(pool, clk)
	artifactSvc := service.NewArtifactService(pool, clk, hub, filepath.Join(home, "nrflo.data"))

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
		NodeID:             testAgentType,
		Findings:           findingsSvc,
		ProjectFindings:    projectFindingsSvc,
		Agent:              agentSvc,
		Workflow:           workflowSvc,
		Ticket:             ticketSvc,
		ArtifactSvc:        artifactSvc,
	}
	return &builtinTestEnv{pool: pool, hub: hub, env: env, clk: clk}
}

func mustExec(t *testing.T, pool *db.Pool, query string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\n  query: %s", err, query)
	}
}

// readSessionFindings returns the findings as a JSON object string for the seeded session.
func (e *builtinTestEnv) readSessionFindings(t *testing.T) string {
	t.Helper()
	rows, err := e.pool.Query(`SELECT key, value FROM findings WHERE scope='session' AND scope_id=?`, testSessionID)
	if err != nil {
		t.Fatalf("read findings: %v", err)
	}
	defer rows.Close()
	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatalf("scan findings row: %v", err)
		}
		result[k] = json.RawMessage(v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("findings rows err: %v", err)
	}
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal findings: %v", err)
	}
	return string(b)
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

// readSessionResultReason returns the result_reason column for the seeded session.
func (e *builtinTestEnv) readSessionResultReason(t *testing.T) string {
	t.Helper()
	var raw *string
	err := e.pool.QueryRow(`SELECT result_reason FROM agent_sessions WHERE id = ?`, testSessionID).Scan(&raw)
	if err != nil {
		t.Fatalf("read result_reason: %v", err)
	}
	if raw == nil {
		return ""
	}
	return *raw
}

// readCursor reads the (revision, current_index, completed, rejections) tuple
// for the seeded (testWFIID, testAgentType) cursor.
func (e *builtinTestEnv) readCursor(t *testing.T) (revision, currentIndex int, completed, rejections string) {
	t.Helper()
	err := e.pool.QueryRow(`SELECT revision, current_index, completed, rejections FROM agent_step_cursors WHERE workflow_instance_id = ? AND node_id = ?`,
		testWFIID, testAgentType).Scan(&revision, &currentIndex, &completed, &rejections)
	if err != nil {
		t.Fatalf("readCursor: %v", err)
	}
	return revision, currentIndex, completed, rejections
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

// seedStepCursor inserts an agent_step_cursors row for (testWFIID,
// testAgentType) at the given progress, for complete_step tests that need a
// cursor pre-seeded past Snapshot's initial state.
func (e *builtinTestEnv) seedStepCursor(t *testing.T, steps []model.StepDefinition, currentIndex, revision int, completed []model.CompletedStep) {
	t.Helper()
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal steps: %v", err)
	}
	completedJSON, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal completed: %v", err)
	}
	mustExec(t, e.pool, `
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', ?, ?)
		ON CONFLICT(workflow_instance_id, node_id) DO UPDATE SET
			steps_snapshot = excluded.steps_snapshot,
			revision = excluded.revision,
			current_index = excluded.current_index,
			completed = excluded.completed,
			updated_at = excluded.updated_at`,
		testWFIID, testAgentType, string(stepsJSON), revision, currentIndex, string(completedJSON), now, now)
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
