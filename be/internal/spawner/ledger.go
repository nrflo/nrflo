package spawner

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"be/internal/id"
)

// ledgerBroadcastDebounce is the minimum interval between epoch-summary
// broadcasts for one session; appends between windows coalesce into the next.
const ledgerBroadcastDebounce = time.Second

var ledgerIDGen = id.New("ctxledger")

// ledger is one session's ordered context-block store. All methods lock mu
// internally, so callers never need their own synchronization.
type ledger struct {
	mu       sync.Mutex
	entries  []*LedgerEntry
	index    map[string]*LedgerEntry // dedup key -> latest non-superseded entry
	toolMeta map[string]toolCallMeta // correlation key -> invoking tool's name/path

	turn             int
	transcriptOffset int64 // cli engine: byte offset already ingested from the transcript
	lastBroadcast    time.Time
}

func newLedger() *ledger {
	return &ledger{
		index:    make(map[string]*LedgerEntry),
		toolMeta: make(map[string]toolCallMeta),
	}
}

// dedupKeyFor picks the identity a new entry supersedes a prior one on:
// file_read entries dedup by path (a fresh read always supersedes the stale
// copy, content aside); everything else dedups by content sha when present.
func dedupKeyFor(kind LedgerKind, source, sha string) string {
	if kind == LedgerKindFileRead && source != "" {
		return "src:" + source
	}
	if sha != "" {
		return "sha:" + sha
	}
	return ""
}

// append records one new entry at the ledger's current turn, marking any
// prior dedup-matching entry (same sha, or same path for file_read) as
// superseded.
func (l *ledger) append(kind LedgerKind, tokensEst int, source, sha string, approx bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := dedupKeyFor(kind, source, sha)
	if key != "" {
		if prior, ok := l.index[key]; ok {
			prior.Superseded = true
		}
	}

	entryID, _ := ledgerIDGen.Generate()
	e := &LedgerEntry{
		ID:          entryID,
		Kind:        kind,
		TokensEst:   tokensEst,
		BornTurn:    l.turn,
		LastRefTurn: l.turn,
		Source:      source,
		SHA:         sha,
		Approx:      approx,
	}
	l.entries = append(l.entries, e)
	if key != "" {
		l.index[key] = e
	}
}

// markRef bumps LastRefTurn on existing, non-superseded entries whose Source
// matches needle via a cheap substring test (either direction) — the ledger's
// last_ref rule for later tool calls re-touching an earlier path/name. Call
// before appending the new entry so it cannot match itself.
func (l *ledger) markRef(needle string) {
	if needle == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.Superseded || e.Source == "" {
			continue
		}
		if strings.Contains(needle, e.Source) || strings.Contains(e.Source, needle) {
			e.LastRefTurn = l.turn
		}
	}
}

// nextTurn advances the ledger's turn counter — called once per observed
// assistant message across all three engines.
func (l *ledger) nextTurn() {
	l.mu.Lock()
	l.turn++
	l.mu.Unlock()
}

// turnNow returns the ledger's current turn counter — the context watcher
// policy uses it to compute how long ago an entry was last referenced
// (turnNow - LastRefTurn), the unreferenced-since basis for decay eviction.
func (l *ledger) turnNow() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.turn
}

func (l *ledger) recordToolMeta(key string, meta toolCallMeta) {
	if key == "" {
		return
	}
	l.mu.Lock()
	l.toolMeta[key] = meta
	l.mu.Unlock()
}

func (l *ledger) lookupToolMeta(key string) toolCallMeta {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.toolMeta[key]
}

func (l *ledger) transcriptOffsetVal() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.transcriptOffset
}

func (l *ledger) setTranscriptOffset(v int64) {
	l.mu.Lock()
	l.transcriptOffset = v
	l.mu.Unlock()
}

// reconcileUsage scales every non-superseded entry's TokensEst so their sum
// matches actual — the provider-reported input-token total for the request
// that just returned. In api mode this is called before that turn's new
// blocks are appended, so it reconciles exactly what usage measured; codex's
// event ordering is inverted (item/completed, which appends new blocks,
// precedes thread/tokenUsage/updated for the same response), so its call
// reconciles against a total that already includes those blocks — a known
// one-response skew, not a correctness bug.
func (l *ledger) reconcileUsage(actual int) {
	if actual <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var est int
	for _, e := range l.entries {
		if !e.Superseded {
			est += e.TokensEst
		}
	}
	if est <= 0 {
		return
	}
	ratio := float64(actual) / float64(est)
	for _, e := range l.entries {
		if !e.Superseded {
			e.TokensEst = int(float64(e.TokensEst)*ratio + 0.5)
		}
	}
}

func (l *ledger) snapshot(sessionID string) ContextLedgerSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := make([]LedgerEntry, len(l.entries))
	totals := make(map[LedgerKind]int)
	for i, e := range l.entries {
		entries[i] = *e
		if !e.Superseded {
			totals[e.Kind] += e.TokensEst
		}
	}
	return ContextLedgerSnapshot{SessionID: sessionID, Entries: entries, TotalsByKind: totals}
}

func (l *ledger) epochSummary(sessionID string) LedgerEpochSummary {
	l.mu.Lock()
	defer l.mu.Unlock()
	totals := make(map[LedgerKind]int)
	total := 0
	for _, e := range l.entries {
		if e.Superseded {
			continue
		}
		totals[e.Kind] += e.TokensEst
		total += e.TokensEst
	}
	return LedgerEpochSummary{SessionID: sessionID, TotalTokens: total, EntryCount: len(l.entries), TotalsByKind: totals}
}

// shouldBroadcast reports whether now clears the debounce window since the
// last broadcast, stamping it as consumed when it does.
func (l *ledger) shouldBroadcast(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastBroadcast) < ledgerBroadcastDebounce {
		return false
	}
	l.lastBroadcast = now
	return true
}

// ledgerSHA returns a short content-identity hash used for dedup keys; empty
// input yields "" (no dedup key).
func ledgerSHA(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
