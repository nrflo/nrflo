package service

import (
	"testing"
	"time"

	"be/internal/model"
)

func TestConsoleSweepIdle_DefaultTTL(t *testing.T) {
	t.Parallel()
	pool, svc, clk := setupConsoleServiceTestEnv(t)

	staleID, _, err := svc.CreateSession("proj1")
	if err != nil {
		t.Fatalf("CreateSession stale: %v", err)
	}
	freshID, _, err := svc.CreateSession("proj1")
	if err != nil {
		t.Fatalf("CreateSession fresh: %v", err)
	}

	now := clk.Now()
	backdateSessionUpdatedAt(t, pool, staleID, now.Add(-(DefaultConsoleIdleTTLHours+1)*time.Hour))
	backdateSessionUpdatedAt(t, pool, freshID, now.Add(-time.Hour))

	n, err := svc.SweepIdle(now)
	if err != nil {
		t.Fatalf("SweepIdle: %v", err)
	}
	if n != 1 {
		t.Fatalf("SweepIdle expired = %d, want 1", n)
	}

	var staleStatus, freshStatus string
	if err := pool.QueryRow(`SELECT status FROM agent_sessions WHERE id = ?`, staleID).Scan(&staleStatus); err != nil {
		t.Fatalf("query stale: %v", err)
	}
	if staleStatus != string(model.AgentSessionInteractiveCompleted) {
		t.Errorf("stale status = %q, want interactive_completed", staleStatus)
	}
	if err := pool.QueryRow(`SELECT status FROM agent_sessions WHERE id = ?`, freshID).Scan(&freshStatus); err != nil {
		t.Fatalf("query fresh: %v", err)
	}
	if freshStatus != string(model.AgentSessionUserInteractive) {
		t.Errorf("fresh status = %q, want unchanged user_interactive", freshStatus)
	}
}

func TestConsoleSweepIdle_CustomTTLViaConfig(t *testing.T) {
	t.Parallel()
	pool, svc, clk := setupConsoleServiceTestEnv(t)

	if err := pool.SetConfig(ConsoleIdleTTLHoursKey, "1"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	sessionID, _, err := svc.CreateSession("proj1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	now := clk.Now()
	// 90 minutes idle: stale under the 1h custom TTL, but would be fresh under
	// the 12h default — proves the config override is actually read.
	backdateSessionUpdatedAt(t, pool, sessionID, now.Add(-90*time.Minute))

	n, err := svc.SweepIdle(now)
	if err != nil {
		t.Fatalf("SweepIdle: %v", err)
	}
	if n != 1 {
		t.Fatalf("SweepIdle expired = %d, want 1 under custom 1h TTL", n)
	}
}

func TestConsoleSweepIdle_LeavesWorkflowAndObserverRowsAlone(t *testing.T) {
	t.Parallel()
	pool, svc, clk := setupConsoleServiceTestEnv(t)

	now := clk.Now()
	staleTs := now.Add(-(DefaultConsoleIdleTTLHours + 1) * time.Hour).UTC().Format(time.RFC3339Nano)

	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj1', 'wf1', '', 'ticket', ?, ?)`, staleTs, staleTs); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES ('wfi-1', 'proj1', '', 'wf1', 'active', 'ticket', ?, ?)`, staleTs, staleTs); err != nil {
		t.Fatalf("insert wfi: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, created_at, updated_at)
		VALUES ('wf-agent-1', 'proj1', '', 'wfi-1', 'p', 'a', 'sonnet', 'user_interactive', 'workflow_agent', ?, ?)`, staleTs, staleTs); err != nil {
		t.Fatalf("insert workflow agent session: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, observer_scope, created_at, updated_at)
		VALUES ('obs-1', 'proj1', '', NULL, 'observer', '_observer', 'sonnet', 'user_interactive', 'observer', 'global', ?, ?)`, staleTs, staleTs); err != nil {
		t.Fatalf("insert observer session: %v", err)
	}

	n, err := svc.SweepIdle(now)
	if err != nil {
		t.Fatalf("SweepIdle: %v", err)
	}
	if n != 0 {
		t.Fatalf("SweepIdle expired = %d, want 0 (no console rows exist)", n)
	}

	for _, id := range []string{"wf-agent-1", "obs-1"} {
		var status string
		if err := pool.QueryRow(`SELECT status FROM agent_sessions WHERE id = ?`, id).Scan(&status); err != nil {
			t.Fatalf("query %s: %v", id, err)
		}
		if status != "user_interactive" {
			t.Errorf("%s status = %q, want unchanged user_interactive", id, status)
		}
	}
}
