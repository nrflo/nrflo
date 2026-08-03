package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// mustExec runs a write query against pool, failing the test on error.
// Fixtures in this file (and its session_flow_query_test.go /
// tool_dispatch_query_test.go siblings) seed rows via raw SQL rather than
// AgentSessionRepo.Create — see be_production_bugs: Create()'s INSERT is
// column/placeholder-mismatched after migration 000230 and errors on every
// call.
func mustExec(t *testing.T, pool *db.Pool, query string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\nquery: %s", err, query)
	}
}

func TestListSessionSummaries_ScopesToProjectNewestFirst(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-a', 'A', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-b', 'B', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('wf', 'proj-a', '', 'ticket', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES ('wfi-1', 'proj-a', '', 'wf', 'active', 'ticket', ?, ?)`, now, now)

	t0, t1, t2 := "2025-01-01T00:00:00Z", "2025-01-01T00:01:00Z", "2025-01-01T00:02:00Z"
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, cost_estimate, started_at, created_at, updated_at)
		VALUES ('s-old', 'proj-a', '', 'wfi-1', 'p', 'builder', 'completed', 0.5, ?, ?, ?)`, t0, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-new', 'proj-a', '', 'wfi-1', 'p', 'analyzer', 'running', ?, ?, ?)`, t1, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-other-proj', 'proj-b', '', 'p', 'analyzer', 'running', ?, ?, ?)`, t2, now, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	rows, err := r.ListSessionSummaries("proj-a", 0)
	if err != nil {
		t.Fatalf("ListSessionSummaries: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (scoped to proj-a)", len(rows))
	}
	if rows[0].SessionID != "s-new" || rows[1].SessionID != "s-old" {
		t.Errorf("order = %s,%s, want s-new,s-old (started_at DESC)", rows[0].SessionID, rows[1].SessionID)
	}
	if rows[1].WorkflowID.String != "wf" {
		t.Errorf("WorkflowID = %q, want wf (joined from workflow_instances)", rows[1].WorkflowID.String)
	}
	if !rows[1].CostEstimate.Valid || rows[1].CostEstimate.Float64 != 0.5 {
		t.Errorf("CostEstimate = %+v, want valid 0.5", rows[1].CostEstimate)
	}
}

func TestListSessionSummaries_LimitDefaultsTo100(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-lim', 'L', ?, ?)`, now, now)
	for i := 0; i < 3; i++ {
		mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
			VALUES (?, 'proj-lim', '', 'p', 'a', 'running', ?, ?, ?)`,
			"s-lim-"+string(rune('a'+i)), now, now, now)
	}

	r := NewAgentSessionRepo(pool, clock.Real())
	rows, err := r.ListSessionSummaries("proj-lim", 2)
	if err != nil {
		t.Fatalf("ListSessionSummaries: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("len(rows) = %d, want 2 (explicit limit honored)", len(rows))
	}

	all, err := r.ListSessionSummaries("proj-lim", 0)
	if err != nil {
		t.Fatalf("ListSessionSummaries(limit=0): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3 (limit<=0 defaults, does not truncate a small set)", len(all))
	}
}

func TestListSessionSummaries_UnknownProject_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewAgentSessionRepo(pool, clock.Real())
	rows, err := r.ListSessionSummaries("no-such-project", 0)
	if err != nil {
		t.Fatalf("ListSessionSummaries: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0", len(rows))
	}
}

func TestListSessionSummariesGlobal_ReturnsAcrossProjects(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-g1', 'G1', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-g2', 'G2', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-g1', 'proj-g1', '', 'p', 'a', 'running', ?, ?, ?)`, now, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-g2', 'proj-g2', '', 'p', 'a', 'running', ?, ?, ?)`, now, now, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	rows, err := r.ListSessionSummariesGlobal(0)
	if err != nil {
		t.Fatalf("ListSessionSummariesGlobal: %v", err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.SessionID] = true
	}
	if !seen["s-g1"] || !seen["s-g2"] {
		t.Errorf("rows = %+v, want both s-g1 and s-g2 present (cross-project)", rows)
	}
}
