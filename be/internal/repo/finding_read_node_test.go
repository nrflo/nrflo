package repo

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

type nodeReadEnv struct {
	repo  *FindingRepo
	pool  *db.Pool
	wfiID string
}

// setupNodeReadDB creates a project/workflow/instance for GetByNode tests.
func setupNodeReadDB(t *testing.T) *nodeReadEnv {
	t.Helper()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	exec := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := pool.Exec(q, args...); err != nil {
			t.Fatalf("setupNodeReadDB: %v", err)
		}
	}

	exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-n', 'Test', ?, ?)`, now, now)
	exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('proj-n', 'wf-n', 'Test wf', 'ticket', ?, ?)`, now, now)

	wfiID := "wfi-node-test"
	exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		VALUES (?, 'proj-n', 'tkt-n', 'wf-n', 'ticket', 'active', ?, ?)`, wfiID, now, now)

	return &nodeReadEnv{repo: NewFindingRepo(pool, clk), pool: pool, wfiID: wfiID}
}

// insertNodeReadSession inserts an agent_sessions row for the given wfiID with an
// explicit node_id (may be empty to exercise the legacy fallback to phase).
// An empty endedAt leaves ended_at NULL (still running).
func insertNodeReadSession(t *testing.T, pool *db.Pool, sessionID, wfiID, nodeID, phase, agentType, status, result, endedAt string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var endedAtVal, resultVal interface{}
	if endedAt != "" {
		endedAtVal = endedAt
	}
	if result != "" {
		resultVal = result
	}
	_, err := pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			status, result, result_reason, pid, context_left, ancestor_session_id,
			spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'proj-n', 'tkt-n', ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, ?, ?, ?)`,
		sessionID, wfiID, phase, nodeID, agentType, status, resultVal, now, endedAtVal, now, now)
	if err != nil {
		t.Fatalf("insertNodeReadSession(%s): %v", sessionID, err)
	}
}

// TestFindingRepo_GetByNode_UnknownNode verifies exists=false and a nil map
// for a node with no agent_sessions row at all, distinguishing "unknown node"
// from "known node with zero findings".
func TestFindingRepo_GetByNode_UnknownNode(t *testing.T) {
	t.Parallel()
	env := setupNodeReadDB(t)

	result, exists, err := env.repo.GetByNode(env.wfiID, "ghost-node")
	if err != nil {
		t.Fatalf("GetByNode: %v", err)
	}
	if exists {
		t.Error("expected exists=false for a node with no sessions")
	}
	if result != nil {
		t.Errorf("expected nil map for unknown node, got %v", result)
	}
}

// TestFindingRepo_GetByNode_KnownNodeNoFindings verifies exists=true with an
// empty (non-nil-vs-nil distinguishable) map when a session exists but wrote
// no findings.
func TestFindingRepo_GetByNode_KnownNodeNoFindings(t *testing.T) {
	t.Parallel()
	env := setupNodeReadDB(t)

	insertNodeReadSession(t, env.pool, "sess-empty", env.wfiID, "empty-node", "empty-node", "worker", "completed", "pass", "2025-06-01T00:00:01Z")

	result, exists, err := env.repo.GetByNode(env.wfiID, "empty-node")
	if err != nil {
		t.Fatalf("GetByNode: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true for a known node")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// TestFindingRepo_GetByNode_LegacyRowFallsBackToPhase verifies a row with
// node_id=” (pre-DYNWF-2, or never explicitly set) is still reachable by its
// phase value, matching the CreateSession legacy fallback.
func TestFindingRepo_GetByNode_LegacyRowFallsBackToPhase(t *testing.T) {
	t.Parallel()
	env := setupNodeReadDB(t)

	insertNodeReadSession(t, env.pool, "sess-legacy", env.wfiID, "" /* node_id */, "legacy-agent", "legacy-agent", "completed", "pass", "2025-06-01T00:00:01Z")
	env.repo.Upsert("session", "sess-legacy", "k", json.RawMessage(`"legacy-value"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "legacy-agent"}, Actor{Source: "agent"})

	result, exists, err := env.repo.GetByNode(env.wfiID, "legacy-agent")
	if err != nil {
		t.Fatalf("GetByNode: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true via phase fallback")
	}
	if string(result["k"]) != `"legacy-value"` {
		t.Errorf("result[k] = %s, want \"legacy-value\"", result["k"])
	}
}

// TestFindingRepo_GetByNode_EndedBeatsRunning verifies the most-recently-ended
// session's value wins on key collision, and a still-running session never
// shadows an ended one.
func TestFindingRepo_GetByNode_EndedBeatsRunning(t *testing.T) {
	t.Parallel()
	env := setupNodeReadDB(t)

	insertNodeReadSession(t, env.pool, "sess-early", env.wfiID, "flaky-node", "flaky-node", "flaky", "completed", "pass", "2025-06-01T00:00:01Z")
	env.repo.Upsert("session", "sess-early", "status", json.RawMessage(`"first"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "flaky"}, Actor{Source: "agent"})

	insertNodeReadSession(t, env.pool, "sess-late", env.wfiID, "flaky-node", "flaky-node", "flaky", "completed", "pass", "2025-06-01T00:00:05Z")
	env.repo.Upsert("session", "sess-late", "status", json.RawMessage(`"second"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "flaky"}, Actor{Source: "agent"})

	insertNodeReadSession(t, env.pool, "sess-running", env.wfiID, "flaky-node", "flaky-node", "flaky", "running", "", "")
	env.repo.Upsert("session", "sess-running", "status", json.RawMessage(`"running-retry"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "flaky"}, Actor{Source: "agent"})

	result, exists, err := env.repo.GetByNode(env.wfiID, "flaky-node")
	if err != nil {
		t.Fatalf("GetByNode: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
	if string(result["status"]) != `"second"` {
		t.Errorf("result[status] = %s, want \"second\" (most-recently-ended session wins, running never shadows)", result["status"])
	}
}

// TestFindingRepo_GetByNode_InstanceIsolation verifies a node id shared across
// two different workflow instances does not leak findings between them.
func TestFindingRepo_GetByNode_InstanceIsolation(t *testing.T) {
	t.Parallel()
	env := setupNodeReadDB(t)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		VALUES ('wfi-node-test-2', 'proj-n', 'tkt-n', 'wf-n', 'ticket', 'active', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert second wfi: %v", err)
	}

	insertNodeReadSession(t, env.pool, "sess-a", env.wfiID, "shared-node", "shared-node", "worker", "completed", "pass", "2025-06-01T00:00:01Z")
	env.repo.Upsert("session", "sess-a", "k", json.RawMessage(`"from-instance-a"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "worker"}, Actor{Source: "agent"})

	insertNodeReadSession(t, env.pool, "sess-b", "wfi-node-test-2", "shared-node", "shared-node", "worker", "completed", "pass", "2025-06-01T00:00:01Z")
	env.repo.Upsert("session", "sess-b", "k", json.RawMessage(`"from-instance-b"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: "wfi-node-test-2", AgentType: "worker"}, Actor{Source: "agent"})

	result, exists, err := env.repo.GetByNode(env.wfiID, "shared-node")
	if err != nil {
		t.Fatalf("GetByNode: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
	if string(result["k"]) != `"from-instance-a"` {
		t.Errorf("result[k] = %s, want \"from-instance-a\" (must not leak from other instance)", result["k"])
	}
}
