package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

func TestBuildSessionStats_ToolDistributionAndSelfVsSubtreeRollup(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-stats-root", wfiID: wfiID, agentType: "caller", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-stats-worker", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.stats1", CallerSessionID: "s-stats-root", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "executor", Fanout: 1, Depth: 1}, 0, "s-stats-worker")

	mustExec(t, pool, `UPDATE agent_sessions SET cost_estimate = 1.0, tokens_json = '{"input_tokens":100,"output_tokens":0,"cache_read_tokens":0,"cache_write_tokens":0}' WHERE id = 's-stats-root'`)
	mustExec(t, pool, `UPDATE agent_sessions SET cost_estimate = 2.0, tokens_json = '{"input_tokens":50,"output_tokens":0,"cache_read_tokens":0,"cache_write_tokens":0}' WHERE id = 's-stats-worker'`)

	dispatchRepo := repo.NewDispatchRepo(pool, clock.Real())
	rootSID, workerSID := "s-stats-root", "s-stats-worker"
	if err := dispatchRepo.Insert(&model.ToolDispatch{ProjectID: "test-proj", SessionID: &rootSID, ToolName: "read_file", Input: "{}", Status: model.DispatchStatusSuccess}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := dispatchRepo.Insert(&model.ToolDispatch{ProjectID: "test-proj", SessionID: &workerSID, ToolName: "read_file", Input: "{}", Status: model.DispatchStatusError}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-stats-root")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	stats, err := BuildSessionStats(pool, clock.Real(), flow)
	if err != nil {
		t.Fatalf("BuildSessionStats: %v", err)
	}

	if len(stats.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(stats.ToolCalls))
	}
	tc := stats.ToolCalls[0]
	if tc.ToolName != "read_file" || tc.Success != 1 || tc.Error != 1 {
		t.Errorf("ToolCalls[0] = %+v, want read_file success=1 error=1", tc)
	}

	if stats.SelfCostUSD != 1.0 || stats.SelfTokens != 100 {
		t.Errorf("Self = cost=%v tokens=%v, want 1.0/100 (root only)", stats.SelfCostUSD, stats.SelfTokens)
	}
	if stats.SubtreeCostUSD != 3.0 || stats.SubtreeTokens != 150 {
		t.Errorf("Subtree = cost=%v tokens=%v, want 3.0/150 (root+worker)", stats.SubtreeCostUSD, stats.SubtreeTokens)
	}
	if stats.RootSessionID != "s-stats-root" {
		t.Errorf("RootSessionID = %q, want s-stats-root", stats.RootSessionID)
	}
}

// TestBuildSessionStats_CacheRollup verifies the cache-hit percentage
// (cache_read over ALL prompt tokens) and the no-cache cost (every prompt
// token at the model's full price_in) across the subtree.
func TestBuildSessionStats_CacheRollup(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-cache-root", wfiID: wfiID, agentType: "caller", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-cache-worker", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.cache1", CallerSessionID: "s-cache-root", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "executor", Fanout: 1, Depth: 1}, 0, "s-cache-worker")

	// haiku-4-5 is seeded with price_in=1/price_out=5 per MTok. Root:
	// 250k fresh + 750k cache-read = 75% hit, no-cache = 1M * $1 = $1.
	// Worker: 500k fresh + 500k cache-write = 0% hit, no-cache = $1.
	mustExec(t, pool, `UPDATE agent_sessions SET model_id = 'haiku-4-5', tokens_json = '{"input_tokens":250000,"output_tokens":0,"cache_read_tokens":750000,"cache_write_tokens":0}' WHERE id = 's-cache-root'`)
	// CLI sessions store the adapter-prefixed id ('claude:haiku-4-5'); the
	// rollup's pricing join must strip the prefix to find the models row.
	mustExec(t, pool, `UPDATE agent_sessions SET model_id = 'claude:haiku-4-5', tokens_json = '{"input_tokens":500000,"output_tokens":0,"cache_read_tokens":0,"cache_write_tokens":500000}' WHERE id = 's-cache-worker'`)

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-cache-root")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	stats, err := BuildSessionStats(pool, clock.Real(), flow)
	if err != nil {
		t.Fatalf("BuildSessionStats: %v", err)
	}

	if stats.SelfCacheHitPct != 75 {
		t.Errorf("SelfCacheHitPct = %v, want 75", stats.SelfCacheHitPct)
	}
	if stats.SubtreeCacheHitPct != 37.5 {
		t.Errorf("SubtreeCacheHitPct = %v, want 37.5 (750k of 2M prompt tokens)", stats.SubtreeCacheHitPct)
	}
	if stats.SelfCostNoCacheUSD != 1.0 {
		t.Errorf("SelfCostNoCacheUSD = %v, want 1.0", stats.SelfCostNoCacheUSD)
	}
	if stats.SubtreeCostNoCacheUSD != 2.0 {
		t.Errorf("SubtreeCostNoCacheUSD = %v, want 2.0", stats.SubtreeCostNoCacheUSD)
	}
}

func TestBuildSessionStats_SharedChildCountedOnceInSubtree(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-shared-root", wfiID: wfiID, agentType: "caller", status: "completed", startedAt: "2025-01-01T00:00:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-shared-a", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t1_executor", status: "completed", startedAt: "2025-01-01T00:01:00Z"})
	insertSubLaneSession(t, pool, subLaneSession{id: "s-shared-child", wfiID: wfiID, phase: "_delegate", nodeID: "_delegate",
		agentType: "_t2_extractor", status: "completed", startedAt: "2025-01-01T00:02:00Z"})

	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.shr1", CallerSessionID: "s-shared-root", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "executor", Fanout: 1, Depth: 1}, 0, "s-shared-a")
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.shr2", CallerSessionID: "s-shared-root", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "extractor", Fanout: 1, Depth: 1}, 0, "s-shared-child")
	insertDelegation(t, pool, &model.Delegation{ID: "wfi-trace.shr3", CallerSessionID: "s-shared-a", WorkflowInstanceID: wfiID, ProjectID: "test-proj", Tier: "extractor", Fanout: 1, Depth: 2}, 0, "s-shared-child")

	mustExec(t, pool, `UPDATE agent_sessions SET cost_estimate = 5.0 WHERE id = 's-shared-child'`)

	flow, err := BuildSessionFlow(pool, clock.Real(), "s-shared-root")
	if err != nil {
		t.Fatalf("BuildSessionFlow: %v", err)
	}
	stats, err := BuildSessionStats(pool, clock.Real(), flow)
	if err != nil {
		t.Fatalf("BuildSessionStats: %v", err)
	}
	// s-shared-child is reachable via two edges but must be summed exactly
	// once in the subtree rollup.
	if stats.SubtreeCostUSD != 5.0 {
		t.Errorf("SubtreeCostUSD = %v, want 5.0 (shared child counted once, not twice)", stats.SubtreeCostUSD)
	}
}

func TestBuildSessionStats_EmptyFlow_ZeroValues(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)
	flow := &types.SessionFlowResponse{RootSessionID: "no-such", Nodes: []types.SessionFlowNode{{SessionID: "no-such", Depth: 0}}}
	stats, err := BuildSessionStats(pool, clock.Real(), flow)
	if err != nil {
		t.Fatalf("BuildSessionStats: %v", err)
	}
	if len(stats.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %+v, want empty", stats.ToolCalls)
	}
	if stats.SelfCostUSD != 0 || stats.SubtreeCostUSD != 0 {
		t.Errorf("costs = self=%v subtree=%v, want 0/0", stats.SelfCostUSD, stats.SubtreeCostUSD)
	}
}
