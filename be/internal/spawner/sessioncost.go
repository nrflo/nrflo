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

// costEntry is one session's live cost accounting: cumulative counters,
// pricing resolved once at register, and the debounce/broadcast wiring for
// this session's flush target.
type costEntry struct {
	mu        sync.Mutex
	pool      *db.Pool
	clock     clock.Clock
	pricing   modelPricing
	known     bool
	snap      CostSnapshot
	lastFlush time.Time
	broadcast func(CostSnapshot) // nil-safe; project- or session-scoped push
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
	e.snap.InputTokens += in
	e.snap.OutputTokens += out
	e.snap.CacheReadTokens += cacheRead
	e.snap.CacheWriteTokens += cacheWrite
	e.recomputeLocked()
	e.mu.Unlock()
	e.maybeFlushAndBroadcast(sessionID)
}

// setUsage overwrites sessionID's cumulative counters (codex app-server
// reports cumulative totals per event, not deltas).
func (s *costStore) setUsage(sessionID string, in, out, cacheRead, cacheWrite int) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	e.snap.InputTokens = in
	e.snap.OutputTokens = out
	e.snap.CacheReadTokens = cacheRead
	e.snap.CacheWriteTokens = cacheWrite
	e.recomputeLocked()
	e.mu.Unlock()
	e.maybeFlushAndBroadcast(sessionID)
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

// maybeFlushAndBroadcast debounce-gates a DB flush + broadcast to at most
// once per costFlushDebounce for this session; calls between windows
// coalesce into the next.
func (e *costEntry) maybeFlushAndBroadcast(sessionID string) {
	e.mu.Lock()
	now := e.clock.Now()
	if now.Sub(e.lastFlush) < costFlushDebounce {
		e.mu.Unlock()
		return
	}
	e.lastFlush = now
	snap := e.snap
	e.mu.Unlock()

	flushCostSnapshot(e.pool, e.clock, sessionID, snap)
	if e.broadcast != nil {
		e.broadcast(snap)
	}
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

// SetSessionCostUsage overwrites sessionID's cumulative usage (codex
// app-server cumulative totals).
func SetSessionCostUsage(sessionID string, in, out, cacheRead, cacheWrite int) {
	globalCostStore.setUsage(sessionID, in, out, cacheRead, cacheWrite)
}

// SessionCost returns sessionID's live cost snapshot, or ok=false when the
// session has no registered cost entry (never registered, or dropped).
func SessionCost(sessionID string) (CostSnapshot, bool) {
	return globalCostStore.snapshot(sessionID)
}

// FinalizeSessionCost force-flushes sessionID's last snapshot then drops its
// entry — call once at session end (mirrors globalLedgerStore.drop).
func FinalizeSessionCost(sessionID string) {
	globalCostStore.finalFlush(sessionID)
	globalCostStore.drop(sessionID)
}
