package repo

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"be/internal/clock"
)

func TestFirstSessionForInstance_ReturnsEarliestByStartedAt(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-fsfi', 'P', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('wf-fsfi', 'proj-fsfi', '', 'ticket', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES ('wfi-fsfi', 'proj-fsfi', '', 'wf-fsfi', 'active', 'ticket', ?, ?)`, now, now)

	t0, t1 := "2025-01-01T00:00:00Z", "2025-01-01T00:01:00Z"
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-second', 'proj-fsfi', '', 'wfi-fsfi', 'p', 'a', 'running', ?, ?, ?)`, t1, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-first', 'proj-fsfi', '', 'wfi-fsfi', 'p', 'a', 'completed', ?, ?, ?)`, t0, now, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	got, err := r.FirstSessionForInstance("wfi-fsfi")
	if err != nil {
		t.Fatalf("FirstSessionForInstance: %v", err)
	}
	if got.ID != "s-first" {
		t.Errorf("ID = %q, want s-first (earliest started_at)", got.ID)
	}
}

func TestFirstSessionForInstance_FallsBackToCreatedAtWhenStartedAtNull(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-fsfi2', 'P', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('wf-fsfi2', 'proj-fsfi2', '', 'ticket', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES ('wfi-fsfi2', 'proj-fsfi2', '', 'wf-fsfi2', 'active', 'ticket', ?, ?)`, now, now)

	created0, created1 := "2025-01-01T00:00:00Z", "2025-01-01T00:01:00Z"
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at)
		VALUES ('s-c1', 'proj-fsfi2', '', 'wfi-fsfi2', 'p', 'a', 'running', ?, ?)`, created1, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at)
		VALUES ('s-c0', 'proj-fsfi2', '', 'wfi-fsfi2', 'p', 'a', 'running', ?, ?)`, created0, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	got, err := r.FirstSessionForInstance("wfi-fsfi2")
	if err != nil {
		t.Fatalf("FirstSessionForInstance: %v", err)
	}
	if got.ID != "s-c0" {
		t.Errorf("ID = %q, want s-c0 (earliest created_at fallback)", got.ID)
	}
}

func TestFirstSessionForInstance_NoSessions_ReturnsErrNoRows(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewAgentSessionRepo(pool, clock.Real())
	_, err := r.FirstSessionForInstance("no-such-wfi")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestCostTokenRollup_SumsAcrossSessions(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-rollup', 'P', ?, ?)`, now, now)

	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, cost_estimate, tokens_json, created_at, updated_at)
		VALUES ('s-r1', 'proj-rollup', '', 'p', 'a', 'completed', 1.5,
		'{"input_tokens":100,"output_tokens":50,"cache_read_tokens":10,"cache_write_tokens":5}', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, cost_estimate, tokens_json, created_at, updated_at)
		VALUES ('s-r2', 'proj-rollup', '', 'p', 'a', 'completed', 2.5,
		'{"input_tokens":200,"output_tokens":0,"cache_read_tokens":0,"cache_write_tokens":0}', ?, ?)`, now, now)
	// Not in the requested set — must not be summed in.
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, cost_estimate, created_at, updated_at)
		VALUES ('s-r-excluded', 'proj-rollup', '', 'p', 'a', 'completed', 999, ?, ?)`, now, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	cost, tokens, err := r.CostTokenRollup([]string{"s-r1", "s-r2"})
	if err != nil {
		t.Fatalf("CostTokenRollup: %v", err)
	}
	if cost != 4.0 {
		t.Errorf("cost = %v, want 4.0", cost)
	}
	if tokens != 365 {
		t.Errorf("tokens = %v, want 365 (100+50+10+5+200)", tokens)
	}
}

func TestCostTokenRollup_EmptyIDs_ReturnsZero(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewAgentSessionRepo(pool, clock.Real())
	cost, tokens, err := r.CostTokenRollup(nil)
	if err != nil {
		t.Fatalf("CostTokenRollup: %v", err)
	}
	if cost != 0 || tokens != 0 {
		t.Errorf("cost=%v tokens=%v, want 0/0", cost, tokens)
	}
}

func TestCostTokenRollup_NullCostAndTokens_TreatedAsZero(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-rollup-null', 'P', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, created_at, updated_at)
		VALUES ('s-null', 'proj-rollup-null', '', 'p', 'a', 'running', ?, ?)`, now, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	cost, tokens, err := r.CostTokenRollup([]string{"s-null"})
	if err != nil {
		t.Fatalf("CostTokenRollup: %v", err)
	}
	if cost != 0 || tokens != 0 {
		t.Errorf("cost=%v tokens=%v, want 0/0 for a session with no cost/tokens recorded yet", cost, tokens)
	}
}
