package spawner

import (
	"encoding/json"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// timingFlushDebounce is the minimum interval between DB flushes of one
// session's running timing buckets; deltas between windows coalesce into
// the next (mirrors costFlushDebounce).
const timingFlushDebounce = time.Second

// TimingBucket identifies which cumulative bucket a timing delta belongs
// to.
type TimingBucket int

const (
	TimingBucketThinking TimingBucket = iota
	TimingBucketToolArg
	TimingBucketText
	TimingBucketToolWait
)

// TimingSnapshot is a session's cumulative bucket-seconds at one instant.
type TimingSnapshot struct {
	ThinkingSec float64
	ToolArgSec  float64
	TextSec     float64
	ToolWaitSec float64
}

// timingEntry is one session's live timing accounting: cumulative bucket
// seconds, the last-seen event timestamp used to compute the next delta,
// and dedup state for offset-reset re-reads.
type timingEntry struct {
	mu        sync.Mutex
	pool      *db.Pool
	clock     clock.Clock
	snap      TimingSnapshot
	lastFlush time.Time
	lastTS    time.Time
	haveLast  bool
	// seen dedups per-entry events keyed by the transcript entry's stable id
	// (uuid) so an offset-reset re-read never double-counts the same line.
	// Dropped with the entry on FinalizeSessionTiming.
	seen map[string]bool
}

// timingStore is a session-keyed table of timing entries, mirroring
// costStore's shape. Production code goes through the process-global
// globalTimingStore; tests construct their own newTimingStore(clock.NewTest(...))
// for isolation and debounce control.
type timingStore struct {
	mu       sync.Mutex
	clock    clock.Clock
	sessions map[string]*timingEntry
}

func newTimingStore(clk clock.Clock) *timingStore {
	return &timingStore{clock: clk, sessions: make(map[string]*timingEntry)}
}

// globalTimingStore is the process-wide store production code writes
// through.
var globalTimingStore = newTimingStore(clock.Real())

func (s *timingStore) register(sessionID string, pool *db.Pool, clk clock.Clock) {
	e := &timingEntry{pool: pool, clock: clk}
	s.mu.Lock()
	s.sessions[sessionID] = e
	s.mu.Unlock()
}

func (s *timingStore) get(sessionID string) *timingEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[sessionID]
}

// drop removes sessionID's timing entry. Safe to call on a session with
// none.
func (s *timingStore) drop(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// recordEvent attributes the delta between entryTS and this session's
// last-seen event timestamp to bucket, then advances the anchor to entryTS.
// Dedup-guarded by key (a non-empty key already seen for this session is a
// no-op — an offset-reset re-read of a transcript line already timed; an
// empty key always records). The first event for a session only seeds the
// anchor (no prior timestamp to measure a delta from); an entryTS that does
// not advance past the anchor (clock skew / out-of-order delivery) still
// advances the anchor but records no delta.
func (s *timingStore) recordEvent(sessionID, key string, entryTS time.Time, bucket TimingBucket) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	if key != "" {
		if e.seen == nil {
			e.seen = make(map[string]bool)
		}
		if e.seen[key] {
			e.mu.Unlock()
			return
		}
		e.seen[key] = true
	}
	if !e.haveLast {
		e.lastTS = entryTS
		e.haveLast = true
		e.mu.Unlock()
		return
	}
	delta := entryTS.Sub(e.lastTS).Seconds()
	if entryTS.After(e.lastTS) {
		e.lastTS = entryTS
	}
	if delta > 0 {
		switch bucket {
		case TimingBucketThinking:
			e.snap.ThinkingSec += delta
		case TimingBucketToolArg:
			e.snap.ToolArgSec += delta
		case TimingBucketText:
			e.snap.TextSec += delta
		case TimingBucketToolWait:
			e.snap.ToolWaitSec += delta
		}
	}
	snap := e.snap
	e.mu.Unlock()
	e.maybeFlush(sessionID, snap)
}

// maybeFlush debounce-gates a DB flush to at most once per
// timingFlushDebounce for this session; calls between windows coalesce
// into the next.
func (e *timingEntry) maybeFlush(sessionID string, snap TimingSnapshot) {
	e.mu.Lock()
	now := e.clock.Now()
	if now.Sub(e.lastFlush) < timingFlushDebounce {
		e.mu.Unlock()
		return
	}
	e.lastFlush = now
	pool, clk := e.pool, e.clock
	e.mu.Unlock()
	flushTimingSnapshot(pool, clk, sessionID, snap)
}

// finalFlush forces an immediate DB flush bypassing the debounce window, so
// a session's last delta is never lost to a pending debounce at session
// end.
func (s *timingStore) finalFlush(sessionID string) {
	e := s.get(sessionID)
	if e == nil {
		return
	}
	e.mu.Lock()
	snap := e.snap
	pool, clk := e.pool, e.clock
	e.mu.Unlock()
	flushTimingSnapshot(pool, clk, sessionID, snap)
}

// snapshot returns sessionID's current timing accounting, or ok=false when
// the session has no registered entry.
func (s *timingStore) snapshot(sessionID string) (TimingSnapshot, bool) {
	e := s.get(sessionID)
	if e == nil {
		return TimingSnapshot{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snap, true
}

// flushTimingSnapshot persists one timing snapshot via a targeted UPDATE
// (repo.AgentSessionRepo.UpdateTimeBuckets) — no full-row rewrite, no
// per-call write from the hot path (only from a debounced or final flush).
func flushTimingSnapshot(pool *db.Pool, clk clock.Clock, sessionID string, snap TimingSnapshot) {
	if pool == nil {
		return
	}
	bucketsJSON, _ := json.Marshal(map[string]float64{
		"thinking_sec":  snap.ThinkingSec,
		"tool_arg_sec":  snap.ToolArgSec,
		"text_sec":      snap.TextSec,
		"tool_wait_sec": snap.ToolWaitSec,
	})
	_ = repo.NewAgentSessionRepo(pool, clk).UpdateTimeBuckets(sessionID, string(bucketsJSON))
}

// RegisterSessionTiming registers sessionID's timing entry against the
// process-global store.
func RegisterSessionTiming(sessionID string, pool *db.Pool, clk clock.Clock) {
	globalTimingStore.register(sessionID, pool, clk)
}

// RecordSessionTimingEvent feeds a dedup-guarded timing event into
// sessionID's running buckets. key is the transcript entry's stable id
// (uuid) — empty always records, non-empty seen-before is a no-op. No-op
// when the session has no registered entry (never registered, or already
// finalized).
func RecordSessionTimingEvent(sessionID, key string, entryTS time.Time, bucket TimingBucket) {
	globalTimingStore.recordEvent(sessionID, key, entryTS, bucket)
}

// FinalizeSessionTiming force-flushes sessionID's last snapshot then drops
// its entry — call once at session end (mirrors FinalizeSessionCost).
func FinalizeSessionTiming(sessionID string) {
	globalTimingStore.finalFlush(sessionID)
	globalTimingStore.drop(sessionID)
}
