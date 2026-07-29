package service

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// subLaneSession describes one agent_sessions fixture row shaped like a
// delegate worker or consult child: phase/node_id are the underscore-prefixed
// launcher marker rather than the agent_type, per the confirmed session
// shapes (delegate.go:23-24,169,224; consult_run.go:66,120).
type subLaneSession struct {
	id, wfiID, phase, nodeID, agentType, status, startedAt, endedAt string
}

func insertSubLaneSession(t *testing.T, pool *db.Pool, s subLaneSession) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			status, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-proj', '', ?, ?, ?, ?, ?, 0, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		s.id, s.wfiID, s.phase, s.nodeID, s.agentType, s.status, s.startedAt, s.endedAt, now, now)
}

func insertDelegation(t *testing.T, pool *db.Pool, d *model.Delegation, workerIdx int, workerSessionID string) {
	t.Helper()
	r := repo.NewDelegationRepo(pool, clock.Real())
	if err := r.Create(d); err != nil {
		t.Fatalf("DelegationRepo.Create: %v", err)
	}
	if err := r.SetWorkerSlot(d.ID, workerIdx, workerSessionID, ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}
}

func insertConsult(t *testing.T, pool *db.Pool, c *model.Consult, childSessionID string) {
	t.Helper()
	r := repo.NewConsultRepo(pool, clock.Real())
	if err := r.Create(c); err != nil {
		t.Fatalf("ConsultRepo.Create: %v", err)
	}
	if err := r.SetChildSession(c.ID, childSessionID); err != nil {
		t.Fatalf("SetChildSession: %v", err)
	}
}

func TestBuildTrace_DelegateWorker_AppearsInSubLanesNotLanes(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-caller", wfiID: wfiID, agentType: "analyzer",
		status: "running", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-worker", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:01:00Z", endedAt: "2025-01-01T00:02:00Z"})
	insertDelegation(t, pool, &model.Delegation{
		ID: "wfi-trace.deleg1", CallerSessionID: "s-caller", WorkflowInstanceID: wfiID, ProjectID: "test-proj",
		Tier: "executor", Fanout: 1, Depth: 1,
	}, 0, "s-worker")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	for _, l := range trace.Lanes {
		if l.LaneID == "s-worker" {
			t.Fatalf("delegate worker session leaked into Lanes: %+v", l)
		}
	}
	if len(trace.SubLanes) != 1 {
		t.Fatalf("SubLanes = %d, want 1", len(trace.SubLanes))
	}
	sl := trace.SubLanes[0]
	if sl.LaneID != "s-worker" || sl.Kind != "delegate" || sl.DelegationID != "wfi-trace.deleg1" {
		t.Errorf("sub-lane = %+v, want s-worker/delegate/wfi-trace.deleg1", sl)
	}
	if sl.ParentLaneID != "s-caller" {
		t.Errorf("ParentLaneID = %q, want s-caller", sl.ParentLaneID)
	}
	if sl.Depth != 1 {
		t.Errorf("Depth = %d, want 1", sl.Depth)
	}
	if sl.AgentType != "_t1_executor" {
		t.Errorf("AgentType = %q, want _t1_executor", sl.AgentType)
	}
}

func TestBuildTrace_ConsultChild_AppearsAsConsultSubLane(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-caller", wfiID: wfiID, agentType: "analyzer",
		status: "running", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-consultant", wfiID: wfiID, phase: "_consult", nodeID: "_consult",
		agentType: "security-consultant", status: "completed", startedAt: "2025-01-01T00:01:00Z", endedAt: "2025-01-01T00:01:30Z"})
	insertConsult(t, pool, &model.Consult{
		ID: "consult.c1", CallerSessionID: "s-caller", WorkflowInstanceID: wfiID, ProjectID: "test-proj",
		ConsultantID: "security-consultant", Question: "is this safe?",
	}, "s-consultant")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if len(trace.SubLanes) != 1 {
		t.Fatalf("SubLanes = %d, want 1", len(trace.SubLanes))
	}
	sl := trace.SubLanes[0]
	if sl.Kind != "consult" || sl.ConsultID != "consult.c1" {
		t.Errorf("sub-lane = %+v, want kind=consult consult_id=consult.c1", sl)
	}
	if sl.ParentLaneID != "s-caller" {
		t.Errorf("ParentLaneID = %q, want s-caller", sl.ParentLaneID)
	}
	if sl.DelegationID != "" {
		t.Errorf("DelegationID = %q, want empty for a consult sub-lane", sl.DelegationID)
	}
	for _, l := range trace.Lanes {
		if l.LaneID == "s-consultant" {
			t.Fatalf("consult child leaked into Lanes: %+v", l)
		}
	}
}

