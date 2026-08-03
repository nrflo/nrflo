package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestBuildSessionFlow_DelegateEdge_NestedDepth exercises a caller -> worker
// -> nested worker delegate chain: depth must accumulate hop by hop, not
// reset per delegation.
func TestBuildSessionFlow_DelegateEdge_NestedDepth(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-root", wfiID: wfiID, agentType: "caller", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-worker1", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-worker2", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t2_extractor", status: "completed", startedAt: "2025-01-01T00:02:00Z"})

	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.d1", CallerSessionID: "s-root", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "executor", Fanout: 1, Depth: 1}, 0, "s-worker1")
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.d2", CallerSessionID: "s-worker1", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "extractor", Fanout: 1, Depth: 2}, 0, "s-worker2")

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-root")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	if len(flow.Nodes) != 3 {
		t.Fatalf("len(Nodes) = %d, want 3", len(flow.Nodes))
	}
	depthByID := map[string]int{}
	for _, n := range flow.Nodes {
		depthByID[n.SessionID] = n.Depth
	}
	if depthByID["s-root"] != 0 || depthByID["s-worker1"] != 1 || depthByID["s-worker2"] != 2 {
		t.Errorf("depths = %+v, want root=0 worker1=1 worker2=2", depthByID)
	}
	if len(flow.Edges) != 2 {
		t.Fatalf("len(Edges) = %d, want 2", len(flow.Edges))
	}
	for _, e := range flow.Edges {
		if e.Kind != types.SessionFlowEdgeDelegate {
			t.Errorf("edge kind = %q, want delegate", e.Kind)
		}
	}
}

// TestBuildSessionFlow_ConsultEdge verifies a consult's caller->child
// resolves to a consult-kind edge.
func TestBuildSessionFlow_ConsultEdge(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-consult-caller", wfiID: wfiID, agentType: "analyzer", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-consult-child", wfiID: wfiID, phase: "_consult", nodeID: "_consult",
		agentType: "security-consultant", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertConsult(t, pool, &model.Consult{ID: "consult.flow1", CallerSessionID: "s-consult-caller", WorkflowInstanceID: wfiID, ProjectID: "test-proj", ConsultantID: "security-consultant"}, "s-consult-child")

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-consult-caller")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	if len(flow.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(flow.Nodes))
	}
	if len(flow.Edges) != 1 || flow.Edges[0].Kind != "consult" || flow.Edges[0].ToSessionID != "s-consult-child" {
		t.Errorf("Edges = %+v, want one consult edge to s-consult-child", flow.Edges)
	}
}

// TestBuildSessionFlow_SubworkflowEdge_ParentSessionGuard verifies a
// run_subworkflow child instance's earliest session becomes the flow-graph
// entry node, and that a child instance parented by a DIFFERENT session is
// excluded (the ParentSession guard).
func TestBuildSessionFlow_SubworkflowEdge_ParentSessionGuard(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-parent", wfiID: wfiID, agentType: "analyzer", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	insertTraceSession(t, pool, traceSession{id: "s-other-caller", wfiID: wfiID, agentType: "builder", status: "completed", startedAt: "2025-01-01T00:00:30Z"})

	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, parent_instance_id, parent_session, created_at, updated_at)
		VALUES ('wfi-child-mine', 'test-proj', '', 'test-wf', 'project', 'active', 0, ?, 's-parent', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`, wfiID)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, parent_instance_id, parent_session, created_at, updated_at)
		VALUES ('wfi-child-other', 'test-proj', '', 'test-wf', 'project', 'active', 0, ?, 's-other-caller', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`, wfiID)

	insertTraceSession(t, pool, traceSession{id: "s-child-entry", wfiID: "wfi-child-mine", agentType: "analyzer", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertTraceSession(t, pool, traceSession{id: "s-child-entry-other", wfiID: "wfi-child-other", agentType: "analyzer", status: "completed", startedAt: "2025-01-01T00:01:00Z"})

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-parent")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range flow.Nodes {
		ids[n.SessionID] = true
	}
	if !ids["s-child-entry"] {
		t.Errorf("Nodes = %+v, want s-child-entry present (my own subworkflow child)", flow.Nodes)
	}
	if ids["s-child-entry-other"] {
		t.Errorf("Nodes = %+v, want s-child-entry-other ABSENT (parented by a different session)", flow.Nodes)
	}
}

