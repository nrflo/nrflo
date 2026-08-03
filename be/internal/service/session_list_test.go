package service

import (
	"testing"

	"be/internal/clock"
)

func TestListSessions_ScopesToProject(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('other-proj', 'Other', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`)
	insertTraceSession(t, pool, traceSession{id: "s-list-1", wfiID: wfiID, agentType: "analyzer", status: "completed", result: "pass", startedAt: "2025-01-01T00:00:00Z"})
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-list-other', 'other-proj', '', 'p', 'a', 'running', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`)

	resp, err := ListSessions(pool, clock.Real(), "test-proj", 0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].SessionID != "s-list-1" {
		t.Errorf("Sessions = %+v, want only s-list-1", resp.Sessions)
	}
	if resp.Sessions[0].Workflow != "test-wf" {
		t.Errorf("Workflow = %q, want test-wf", resp.Sessions[0].Workflow)
	}
	if resp.Sessions[0].Result != "pass" {
		t.Errorf("Result = %q, want pass", resp.Sessions[0].Result)
	}
}

func TestListSessions_CostEstimateOnlySetWhenValid(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-nocost", wfiID: wfiID, agentType: "analyzer", status: "running", startedAt: "2025-01-01T00:00:00Z"})

	resp, err := ListSessions(pool, clock.Real(), "test-proj", 0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("len(Sessions) = %d, want 1", len(resp.Sessions))
	}
	if resp.Sessions[0].CostEstimate != nil {
		t.Errorf("CostEstimate = %v, want nil for a session with no cost recorded", *resp.Sessions[0].CostEstimate)
	}
}

func TestListSessionsGlobal_ReturnsAcrossProjects(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('other-proj-g', 'Other', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`)
	insertTraceSession(t, pool, traceSession{id: "s-glob-1", wfiID: wfiID, agentType: "analyzer", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
		VALUES ('s-glob-2', 'other-proj-g', '', 'p', 'a', 'running', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`)

	resp, err := ListSessionsGlobal(pool, clock.Real(), 0)
	if err != nil {
		t.Fatalf("ListSessionsGlobal: %v", err)
	}
	seen := map[string]bool{}
	for _, s := range resp.Sessions {
		seen[s.SessionID] = true
	}
	if !seen["s-glob-1"] || !seen["s-glob-2"] {
		t.Errorf("Sessions = %+v, want both s-glob-1 and s-glob-2", resp.Sessions)
	}
}