// TestBuildTrace_TransientSessions_ExcludedFromLanesAndSubLanes is the
// regression guard proving the underscore-session exclusion was not blanket-
// removed: planner/context-saver/conflict-resolver stay hidden from both
// Lanes AND SubLanes when they are not referenced by any delegations/consults
// row (sub-lanes are selected by explicit session-id list, not a loosened
// WHERE).
func TestBuildTrace_TransientSessions_ExcludedFromLanesAndSubLanes(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-a", wfiID: wfiID, agentType: "analyzer",
		status: "completed", result: "pass", startedAt: "2025-01-01T00:00:00Z", endedAt: "2025-01-01T00:01:00Z"})
	for _, hidden := range []string{"planner", "context-saver", "conflict-resolver", "_observer"} {
		insertTraceSession(t, pool, traceSession{id: "s-" + hidden, wfiID: wfiID, agentType: hidden,
			status: "completed", result: "pass", startedAt: "2025-01-01T00:00:00Z", endedAt: "2025-01-01T00:01:00Z"})
	}

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if len(trace.Lanes) != 1 || trace.Lanes[0].LaneID != "s-a" {
		t.Fatalf("Lanes = %+v, want only s-a", trace.Lanes)
	}
	if len(trace.SubLanes) != 0 {
		t.Fatalf("SubLanes = %+v, want none (no delegations/consults rows reference these sessions)", trace.SubLanes)
	}
}

func TestBuildTrace_NestedDelegation_T2ParentsOntoT1SubLane(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-caller", wfiID: wfiID, agentType: "analyzer",
		status: "running", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-t1", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "running", startedAt: "2025-01-01T00:01:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-t2", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t2_extractor", status: "completed", startedAt: "2025-01-01T00:02:00Z", endedAt: "2025-01-01T00:02:30Z"})
	// depth 1: root caller -> T1 worker. depth 2: T1 worker -> T2 worker.
	insertDelegation(t, pool, &model.Delegation{
		ID: "wfi-trace.deleg-t1", CallerSessionID: "s-caller", WorkflowInstanceID: wfiID, ProjectID: "test-proj",
		Tier: "executor", Fanout: 1, Depth: 1,
	}, 0, "s-t1")
	insertDelegation(t, pool, &model.Delegation{
		ID: "wfi-trace.deleg-t2", CallerSessionID: "s-t1", WorkflowInstanceID: wfiID, ProjectID: "test-proj",
		Tier: "extractor", Fanout: 1, Depth: 2,
	}, 0, "s-t2")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if len(trace.SubLanes) != 2 {
		t.Fatalf("SubLanes = %d, want 2", len(trace.SubLanes))
	}
	var t1, t2 *types.TraceLane
	for i := range trace.SubLanes {
		switch trace.SubLanes[i].LaneID {
		case "s-t1":
			t1 = &trace.SubLanes[i]
		case "s-t2":
			t2 = &trace.SubLanes[i]
		}
	}
	if t1 == nil || t2 == nil {
		t.Fatalf("expected both s-t1 and s-t2 sub-lanes, got %+v", trace.SubLanes)
	}
	if t1.ParentLaneID != "s-caller" {
		t.Errorf("T1 ParentLaneID = %q, want s-caller", t1.ParentLaneID)
	}
	if t2.ParentLaneID != "s-t1" {
		t.Errorf("T2 ParentLaneID = %q, want s-t1 (nested onto its T1 caller, not root)", t2.ParentLaneID)
	}
	if t2.Depth != 2 {
		t.Errorf("T2 Depth = %d, want 2", t2.Depth)
	}
}

