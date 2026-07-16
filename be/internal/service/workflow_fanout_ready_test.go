package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// setupFanoutReadyTestEnv builds a minimal project + active instance. It does
// not seed real fan-out agent_definitions (fan-out isn't implemented in
// production yet) — instead the tests below pass a hand-built []PhaseDef /
// nodeLayers map directly to the read-model functions, which is exactly the
// seam a real fan-out expansion would plug into.
func setupFanoutReadyTestEnv(t *testing.T) (*db.Pool, *WorkflowService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "fanout_ready_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('fo-proj', 'Test', '/tmp', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}
	if _, err = pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('fo-wf', 'fo-proj', '', 'ticket', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("workflow insert: %v", err)
	}
	wfiID := "fo-wfi"
	if _, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES (?, 'fo-proj', '', 'fo-wf', 'ticket', 'active', 0, ?, ?)`,
		wfiID, now, now); err != nil {
		t.Fatalf("workflow_instance insert: %v", err)
	}

	svc := NewWorkflowService(pool, clock.Real())
	return pool, svc, wfiID
}

// TestFanoutReady_ReadModelKeepsSiblingsSeparate is the fan-out-readiness
// assertion: two SpawnerPhaseDefs sharing the same Agent template but
// distinct NodeIDs must produce two active-agent entries, two phase
// statuses, and two trace lanes — proving the node_id/agent_type split
// actually separates execution identity from template identity, not just
// renames a field. This scenario cannot occur from real agent_definitions
// today (no fan-out expansion yet); it exercises the read-model functions
// directly with a hand-built phase list, which is the seam fan-out will use.
func TestFanoutReady_ReadModelKeepsSiblingsSeparate(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFanoutReadyTestEnv(t)

	phases := []PhaseDef{
		{NodeID: "worker#1", Agent: "worker", Layer: 0},
		{NodeID: "worker#2", Agent: "worker", Layer: 0},
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, sess := range []struct{ id, nodeID string }{
		{"sess-worker-1", "worker#1"},
		{"sess-worker-2", "worker#2"},
	} {
		if _, err := pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
				model_id, status, restart_count, started_at, created_at, updated_at)
			VALUES (?, 'fo-proj', '', ?, ?, ?, 'worker', 'claude:sonnet-5', 'running', 0, ?, ?, ?)`,
			sess.id, wfiID, sess.nodeID, sess.nodeID, now, now, now); err != nil {
			t.Fatalf("insert session %s: %v", sess.id, err)
		}
	}

	// 1. Phase statuses: two distinct keys, one per node.
	statuses := svc.derivePhaseStatuses(wfiID, phases)
	if len(statuses) != 2 {
		t.Fatalf("derivePhaseStatuses len = %d, want 2, got %+v", len(statuses), statuses)
	}
	for _, nodeID := range []string{"worker#1", "worker#2"} {
		st, ok := statuses[nodeID]
		if !ok {
			t.Errorf("derivePhaseStatuses missing key %q", nodeID)
			continue
		}
		if st.Status != "in_progress" {
			t.Errorf("derivePhaseStatuses[%q].Status = %q, want in_progress", nodeID, st.Status)
		}
	}

	// 2. Active agents: two distinct map entries, keyed by node_id:model_id.
	active := svc.buildActiveAgentsMap(wfiID, nil)
	if len(active) != 2 {
		t.Fatalf("buildActiveAgentsMap len = %d, want 2, got keys %v", len(active), activeAgentKeys(active))
	}
	for _, key := range []string{"worker#1:claude:sonnet-5", "worker#2:claude:sonnet-5"} {
		entry, ok := active[key]
		if !ok {
			t.Errorf("buildActiveAgentsMap missing key %q, got keys %v", key, activeAgentKeys(active))
			continue
		}
		agent, _ := entry.(map[string]interface{})
		if agent["agent_type"] != "worker" {
			t.Errorf("active[%q].agent_type = %v, want worker", key, agent["agent_type"])
		}
	}

	// 3. Trace lanes: two distinct lanes, correct NodeID each, shared AgentType.
	nodeLayers := map[string]int{"worker#1": 0, "worker#2": 0}
	lanes, _, _ := svc.loadTraceLanes(wfiID, nodeLayers)
	if len(lanes) != 2 {
		t.Fatalf("loadTraceLanes len = %d, want 2", len(lanes))
	}
	seenNodes := map[string]bool{}
	for _, lane := range lanes {
		seenNodes[lane.NodeID] = true
		if lane.AgentType != "worker" {
			t.Errorf("lane %q AgentType = %q, want worker", lane.NodeID, lane.AgentType)
		}
		if lane.Layer != 0 {
			t.Errorf("lane %q Layer = %d, want 0", lane.NodeID, lane.Layer)
		}
	}
	if !seenNodes["worker#1"] || !seenNodes["worker#2"] {
		t.Errorf("trace lanes node ids = %v, want both worker#1 and worker#2", seenNodes)
	}
}

func activeAgentKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
