package spawner

import (
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// costEntry is one session's live cost accounting: cumulative counters,
// pricing resolved once at register, and the debounce/broadcast wiring for
// this session's flush target.
type costEntry struct {
	mu      sync.Mutex
	pool    *db.Pool
	clock   clock.Clock
	pricing modelPricing
	known   bool
	snap    CostSnapshot
	// reported is the raw provider cumulative last seen per field (codex
	// app-server's thread-cumulative tokenUsage totals); zero for
	// claude/api sessions, which only ever call addUsage. setUsage advances
	// this to each report's high water and adds only the increase to snap,
	// so the store is monotone regardless of report ordering. seedReported
	// arms it for a resumed thread (reproducing the old baseline-subtract
	// behavior); resetReported clears it for an in-place rotation onto a
	// fresh thread without touching the accumulated snap.
	reported          CostSnapshot
	lastFlush         time.Time
	lastBroadcastCost float64            // high-water mark; guards a reordered debounce goroutine from pushing a stale-lower cost
	broadcast         func(CostSnapshot) // nil-safe; project- or session-scoped push
	// seenUsage dedups per-turn usage keyed by the transcript entry's stable
	// id (uuid/message.id) so an offset-reset re-read never re-bills the same
	// turn twice. Dropped with the entry on FinalizeSessionCost.
	seenUsage map[string]bool
}

// highWaterDelta returns v's increase over *reported, floored at 0 (a
// stale/out-of-order report at or below the high water contributes nothing),
// and advances *reported to max(*reported, v).
func highWaterDelta(reported *int, v int) int {
	d := v - *reported
	if d < 0 {
		return 0
	}
	*reported = v
	return d
}

func (e *costEntry) recomputeLocked() {
	e.snap.PricingKnown = e.known
	if !e.known {
		e.snap.CostUSD = 0
		return
	}
	e.snap.CostUSD = float64(e.snap.InputTokens)/1e6*e.pricing.in +
		float64(e.snap.OutputTokens)/1e6*e.pricing.out +
		float64(e.snap.CacheReadTokens)/1e6*e.pricing.cacheRead +
		float64(e.snap.CacheWriteTokens)/1e6*e.pricing.cacheWrite
}

// addUsageLocked adds a per-turn delta directly to snap (api/claude-CLI feeds
// already report deltas, not cumulative totals).
func (e *costEntry) addUsageLocked(in, out, cacheRead, cacheWrite int) {
	e.snap.InputTokens += in
	e.snap.OutputTokens += out
	e.snap.CacheReadTokens += cacheRead
	e.snap.CacheWriteTokens += cacheWrite
	e.recomputeLocked()
}

// setUsageLocked accumulates the increase of each field over its last
// reported high water into snap (codex app-server reports cumulative totals
// per event, not deltas) — this makes the store monotone by construction and
// composes correctly with concurrent addUsage deltas from another feed (e.g.
// the refinery sidecar), which setUsageLocked's high-water tracking never
// overwrites.
func (e *costEntry) setUsageLocked(in, out, cacheRead, cacheWrite int) {
	e.snap.InputTokens += highWaterDelta(&e.reported.InputTokens, in)
	e.snap.OutputTokens += highWaterDelta(&e.reported.OutputTokens, out)
	e.snap.CacheReadTokens += highWaterDelta(&e.reported.CacheReadTokens, cacheRead)
	e.snap.CacheWriteTokens += highWaterDelta(&e.reported.CacheWriteTokens, cacheWrite)
	e.recomputeLocked()
}

// seedReportedLocked arms the reported high water at a resumed thread's
// pre-crash cumulative usage, so the first post-resume setUsage report bills
// only the segment past that point instead of re-billing it onto the new
// session.
func (e *costEntry) seedReportedLocked(in, out, cacheRead, cacheWrite int) {
	e.reported = CostSnapshot{InputTokens: in, OutputTokens: out, CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite}
}

// resetReportedLocked clears the reported high water back to zero for an
// in-place rotation onto a fresh provider thread, whose cumulative counters
// restart at zero too — snap (the accumulated total this session has already
// billed) is left untouched.
func (e *costEntry) resetReportedLocked() {
	e.reported = CostSnapshot{}
}

// maybeFlushAndBroadcast debounce-gates a DB flush + broadcast to at most
// once per costFlushDebounce for this session; calls between windows
// coalesce into the next. A snapshot whose cost is below the last broadcast
// high water is skipped outright — the store is monotone, so this can only
// happen when two debounce goroutines race and reorder after releasing the
// mutex, and pushing the earlier one would visibly regress the displayed
// cost.
func (e *costEntry) maybeFlushAndBroadcast(sessionID string) {
	e.mu.Lock()
	now := e.clock.Now()
	if now.Sub(e.lastFlush) < costFlushDebounce {
		e.mu.Unlock()
		return
	}
	e.lastFlush = now
	snap := e.snap
	if snap.CostUSD < e.lastBroadcastCost {
		e.mu.Unlock()
		return
	}
	e.lastBroadcastCost = snap.CostUSD
	e.mu.Unlock()

	flushCostSnapshot(e.pool, e.clock, sessionID, snap)
	if e.broadcast != nil {
		e.broadcast(snap)
	}
}
