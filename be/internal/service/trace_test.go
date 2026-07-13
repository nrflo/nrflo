package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// setupTraceTestEnv builds a project + 2-layer workflow (analyzer L0,
// builder L1) + active instance for trace assembly tests.
func setupTraceTestEnv(t *testing.T) (*db.Pool, *WorkflowService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "trace_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('test-proj', 'Test', '/tmp', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('test-wf', 'test-proj', '', 'ticket', ?, ?)`, now, now)
	for _, ad := range []struct {
		id    string
		layer int
	}{{"analyzer", 0}, {"builder", 1}} {
		mustExec(t, pool, `INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at) VALUES (?, 'test-proj', 'test-wf', '', ?, ?, ?)`,
			ad.id, ad.layer, now, now)
	}
	wfiID := "wfi-trace"
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES (?, 'test-proj', '', 'test-wf', 'ticket', 'active', 0, ?, ?)`, wfiID, now, now)

	return pool, NewWorkflowService(pool, clock.Real()), wfiID
}

func mustExec(t *testing.T, pool *db.Pool, query string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\nquery: %s", err, query)
	}
}

// traceSession describes one agent_sessions fixture row.
type traceSession struct {
	id, wfiID, agentType, status, result, resultReason, ancestor, startedAt, endedAt string
}

func insertTraceSession(t *testing.T, pool *db.Pool, s traceSession) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			status, result, result_reason, ancestor_session_id, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-proj', '', ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), 0, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		s.id, s.wfiID, s.agentType, s.agentType, s.agentType, s.status, s.result, s.resultReason,
		s.ancestor, s.startedAt, s.endedAt, now, now)
}

func TestBuildTrace_LanesLayersChildren(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)

	t0, t1, t2 := "2025-01-01T00:00:00Z", "2025-01-01T00:01:00Z", "2025-01-01T00:02:00Z"
	// Layer 0: one completed analyzer session.
	insertTraceSession(t, pool, traceSession{id: "s-a", wfiID: wfiID, agentType: "analyzer",
		status: "completed", result: "pass", startedAt: t0, endedAt: t1})
	// Layer 1: builder relaunch chain — continued root + running relaunch.
	insertTraceSession(t, pool, traceSession{id: "s-b1", wfiID: wfiID, agentType: "builder",
		status: "continued", result: "continue", resultReason: "low_context", startedAt: t1, endedAt: t2})
	insertTraceSession(t, pool, traceSession{id: "s-b2", wfiID: wfiID, agentType: "builder",
		status: "running", ancestor: "s-b1", startedAt: t2})
	// Transient sessions must be excluded from the trace.
	insertTraceSession(t, pool, traceSession{id: "s-fin", wfiID: wfiID, agentType: "_finalize",
		status: "completed", result: "pass", startedAt: t2, endedAt: t2})
	insertTraceSession(t, pool, traceSession{id: "s-plan", wfiID: wfiID, agentType: "planner",
		status: "completed", result: "pass", startedAt: t0, endedAt: t1})
	// Child sub-workflow launched by the builder session.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, parent_session, parent_instance_id, created_at, updated_at)
		 VALUES ('wfi-child', 'test-proj', '', 'test-wf', 'ticket', 'active', 0, 's-b2', ?, ?, ?)`, wfiID, now, now)

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}

	if trace.Status != "active" || trace.EndedAt != nil {
		t.Errorf("active instance: status=%q ended=%v, want active/nil", trace.Status, trace.EndedAt)
	}
	if len(trace.Lanes) != 2 {
		t.Fatalf("lanes = %d, want 2 (transient excluded)", len(trace.Lanes))
	}
	if trace.Lanes[0].Phase != "analyzer" || trace.Lanes[0].Layer != 0 {
		t.Errorf("lane 0 = %s/L%d, want analyzer/L0", trace.Lanes[0].Phase, trace.Lanes[0].Layer)
	}
	builder := trace.Lanes[1]
	if builder.LaneID != "s-b1" || len(builder.Segments) != 2 {
		t.Fatalf("builder lane_id=%q segments=%d, want s-b1/2", builder.LaneID, len(builder.Segments))
	}
	if builder.Segments[0].SessionID != "s-b1" || builder.Segments[1].SessionID != "s-b2" {
		t.Errorf("segment order = %s,%s, want s-b1,s-b2", builder.Segments[0].SessionID, builder.Segments[1].SessionID)
	}
	if builder.Status != "running" {
		t.Errorf("builder lane status = %q, want running (last segment)", builder.Status)
	}
	if len(builder.Restarts) != 1 || builder.Restarts[0].Reason != "low_context" {
		t.Errorf("builder restarts = %+v, want 1 low_context entry", builder.Restarts)
	}

	if len(trace.Layers) != 2 {
		t.Fatalf("layers = %d, want 2", len(trace.Layers))
	}
	l0, l1 := trace.Layers[0], trace.Layers[1]
	if l0.StartedAt == nil || l0.EndedAt == nil {
		t.Errorf("layer 0 band should be closed, got start=%v end=%v", l0.StartedAt, l0.EndedAt)
	}
	if l1.StartedAt == nil || l1.EndedAt != nil {
		t.Errorf("layer 1 band should be open (running session), got start=%v end=%v", l1.StartedAt, l1.EndedAt)
	}

	if len(trace.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(trace.Children))
	}
	if trace.Children[0].InstanceID != "wfi-child" || trace.Children[0].ParentSessionID != "s-b2" {
		t.Errorf("child = %+v, want wfi-child launched by s-b2", trace.Children[0])
	}
}

func TestBuildTrace_TerminalInstanceHasEndedAt(t *testing.T) {
	t.Parallel()
	pool, svc, _ := setupTraceTestEnv(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES ('wfi-done', 'test-proj', '', 'test-wf', 'ticket', 'completed', 0, ?, ?)`, now, now)

	trace, err := svc.BuildTrace("wfi-done", TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if trace.EndedAt == nil {
		t.Error("completed instance should have ended_at")
	}
	// Zero-session layers still emitted as pending bands.
	if len(trace.Layers) != 2 || trace.Layers[0].StartedAt != nil {
		t.Errorf("expected 2 empty pending bands, got %+v", trace.Layers)
	}
	if len(trace.Lanes) != 0 || len(trace.Children) != 0 {
		t.Errorf("expected no lanes/children, got %d/%d", len(trace.Lanes), len(trace.Children))
	}
}

func TestBuildTrace_UnknownInstance(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupTraceTestEnv(t)
	if _, err := svc.BuildTrace("no-such-wfi", TraceOptions{}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestBuildTrace_DeletedDefDegradesToLayerMinusOne(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-a", wfiID: wfiID, agentType: "analyzer",
		status: "completed", result: "pass", startedAt: "2025-01-01T00:00:00Z", endedAt: "2025-01-01T00:01:00Z"})
	mustExec(t, pool, `DELETE FROM agent_definitions WHERE workflow_id = 'test-wf'`)

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if len(trace.Layers) != 0 {
		t.Errorf("layers = %d, want 0 (no phases in def)", len(trace.Layers))
	}
	if len(trace.Lanes) != 1 || trace.Lanes[0].Layer != -1 {
		t.Errorf("lane layer = %+v, want single lane with layer -1", trace.Lanes)
	}
}