// TestBuildSessionFlow_OriginEdge_ConsoleLaunchedDynamicRun covers a
// console (run-less) caller session that launched a workflow instance
// attributed via origin_session_id (StartConsoleWorkflow / the hidden
// _delegate_host path) — the instance's earliest session must appear as an
// origin-kind child.
func TestBuildSessionFlow_OriginEdge_ConsoleLaunchedDynamicRun(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)
	now := "2025-01-01T00:00:00Z"
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('console-origin', 'test-proj', '', 'p', 'a', 'user_interactive', 'console', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, origin, origin_session_id, created_at, updated_at)
		VALUES ('wfi-console-launched', 'test-proj', '', 'test-wf', 'project', 'active', 0, 'console', 'console-origin', ?, ?)`, now, now)
	insertTraceSession(t, pool, traceSession{id: "s-console-entry", wfiID: "wfi-console-launched", agentType: "analyzer", status: "running", startedAt: "2025-01-01T00:01:00Z"})

	flow, err := BuildSessionFlow(pool, clock.Real(), "console-origin")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	if len(flow.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2 (console-origin + s-console-entry)", len(flow.Nodes))
	}
	if len(flow.Edges) != 1 || flow.Edges[0].Kind != "origin" || flow.Edges[0].ToSessionID != "s-console-entry" {
		t.Errorf("Edges = %+v, want one origin edge to s-console-entry", flow.Edges)
	}
}

// TestBuildSessionFlow_SiblingEdge covers a console sibling chat opened from
// an origin session (durable sibling_origin_session_id link).
func TestBuildSessionFlow_SiblingEdge(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)
	now := "2025-01-01T00:00:00Z"
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('chat-origin', 'test-proj', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('chat-sibling', 'test-proj', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?)`, now, now)

	if err := repo.NewAgentSessionRepo(pool, clock.Real()).SetSiblingOrigin("chat-sibling", "chat-origin"); err != nil {
		t.Fatalf("SetSiblingOrigin: %v", err)
	}

	flow, err := BuildSessionFlow(pool, clock.Real(), "chat-origin")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	if len(flow.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(flow.Nodes))
	}
	if len(flow.Edges) != 1 || flow.Edges[0].Kind != "sibling" || flow.Edges[0].ToSessionID != "chat-sibling" {
		t.Errorf("Edges = %+v, want one sibling edge to chat-sibling", flow.Edges)
	}
}

// TestBuildSessionFlow_DiamondDedupe_SharedChildCountedOnce verifies a
// session reachable via two independent edges (two delegations fanning into
// the same worker id — a diamond) is visited exactly once, not duplicated.
func TestBuildSessionFlow_DiamondDedupe_SharedChildCountedOnce(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-diamond-root", wfiID: wfiID, agentType: "caller", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-diamond-a", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-diamond-b", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-diamond-shared", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t2_extractor", status: "completed", startedAt: "2025-01-01T00:02:00Z"})

	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.dia1", CallerSessionID: "s-diamond-root", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "executor", Fanout: 2, Depth: 1}, 0, "s-diamond-a")
	// Second slot of the same delegation also resolves to diamond-b.
	if err := repo.NewDelegationRepo(pool, clock.Real()).SetWorkerSlot("wfi-trace.dia1", 1, "s-diamond-b", ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.dia2", CallerSessionID: "s-diamond-a", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "extractor", Fanout: 1, Depth: 2}, 0, "s-diamond-shared")
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.dia3", CallerSessionID: "s-diamond-b", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "extractor", Fanout: 1, Depth: 2}, 0, "s-diamond-shared")

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-diamond-root")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	count := 0
	for _, n := range flow.Nodes {
		if n.SessionID == "s-diamond-shared" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("s-diamond-shared appears %d times in Nodes, want 1 (deduped)", count)
	}
	if len(flow.Nodes) != 4 {
		t.Errorf("len(Nodes) = %d, want 4 (root, a, b, shared once)", len(flow.Nodes))
	}
}

// TestBuildSessionFlow_DepthCap_Truncates builds a delegate chain longer
// than sessionFlowMaxDepth and checks Truncated=true with the walk stopping
// at the cap instead of running away.
func TestBuildSessionFlow_DepthCap_Truncates(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)

	const chainLen = 12 // > sessionFlowMaxDepth (8)
	prev := "s-cap-0"
	insertTraceSession(t, pool, traceSession{id: prev, wfiID: wfiID, agentType: "caller", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	for i := 1; i < chainLen; i++ {
		cur := "s-cap-" + string(rune('a'+i))
		insertSubLaneSession(t, pool, subLaneSession{id: cur, wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
			agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
		insertDelegation(t, pool, &model.Delegation{
			ID: "wfi-trace.cap" + string(rune('a'+i)), CallerSessionID: prev, WorkflowInstanceID: wfiID,
			ProjectID: "test-proj", Tier: "executor", Fanout: 1, Depth: i,
		}, 0, cur)
		prev = cur
	}

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-cap-0")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	if !flow.Truncated {
		t.Error("Truncated = false, want true (chain exceeds sessionFlowMaxDepth)")
	}
	if len(flow.Nodes) > chainLen {
		t.Errorf("len(Nodes) = %d, want <= %d", len(flow.Nodes), chainLen)
	}
	for _, n := range flow.Nodes {
		if n.Depth > 8 {
			t.Errorf("node %s has depth %d, want <= 8 (sessionFlowMaxDepth)", n.SessionID, n.Depth)
		}
	}
}

// TestBuildSessionFlow_UnknownRootSession_BareNodeNoError covers an
// empty/legacy or purged root session id: BuildSessionFlow must return a
// single bare node (no error) rather than failing the request.
func TestBuildSessionFlow_UnknownRootSession_BareNodeNoError(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)

	flow, err := BuildSessionFlow(pool, clock.Real(), "no-such-session")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v, want nil error", err)
	}
	if len(flow.Nodes) != 1 || flow.Nodes[0].SessionID != "no-such-session" {
		t.Errorf("Nodes = %+v, want one bare node for no-such-session", flow.Nodes)
	}
	if len(flow.Edges) != 0 {
		t.Errorf("Edges = %+v, want none", flow.Edges)
	}
	if flow.Truncated {
		t.Error("Truncated = true, want false")
	}
}
