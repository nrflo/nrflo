package service

import (
	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// BuildSessionStats assembles the tool-call distribution and cost/token
// rollup over flow's node set (the session-flow read model's stats
// companion, GET /api/v1/sessions/{sid}/stats). Self* covers the root
// session alone; Subtree* sums every node in flow exactly once — a session
// shared by two callers (e.g. a fanned-in delegate result) is never
// double-counted since flow.Nodes is already deduped by BuildSessionFlow.
func BuildSessionStats(pool *db.Pool, clk clock.Clock, flow *types.SessionFlowResponse) (*types.SessionStatsResponse, error) {
	sessionIDs := make([]string, 0, len(flow.Nodes))
	for _, n := range flow.Nodes {
		sessionIDs = append(sessionIDs, n.SessionID)
	}

	dispatchRepo := repo.NewDispatchRepo(pool, clk)
	stats, err := dispatchRepo.ToolDistribution(sessionIDs)
	if err != nil {
		return nil, err
	}

	resp := &types.SessionStatsResponse{
		RootSessionID: flow.RootSessionID,
		ToolCalls:     mergeToolCallStats(stats),
	}

	sessionRepo := repo.NewAgentSessionRepo(pool, clk)
	if selfCost, selfTokens, err := sessionRepo.CostTokenRollup([]string{flow.RootSessionID}); err == nil {
		resp.SelfCostUSD = selfCost
		resp.SelfTokens = selfTokens
	}
	if subCost, subTokens, err := sessionRepo.CostTokenRollup(sessionIDs); err == nil {
		resp.SubtreeCostUSD = subCost
		resp.SubtreeTokens = subTokens
	}
	if in, cr, cw, _, noCache, err := sessionRepo.CacheRollup([]string{flow.RootSessionID}); err == nil {
		resp.SelfCacheHitPct = cacheHitPct(in, cr, cw)
		resp.SelfCostNoCacheUSD = noCache
	}
	if in, cr, cw, _, noCache, err := sessionRepo.CacheRollup(sessionIDs); err == nil {
		resp.SubtreeCacheHitPct = cacheHitPct(in, cr, cw)
		resp.SubtreeCostNoCacheUSD = noCache
	}

	return resp, nil
}

// cacheHitPct is cache-read tokens over all prompt tokens, 0 when no prompt
// tokens were recorded.
func cacheHitPct(input, cacheRead, cacheWrite int64) float64 {
	total := input + cacheRead + cacheWrite
	if total == 0 {
		return 0
	}
	return 100 * float64(cacheRead) / float64(total)
}

// mergeToolCallStats folds repo.ToolCallStat's (tool_name, status, count)
// buckets into one success/error pair per tool.
func mergeToolCallStats(stats []repo.ToolCallStat) []types.ToolCallDistributionEntry {
	byTool := map[string]*types.ToolCallDistributionEntry{}
	order := []string{}
	for _, st := range stats {
		entry, ok := byTool[st.ToolName]
		if !ok {
			entry = &types.ToolCallDistributionEntry{ToolName: st.ToolName}
			byTool[st.ToolName] = entry
			order = append(order, st.ToolName)
		}
		if st.Status == "error" {
			entry.Error += st.Count
		} else {
			entry.Success += st.Count
		}
	}
	out := make([]types.ToolCallDistributionEntry, 0, len(order))
	for _, name := range order {
		out = append(out, *byTool[name])
	}
	return out
}
