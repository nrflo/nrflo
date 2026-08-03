package spawner

import (
	"encoding/json"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// costFlushDebounce is the minimum interval between DB flush + broadcast for
// one session's running cost; deltas between windows coalesce into the next
// (mirrors ledgerBroadcastDebounce).
const costFlushDebounce = time.Second

// CostSnapshot is a session's cumulative token/cost accounting at one
// instant. PricingKnown is false when the session's model has no seeded
// per-MTok pricing — CostUSD then stays 0 rather than misreporting free.
type CostSnapshot struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	PricingKnown     bool
}

// costStore is a session-keyed table of cost entries, mirroring ledgerStore's
// shape. Production code goes through the process-global globalCostStore;
// tests construct their own newCostStore(clock.NewTest(...)) for isolation
// and debounce control.
type costStore struct {
	mu       sync.Mutex
	clock    clock.Clock
	sessions map[string]*costEntry
}

func newCostStore(clk clock.Clock) *costStore {
	return &costStore{clock: clk, sessions: make(map[string]*costEntry)}
}

// globalCostStore is the process-wide store production code writes through.
var globalCostStore = newCostStore(clock.Real())

// register creates sessionID's cost entry, resolving modelID's pricing once
// (best-effort: a nil pool, empty modelID, unknown model, or NULL price_in
// leaves pricing unknown — cost then stays 0/unknown rather than erroring).
// broadcast may be nil (no WS push wired, e.g. tests).
func (s *costStore) register(sessionID, modelID string, pool *db.Pool, clk clock.Clock, broadcast func(CostSnapshot)) {
	pr, known := lookupModelPricing(pool, clk, modelID)
	e := &costEntry{pool: pool, clock: clk, pricing: pr, known: known, broadcast: broadcast}
	s.mu.Lock()
	s.sessions[sessionID] = e
	s.mu.Unlock()
}

func (s *costStore) get(sessionID string) *costEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[sessionID]
}

// drop removes sessionID's cost entry. Safe to call on a session with none.
func (s *costStore) drop(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// addUsage adds a per-turn delta to sessionID's cumulative counters (api and
// claude-CLI feeds report per-turn usage). No-op when the session has no
// registered entry (never registered, or already dropped).
func (s *costStore) addUsage(sessionID string, in, out, cacheRead, cacheWrite int) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	e.addUsageLocked(in, out, cacheRead, cacheWrite)
	e.mu.Unlock()
	e.maybeFlushAndBroadcast(sessionID)
}

// addUsageOnce is addUsage guarded by a per-entry seen-key set: a non-empty
// key already seen for this session is a no-op (offset-reset re-read of a
// transcript line already billed), and an empty key always bills — never
// drop usage just because the caller had no stable id.
func (s *costStore) addUsageOnce(sessionID, key string, in, out, cacheRead, cacheWrite int) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	if key != "" {
		e.mu.Lock()
		if e.seenUsage == nil {
			e.seenUsage = make(map[string]bool)
		}
		if e.seenUsage[key] {
			e.mu.Unlock()
			return
		}
		e.seenUsage[key] = true
		e.mu.Unlock()
	}
	s.addUsage(sessionID, in, out, cacheRead, cacheWrite)
}

// setUsage accumulates sessionID's cumulative counters (codex app-server
// reports cumulative totals per event, not deltas) by adding each field's
// increase over its last-reported high water — see costEntry.setUsageLocked.
func (s *costStore) setUsage(sessionID string, in, out, cacheRead, cacheWrite int) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	e.setUsageLocked(in, out, cacheRead, cacheWrite)
	e.mu.Unlock()
	e.maybeFlushAndBroadcast(sessionID)
}

// seedReported arms sessionID's reported high water at a resumed thread's
// pre-crash cumulative usage. No-op when the session has no registered
// entry.
func (s *costStore) seedReported(sessionID string, in, out, cacheRead, cacheWrite int) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	e.seedReportedLocked(in, out, cacheRead, cacheWrite)
	e.mu.Unlock()
}

// resetReported clears sessionID's reported high water back to zero (an
// in-place rotation onto a fresh provider thread), leaving the accumulated
// snap untouched. No-op when the session has no registered entry.
func (s *costStore) resetReported(sessionID string) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	e.resetReportedLocked()
	e.mu.Unlock()
}

