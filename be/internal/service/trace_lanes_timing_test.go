package service

import (
	"testing"

	"be/internal/types"
)

// TestBuildTrace_TimeBucketsSummedAcrossRelaunchChain verifies a lane with a
// relaunch chain (continued root + running relaunch) sums each segment's
// time_buckets_json into the lane's TimeBuckets, same accumulate-across-
// chain pattern as NudgeCount/StopBlockCount.
func TestBuildTrace_TimeBucketsSummedAcrossRelaunchChain(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)

	t0, t1 := "2025-01-01T00:00:00Z", "2025-01-01T00:01:00Z"
	insertTraceSession(t, pool, traceSession{id: "s-tb1", wfiID: wfiID, agentType: "builder",
		status: "continued", result: "continue", resultReason: "low_context", startedAt: t0, endedAt: t1})
	insertTraceSession(t, pool, traceSession{id: "s-tb2", wfiID: wfiID, agentType: "builder",
		status: "running", ancestor: "s-tb1", startedAt: t1})

	mustExec(t, pool, `UPDATE agent_sessions SET time_buckets_json = ? WHERE id = ?`,
		`{"thinking_sec":2,"tool_arg_sec":1,"text_sec":0.5,"tool_wait_sec":3}`, "s-tb1")
	mustExec(t, pool, `UPDATE agent_sessions SET time_buckets_json = ? WHERE id = ?`,
		`{"thinking_sec":1,"tool_arg_sec":0.5,"text_sec":1.5,"tool_wait_sec":2}`, "s-tb2")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}

	var lane *types.TraceLane
	for i := range trace.Lanes {
		if trace.Lanes[i].LaneID == "s-tb1" {
			lane = &trace.Lanes[i]
		}
	}
	if lane == nil {
		t.Fatalf("lane s-tb1 not found among %d lanes", len(trace.Lanes))
	}
	if lane.TimeBuckets == nil {
		t.Fatal("TimeBuckets = nil, want a summed value")
	}
	if lane.TimeBuckets.ThinkingSec != 3 || lane.TimeBuckets.ToolArgSec != 1.5 ||
		lane.TimeBuckets.TextSec != 2 || lane.TimeBuckets.ToolWaitSec != 5 {
		t.Errorf("TimeBuckets = %+v, want thinking:3 toolArg:1.5 text:2 toolWait:5 (summed across chain)", lane.TimeBuckets)
	}
}

// TestBuildTrace_TimeBucketsNilWhenAllSegmentsNull verifies a lane whose
// segments all have a NULL time_buckets_json column omits TimeBuckets
// entirely (nil pointer -> renders nothing, not zeros).
func TestBuildTrace_TimeBucketsNilWhenAllSegmentsNull(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupTraceTestEnv(t)

	insertTraceSession(t, pool, traceSession{id: "s-notiming", wfiID: wfiID, agentType: "analyzer",
		status: "completed", result: "pass", startedAt: "2025-01-01T00:00:00Z", endedAt: "2025-01-01T00:01:00Z"})

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}

	var lane *types.TraceLane
	for i := range trace.Lanes {
		if trace.Lanes[i].LaneID == "s-notiming" {
			lane = &trace.Lanes[i]
		}
	}
	if lane == nil {
		t.Fatalf("lane s-notiming not found among %d lanes", len(trace.Lanes))
	}
	if lane.TimeBuckets != nil {
		t.Errorf("TimeBuckets = %+v, want nil when the segment's column is NULL", lane.TimeBuckets)
	}
}
