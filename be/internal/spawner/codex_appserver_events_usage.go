package spawner

import "encoding/json"

// codexUsageBreakdown mirrors codex 0.145.0's TokenUsageBreakdown schema
// (`codex app-server generate-json-schema`): totalTokens/inputTokens are
// required, cachedInputTokens/cacheWriteInputTokens/reasoningOutputTokens
// default to 0 on older payloads that omit them.
type codexUsageBreakdown struct {
	TotalTokens           int `json:"totalTokens"`
	InputTokens           int `json:"inputTokens"`
	CachedInputTokens     int `json:"cachedInputTokens"`
	CacheWriteInputTokens int `json:"cacheWriteInputTokens"`
	OutputTokens          int `json:"outputTokens"`
	ReasoningOutputTokens int `json:"reasoningOutputTokens"`
}

// codexTokenUsage mirrors codex 0.145.0's ThreadTokenUsage schema. `last` is
// the most recent upstream Responses-API call's exact usage — one event per
// upstream response, not per turn. `total` is cumulative across the thread
// (each turn re-sends the growing history), so it overcounts and must NOT
// drive context_left, but IS the right basis for cumulative cost billing
// (SetSessionCostUsage accumulates each field's increase over its last
// reported high water — see sessioncost.go).
//
// A checkpoint with `last.inputTokens==0` is NOT a usage sample: it is the
// post-compaction footprint report that immediately follows a contextCompaction
// item (last.totalTokens is the new footprint; every other last.* field and
// the entire total.* block are zero/unchanged). Validated live against
// codex-cli 0.145.0: every real per-response checkpoint carries
// last.inputTokens>0.
type codexTokenUsage struct {
	Last               codexUsageBreakdown `json:"last"`
	Total              codexUsageBreakdown `json:"total"`
	ModelContextWindow int                 `json:"modelContextWindow"`
}

// freshInputTokens returns the portion of b.InputTokens billed at the full
// input rate — inputTokens is the superset of both cachedInputTokens and
// cacheWriteInputTokens, so billing them again would double-charge.
func freshInputTokens(b codexUsageBreakdown) int {
	fresh := b.InputTokens - b.CachedInputTokens - b.CacheWriteInputTokens
	if fresh < 0 {
		return 0
	}
	return fresh
}

// engineUsage maps one codex usage breakdown onto the provider-agnostic
// EngineUsage the ledger reconciles against.
func engineUsage(b codexUsageBreakdown, ctxWindow int) *EngineUsage {
	return &EngineUsage{
		InputTokens:           b.InputTokens,
		CachedInputTokens:     b.CachedInputTokens,
		CacheWriteTokens:      b.CacheWriteInputTokens,
		OutputTokens:          b.OutputTokens,
		ReasoningOutputTokens: b.ReasoningOutputTokens,
		TotalTokens:           b.TotalTokens,
		ContextWindow:         ctxWindow,
	}
}

func dispatchTokenUsage(sessionID string, params json.RawMessage, sink Sink, maxCtx int, emit EventEmitter) {
	var p struct {
		TokenUsage codexTokenUsage `json:"tokenUsage"`
	}
	if json.Unmarshal(params, &p) != nil {
		sink.BumpLastMessage(sessionID)
		return
	}
	// codex reports cumulative totals per event, not per-turn deltas; bill the
	// fresh (non-cached, non-cache-write) portion at the full input rate.
	SetSessionCostUsage(sessionID, freshInputTokens(p.TokenUsage.Total), p.TokenUsage.Total.OutputTokens, p.TokenUsage.Total.CachedInputTokens, p.TokenUsage.Total.CacheWriteInputTokens)

	if p.TokenUsage.Last.InputTokens <= 0 {
		// Compaction checkpoint, not a usage sample — the cost accumulation
		// above is a harmless no-op since total is unchanged on this event.
		sink.BumpLastMessage(sessionID)
		return
	}

	ctxWindow := p.TokenUsage.ModelContextWindow
	if ctxWindow <= 0 {
		ctxWindow = maxCtx
	}
	if ctxWindow > 0 {
		pct := ComputeContextLeftPct(p.TokenUsage.Last.InputTokens, ctxWindow)
		sink.UpdateContextLeft(sessionID, pct)
		emitTokenUsageEvent(sessionID, pct, engineUsage(p.TokenUsage.Last, ctxWindow), emit)
	}
	sink.BumpLastMessage(sessionID)
}