// reportedSnapshot returns sessionID's raw reported cumulative high water, or
// ok=false when the session has no registered entry.
func (s *costStore) reportedSnapshot(sessionID string) (CostSnapshot, bool) {
	e := s.get(sessionID)
	if e == nil {
		return CostSnapshot{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reported, true
}

// snapshot returns sessionID's current cost accounting, or ok=false when the
// session has no registered entry.
func (s *costStore) snapshot(sessionID string) (CostSnapshot, bool) {
	e := s.get(sessionID)
	if e == nil {
		return CostSnapshot{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snap, true
}

// finalFlush forces an immediate DB flush bypassing the debounce window, so a
// session's last delta is never lost to a pending debounce at session end.
func (s *costStore) finalFlush(sessionID string) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	snap := e.snap
	pool, clk := e.pool, e.clock
	e.mu.Unlock()
	flushCostSnapshot(pool, clk, sessionID, snap)
}

// flushCostSnapshot persists one cost snapshot via a targeted UPDATE
// (repo.AgentSessionRepo.UpdateCost) — no full-row rewrite, no per-call write
// from the hot path (only from a debounced or final flush).
func flushCostSnapshot(pool *db.Pool, clk clock.Clock, sessionID string, snap CostSnapshot) {
	if pool == nil {
		return
	}
	tokensJSON, _ := json.Marshal(map[string]int{
		"input_tokens":       snap.InputTokens,
		"output_tokens":      snap.OutputTokens,
		"cache_read_tokens":  snap.CacheReadTokens,
		"cache_write_tokens": snap.CacheWriteTokens,
	})
	_ = repo.NewAgentSessionRepo(pool, clk).UpdateCost(sessionID, string(tokensJSON), snap.CostUSD)
}

// RegisterSessionCost registers sessionID's cost entry against the
// process-global store. modelID is the registry slug (a "cli:model"
// composite is accepted too — lookupModelPricing strips the prefix).
func RegisterSessionCost(sessionID, modelID string, pool *db.Pool, clk clock.Clock, broadcast func(CostSnapshot)) {
	globalCostStore.register(sessionID, modelID, pool, clk, broadcast)
}

// AddSessionCostUsage feeds a per-turn usage delta into sessionID's running
// cost (api turns, claude-CLI assistant events).
func AddSessionCostUsage(sessionID string, in, out, cacheRead, cacheWrite int) {
	globalCostStore.addUsage(sessionID, in, out, cacheRead, cacheWrite)
}

// AddSessionCostUsageOnce is AddSessionCostUsage guarded by dedup key (see
// addUsageOnce) — the sole entry point both Claude transcript tailers use.
func AddSessionCostUsageOnce(sessionID, key string, in, out, cacheRead, cacheWrite int) {
	globalCostStore.addUsageOnce(sessionID, key, in, out, cacheRead, cacheWrite)
}

// SetSessionCostUsage accumulates sessionID's cumulative usage (codex
// app-server cumulative totals) — see costStore.setUsage.
func SetSessionCostUsage(sessionID string, in, out, cacheRead, cacheWrite int) {
	globalCostStore.setUsage(sessionID, in, out, cacheRead, cacheWrite)
}

// SeedSessionCostReported arms sessionID's reported high water at a resumed
// session's pre-crash cumulative usage — call once, right after
// RegisterSessionCost, for a resumed native session whose provider reports
// thread-cumulative totals, so the first post-resume SetSessionCostUsage call
// bills only the segment past the hand-off point.
func SeedSessionCostReported(sessionID string, in, out, cacheRead, cacheWrite int) {
	globalCostStore.seedReported(sessionID, in, out, cacheRead, cacheWrite)
}

// ResetSessionCostThread clears sessionID's reported high water back to zero
// — call right after an in-place console rotation swaps in a fresh provider
// thread, whose cumulative counters restart at zero too. The session's
// accumulated cost snapshot is left untouched.
func ResetSessionCostThread(sessionID string) {
	globalCostStore.resetReported(sessionID)
}

// SessionCost returns sessionID's live cost snapshot, or ok=false when the
// session has no registered cost entry (never registered, or dropped).
func SessionCost(sessionID string) (CostSnapshot, bool) {
	return globalCostStore.snapshot(sessionID)
}

// SessionCostReported returns sessionID's raw provider-reported cumulative
// high water (codex thread-cumulative totals), or ok=false when the session
// has no registered cost entry. Used to hand off a resume's baseline without
// re-billing the attributed snapshot (see backend_resume.go).
func SessionCostReported(sessionID string) (CostSnapshot, bool) {
	return globalCostStore.reportedSnapshot(sessionID)
}

// FinalizeSessionCost force-flushes sessionID's last snapshot then drops its
// entry — call once at session end (mirrors globalLedgerStore.drop).
func FinalizeSessionCost(sessionID string) {
	globalCostStore.finalFlush(sessionID)
	globalCostStore.drop(sessionID)
}
