package repo

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// setupLayerNodeReadDB mirrors setupNodeReadDB with its own project/workflow
// namespace so GetByLayer roster tests don't collide with GetByNode tests.
func setupLayerNodeReadDB(t *testing.T) *nodeReadEnv {
	t.Helper()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	exec := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := pool.Exec(q, args...); err != nil {
			t.Fatalf("setupLayerNodeReadDB: %v", err)
		}
	}

	exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-ln', 'Test', ?, ?)`, now, now)
	exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('proj-ln', 'wf-ln', 'Test wf', 'ticket', ?, ?)`, now, now)

	wfiID := "wfi-layer-node-test"
	exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		VALUES (?, 'proj-ln', 'tkt-ln', 'wf-ln', 'ticket', 'active', ?, ?)`, wfiID, now, now)

	return &nodeReadEnv{repo: NewFindingRepo(pool, clk), pool: pool, wfiID: wfiID}
}

// insertLayerAgentDef inserts an agent_definitions row with explicit
// consultant/node_role, so exclusion from the executable roster can be tested.
func insertLayerAgentDef(t *testing.T, pool *db.Pool, id string, layer, consultant int, nodeRole string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO agent_definitions (id, project_id, workflow_id, layer, model, prompt, tag, low_consumption_model, tools, consultant, node_role, created_at, updated_at)
		VALUES (?, 'proj-ln', 'wf-ln', ?, 'sonnet-5', '', '', '', '', ?, ?, ?, ?)`,
		id, layer, consultant, nodeRole, now, now)
	if err != nil {
		t.Fatalf("insertLayerAgentDef(%s): %v", id, err)
	}
}

// TestFindingRepo_GetByLayer_FanOutSiblingsDisjoint verifies two sessions that
// share one agent_type but carry distinct node_ids are bucketed under
// separate node_id keys with disjoint findings, plus a nil pending entry for
// the def's own id (no session was ever attributed to that literal node_id).
func TestFindingRepo_GetByLayer_FanOutSiblingsDisjoint(t *testing.T) {
	t.Parallel()
	env := setupLayerNodeReadDB(t)
	insertLayerAgentDef(t, env.pool, "worker", 1, 0, "static")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	insert := func(sessionID, nodeID string) {
		if _, err := env.pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
				status, result, result_reason, pid, context_left, ancestor_session_id,
				spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
			VALUES (?, 'proj-ln', 'tkt-ln', ?, ?, ?, 'worker', 'completed', 'pass', NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, ?, ?, ?)`,
			sessionID, env.wfiID, nodeID, nodeID, now, now, now, now); err != nil {
			t.Fatalf("insert session %s: %v", sessionID, err)
		}
	}
	insert("sess-worker-1", "worker#1")
	insert("sess-worker-2", "worker#2")

	env.repo.Upsert("session", "sess-worker-1", "picked", json.RawMessage(`"g2"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "worker"}, Actor{Source: "agent"})
	env.repo.Upsert("session", "sess-worker-2", "picked", json.RawMessage(`"reddit"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "worker"}, Actor{Source: "agent"})

	byNode, err := env.repo.GetByLayer(env.wfiID, 1)
	if err != nil {
		t.Fatalf("GetByLayer: %v", err)
	}

	w1, ok := byNode["worker#1"]
	if !ok || w1 == nil || string(w1["picked"]) != `"g2"` {
		t.Errorf("worker#1 = %v, want map{picked: g2}", byNode["worker#1"])
	}
	w2, ok := byNode["worker#2"]
	if !ok || w2 == nil || string(w2["picked"]) != `"reddit"` {
		t.Errorf("worker#2 = %v, want map{picked: reddit}", byNode["worker#2"])
	}

	// The def's own id has no session bearing that literal node_id, so it
	// remains a pending (nil) roster line — distinct from its fanned-out siblings.
	if v, exists := byNode["worker"]; !exists || v != nil {
		t.Errorf("worker (def id) = %v, exists=%v; want nil, exists=true", v, exists)
	}
}

// TestFindingRepo_GetByLayer_ExcludesConsultantAndNonStaticDefs verifies a
// consultant def and a node_role!='static' def at the same layer never appear
// in the roster, and sessions attributed to their agent_type are excluded too.
func TestFindingRepo_GetByLayer_ExcludesConsultantAndNonStaticDefs(t *testing.T) {
	t.Parallel()
	env := setupLayerNodeReadDB(t)
	insertLayerAgentDef(t, env.pool, "static-node", 2, 0, "static")
	insertLayerAgentDef(t, env.pool, "consult-node", 2, 1, "static")
	insertLayerAgentDef(t, env.pool, "planner-node", 2, 0, "planner")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertSess := func(sessionID, agentType string) {
		if _, err := env.pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
				status, result, result_reason, pid, context_left, ancestor_session_id,
				spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
			VALUES (?, 'proj-ln', 'tkt-ln', ?, ?, ?, ?, 'completed', 'pass', NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, ?, ?, ?)`,
			sessionID, env.wfiID, agentType, agentType, agentType, now, now, now, now); err != nil {
			t.Fatalf("insert session %s: %v", sessionID, err)
		}
	}
	insertSess("sess-static", "static-node")
	insertSess("sess-consult", "consult-node")
	insertSess("sess-planner", "planner-node")

	byNode, err := env.repo.GetByLayer(env.wfiID, 2)
	if err != nil {
		t.Fatalf("GetByLayer: %v", err)
	}

	if len(byNode) != 1 {
		t.Fatalf("roster len = %d, want 1 (only static-node); got keys: %v", len(byNode), sortedKeys(byNode))
	}
	if _, ok := byNode["static-node"]; !ok {
		t.Error("expected 'static-node' in roster")
	}
	if _, ok := byNode["consult-node"]; ok {
		t.Error("consultant def leaked into roster")
	}
	if _, ok := byNode["planner-node"]; ok {
		t.Error("node_role='planner' def leaked into roster")
	}
}
