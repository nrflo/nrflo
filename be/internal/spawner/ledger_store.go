package spawner

import (
	"sync"

	"be/internal/clock"
	"be/internal/ws"
)

// ledgerStore is a session-keyed table of ledgers. Production code goes
// through the process-global globalLedgerStore below; tests construct their
// own newLedgerStore(clock.NewTest(...)) for isolation and debounce control.
type ledgerStore struct {
	mu       sync.Mutex
	clock    clock.Clock
	sessions map[string]*ledger
}

func newLedgerStore(clk clock.Clock) *ledgerStore {
	return &ledgerStore{clock: clk, sessions: make(map[string]*ledger)}
}

// get returns sessionID's ledger, creating an empty one on first access.
func (s *ledgerStore) get(sessionID string) *ledger {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.sessions[sessionID]
	if !ok {
		l = newLedger()
		s.sessions[sessionID] = l
	}
	return l
}

// drop removes sessionID's ledger. Safe to call on a session with no ledger.
func (s *ledgerStore) drop(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

func (s *ledgerStore) snapshot(sessionID string) (ContextLedgerSnapshot, bool) {
	s.mu.Lock()
	l, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return ContextLedgerSnapshot{}, false
	}
	return l.snapshot(sessionID), true
}

func (s *ledgerStore) epochSummary(sessionID string) (LedgerEpochSummary, bool) {
	s.mu.Lock()
	l, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return LedgerEpochSummary{}, false
	}
	return l.epochSummary(sessionID), true
}

func (s *ledgerStore) shouldBroadcast(sessionID string) bool {
	s.mu.Lock()
	l, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return false
	}
	return l.shouldBroadcast(s.clock.Now())
}

// globalLedgerStore is the process-wide store production code writes
// through; tests construct their own newLedgerStore(clock.NewTest(...))
// instead of reaching through this singleton.
var globalLedgerStore = newLedgerStore(clock.Real())

// LedgerSnapshot returns a read-only snapshot of sessionID's context ledger,
// or ok=false when the session has no tracked ledger (finished, dropped, or
// never written to). Used by the api package's debug endpoint.
func LedgerSnapshot(sessionID string) (ContextLedgerSnapshot, bool) {
	return globalLedgerStore.snapshot(sessionID)
}

// ledgerBroadcastData renders an epoch summary as a WS event payload.
func ledgerBroadcastData(sum LedgerEpochSummary) map[string]interface{} {
	totals := make(map[string]int, len(sum.TotalsByKind))
	for k, v := range sum.TotalsByKind {
		totals[string(k)] = v
	}
	return map[string]interface{}{
		"session_id":     sum.SessionID,
		"total_tokens":   sum.TotalTokens,
		"entry_count":    sum.EntryCount,
		"totals_by_kind": totals,
	}
}

// broadcastLedgerEpoch emits a debounced EventAgentContextLedger snapshot for
// proc's session, shared by all three per-engine ledger writers.
func (s *Spawner) broadcastLedgerEpoch(proc *processInfo) {
	if !globalLedgerStore.shouldBroadcast(proc.sessionID) {
		return
	}
	summary, ok := globalLedgerStore.epochSummary(proc.sessionID)
	if !ok {
		return
	}
	s.broadcast(ws.EventAgentContextLedger, proc.projectID, proc.ticketID, proc.workflowName, ledgerBroadcastData(summary))
}
