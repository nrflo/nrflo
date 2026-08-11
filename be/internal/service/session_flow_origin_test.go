package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/types"
)

// TestBuildSessionFlow_OriginInstance_AllSessionsAndDelegateDedupe covers the
// reusable-host shape: an origin-attributed instance with several one-off
// sessions must yield one node per session (not freeze on the earliest), and
// a worker reachable both as a delegate edge and through the host instance's
// origin expansion must keep the single delegate edge.
func TestBuildSessionFlow_OriginInstance_AllSessionsAndDelegateDedupe(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)
	now := "2025-01-01T00:00:00Z"
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('chat-host-origin', 'test-proj', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, origin, origin_session_id, created_at, updated_at)
		VALUES ('wfi-fold-host', 'test-proj', '', '_refinery_fold', 'project', 'active', 0, 'console', 'chat-host-origin', ?, ?)`, now, now)
	insertTraceSession(t, pool, traceSession{id: "s-fold-1", wfiID: "wfi-fold-host", agentType: "_refinery-cli", status: "failed", startedAt: "2025-01-01T00:01:00Z"})
	insertTraceSession(t, pool, traceSession{id: "s-fold-2", wfiID: "wfi-fold-host", agentType: "_refinery-cli", status: "completed", startedAt: "2025-01-01T00:03:00Z"})
	insertTraceSession(t, pool, traceSession{id: "s-host-worker", wfiID: "wfi-fold-host", agentType: "_t2_extractor", status: "completed", startedAt: "2025-01-01T00:02:00Z"})
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-fold-host.d1", CallerSessionID: "chat-host-origin", WorkflowInstanceID: "wfi-fold-host",
		ProjectID: "test-proj", Tier: "extractor", Brief: "Report the working-tree state\nsecond line ignored", Fanout: 1, Depth: 1}, 0, "s-host-worker")

	flow, err := BuildSessionFlow(pool, clock.Real(), "chat-host-origin")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	if len(flow.Nodes) != 4 {
		t.Fatalf("len(Nodes) = %d, want 4 (root + worker + both fold sessions): %+v", len(flow.Nodes), flow.Nodes)
	}
	kinds := map[string]string{}
	for _, e := range flow.Edges {
		if prev, dup := kinds[e.ToSessionID]; dup {
			t.Errorf("duplicate edges to %s (%s + %s), want deduped", e.ToSessionID, prev, e.Kind)
		}
		kinds[e.ToSessionID] = e.Kind
	}
	if kinds["s-host-worker"] != types.SessionFlowEdgeDelegate {
		t.Errorf("s-host-worker edge kind = %q, want delegate (wins over origin)", kinds["s-host-worker"])
	}
	if kinds["s-fold-1"] != types.SessionFlowEdgeOrigin || kinds["s-fold-2"] != types.SessionFlowEdgeOrigin {
		t.Errorf("fold session edges = %+v, want origin edges to both", kinds)
	}
	titles := map[string]string{}
	for _, n := range flow.Nodes {
		titles[n.SessionID] = n.Title
	}
	if titles["s-host-worker"] != "Report the working-tree state" {
		t.Errorf("worker title = %q, want the brief's first line", titles["s-host-worker"])
	}
	if titles["s-fold-1"] != "" {
		t.Errorf("fold session title = %q, want empty (hidden _-prefixed workflow)", titles["s-fold-1"])
	}
}