func TestBuildTrace_LayersUnaffectedByLongRunningSubLaneWorker(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)
	// Root analyzer (layer 0) closes quickly.
	insertTraceSession(t, pool, traceSession{id: "s-a", wfiID: wfiID, agentType: "analyzer",
		status: "completed", result: "pass", startedAt: "2025-01-01T00:00:00Z", endedAt: "2025-01-01T00:01:00Z"})
	// A delegate worker spawned from the (still-open) builder session runs far
	// longer than any root-lane segment and starts before the builder itself.
	insertTraceSession(t, pool, traceSession{id: "s-b", wfiID: wfiID, agentType: "builder",
		status: "running", startedAt: "2025-01-01T00:01:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-worker", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2024-01-01T00:00:00Z", endedAt: "2026-01-01T00:00:00Z"})
	insertDelegation(t, pool, &model.Delegation{
		ID: "wfi-trace.deleg-long", CallerSessionID: "s-b", WorkflowInstanceID: wfiID, ProjectID: "test-proj",
		Tier: "executor", Fanout: 1, Depth: 1,
	}, 0, "s-worker")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if len(trace.Layers) != 2 {
		t.Fatalf("layers = %d, want 2", len(trace.Layers))
	}
	l0 := trace.Layers[0]
	if l0.StartedAt == nil || *l0.StartedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("layer 0 started_at = %v, want 2025-01-01T00:00:00Z (unaffected by the sub-lane worker's wider span)", l0.StartedAt)
	}
	if l0.EndedAt == nil || *l0.EndedAt != "2025-01-01T00:01:00Z" {
		t.Errorf("layer 0 ended_at = %v, want 2025-01-01T00:01:00Z", l0.EndedAt)
	}
	l1 := trace.Layers[1]
	if l1.StartedAt == nil || *l1.StartedAt != "2025-01-01T00:01:00Z" {
		t.Errorf("layer 1 started_at = %v, want the builder's own start (2025-01-01T00:01:00Z), not the sub-lane worker's 2024 start", l1.StartedAt)
	}
	if l1.EndedAt != nil {
		t.Errorf("layer 1 ended_at = %v, want nil (builder still running; sub-lane worker's completion must not close the band)", l1.EndedAt)
	}
}

func TestBuildTrace_ToolMarker_AttachesToSubLaneNotRootMarkers(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-caller", wfiID: wfiID, agentType: "analyzer",
		status: "running", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-worker", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "running", startedAt: "2025-01-01T00:01:00Z"})
	insertDelegation(t, pool, &model.Delegation{
		ID: "wfi-trace.deleg-mark", CallerSessionID: "s-caller", WorkflowInstanceID: wfiID, ProjectID: "test-proj",
		Tier: "executor", Fanout: 1, Depth: 1,
	}, 0, "s-worker")
	insertTraceMessage(t, pool, "s-worker", 0, "[Bash] ls", "tool", "2025-01-01T00:01:01Z")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if len(trace.RootMarkers) != 0 {
		t.Errorf("RootMarkers = %+v, want none (marker must attribute to the sub-lane)", trace.RootMarkers)
	}
	if len(trace.SubLanes) != 1 || len(trace.SubLanes[0].Markers) != 1 {
		t.Fatalf("SubLanes = %+v, want 1 sub-lane with 1 marker", trace.SubLanes)
	}
	if trace.SubLanes[0].Markers[0].Label != "[Bash] ls" {
		t.Errorf("marker label = %q, want [Bash] ls", trace.SubLanes[0].Markers[0].Label)
	}
	// The caller's own root lane must not receive the worker's marker.
	for _, l := range trace.Lanes {
		if l.LaneID == "s-caller" && len(l.Markers) != 0 {
			t.Errorf("caller lane markers = %+v, want none (worker tool marker must not leak onto the caller's lane)", l.Markers)
		}
	}
}
