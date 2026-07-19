package spawner

import (
	"strings"
	"testing"
)

// TestSelectEviction_TierOrdering verifies the priority ordering: superseded
// first, then stale tool_result/file_read (unreferenced >= decayTurns), then
// plain dialog — never picking a fresh (non-stale) tool_result/file_read or
// a superseded/stale entry out of order.
func TestSelectEviction_TierOrdering(t *testing.T) {
	entries := []LedgerEntry{
		{ID: "dialog-1", Kind: LedgerKindDialog, TokensEst: 10, Source: "d1"},
		{ID: "stale-tr", Kind: LedgerKindToolResult, TokensEst: 20, LastRefTurn: 0, Source: "tr1"},
		{ID: "fresh-tr", Kind: LedgerKindToolResult, TokensEst: 30, LastRefTurn: 19, Source: "tr2"}, // referenced recently: not stale
		{ID: "superseded-1", Kind: LedgerKindFileRead, TokensEst: 40, Superseded: true, Source: "sup1"},
		{ID: "stale-fr", Kind: LedgerKindFileRead, TokensEst: 50, LastRefTurn: 0, Source: "fr1"},
	}
	currentTurn, decayTurns := 20, 20

	// Uncapped target (<=0): every eligible entry (all but fresh-tr) is
	// picked, in tier order: superseded, then stale tool_result/file_read (in
	// original slice order), then dialog. buildReferenceDigest renders one
	// line per picked entry in pick order, so source-substring positions in
	// the digest text expose the ordering directly.
	res := selectEviction(entries, currentTurn, decayTurns, 0)
	if res.evictCount != 4 {
		t.Fatalf("evictCount = %d, want 4 (excludes the fresh tool_result)", res.evictCount)
	}
	if strings.Contains(res.digest, "tr2") {
		t.Errorf("digest contains the fresh (non-stale) entry: %q", res.digest)
	}
	posSup := strings.Index(res.digest, "sup1")
	posTR := strings.Index(res.digest, "tr1")
	posFR := strings.Index(res.digest, "fr1")
	posDialog := strings.Index(res.digest, "d1")
	if posSup < 0 || posTR < 0 || posFR < 0 || posDialog < 0 {
		t.Fatalf("digest missing an expected entry: %q", res.digest)
	}
	if posSup >= posTR || posTR >= posFR || posFR >= posDialog {
		t.Errorf("eviction order wrong: superseded=%d stale-tr=%d stale-fr=%d dialog=%d, want ascending", posSup, posTR, posFR, posDialog)
	}
	if res.tokensEvicted != 40+20+50+10 {
		t.Errorf("tokensEvicted = %d, want %d", res.tokensEvicted, 40+20+50+10)
	}
}

// TestSelectEviction_StaleBoundary verifies the exact decay boundary:
// currentTurn-LastRefTurn == decayTurns is stale (eligible); one turn short
// is not.
func TestSelectEviction_StaleBoundary(t *testing.T) {
	entries := []LedgerEntry{
		{ID: "at-boundary", Kind: LedgerKindToolResult, TokensEst: 10, LastRefTurn: 0},
		{ID: "one-short", Kind: LedgerKindToolResult, TokensEst: 10, LastRefTurn: 1},
	}
	res := selectEviction(entries, 20, 20, 0)
	if res.evictCount != 1 {
		t.Fatalf("evictCount = %d, want 1 (only the entry exactly at the decay boundary)", res.evictCount)
	}
}

// TestSelectEviction_TargetTokensStopsEarly verifies a positive targetTokens
// bound stops picking once the running total reaches it, rather than
// draining every eligible entry.
func TestSelectEviction_TargetTokensStopsEarly(t *testing.T) {
	entries := []LedgerEntry{
		{ID: "d1", Kind: LedgerKindDialog, TokensEst: 30},
		{ID: "d2", Kind: LedgerKindDialog, TokensEst: 30},
		{ID: "d3", Kind: LedgerKindDialog, TokensEst: 30},
	}
	res := selectEviction(entries, 0, 20, 50) // target=50: d1(30) short of target, d2 pushes to 60 then stop
	if res.evictCount != 2 {
		t.Fatalf("evictCount = %d, want 2 (stops once target is reached)", res.evictCount)
	}
	if res.tokensEvicted != 60 {
		t.Errorf("tokensEvicted = %d, want 60", res.tokensEvicted)
	}
}

// TestSelectEviction_NoEligibleEntries verifies a fresh, non-superseded,
// non-dialog set evicts nothing.
func TestSelectEviction_NoEligibleEntries(t *testing.T) {
	entries := []LedgerEntry{
		{ID: "fresh", Kind: LedgerKindToolResult, TokensEst: 10, LastRefTurn: 19},
	}
	res := selectEviction(entries, 20, 20, 0)
	if res.evictCount != 0 {
		t.Errorf("evictCount = %d, want 0", res.evictCount)
	}
	if res.digest != "" {
		t.Errorf("digest = %q, want empty for zero evictions", res.digest)
	}
}

// TestBuildReferenceDigest_ContainsSourceAndTruncatedSHA verifies each line
// names the entry's source (or kind, when source is empty) and a truncated
// (<=12 char) sha when present.
func TestBuildReferenceDigest_ContainsSourceAndTruncatedSHA(t *testing.T) {
	entries := []LedgerEntry{
		{Kind: LedgerKindFileRead, Source: "src/foo.go", SHA: "0123456789abcdefextra"},
		{Kind: LedgerKindDialog, Source: "", SHA: ""},
	}
	digest := buildReferenceDigest(entries)

	if !strings.Contains(digest, "src/foo.go") {
		t.Errorf("digest missing source path: %q", digest)
	}
	if !strings.Contains(digest, "sha:0123456789ab") {
		t.Errorf("digest missing 12-char truncated sha: %q", digest)
	}
	if strings.Contains(digest, "0123456789abcdefextra") {
		t.Errorf("digest leaked the full untruncated sha: %q", digest)
	}
	if !strings.Contains(digest, string(LedgerKindDialog)) {
		t.Errorf("digest missing kind fallback for a source-less entry: %q", digest)
	}
}

// TestBuildReferenceDigest_Empty verifies no entries yields an empty digest,
// not a bare header.
func TestBuildReferenceDigest_Empty(t *testing.T) {
	if got := buildReferenceDigest(nil); got != "" {
		t.Errorf("buildReferenceDigest(nil) = %q, want empty", got)
	}
}

// TestResolveKeepCounts_ShortConversation_KeepsEverything verifies a
// conversation no longer than pinnedPrefix+recentWindow is never touched
// (keepPrefix=0, keepSuffix=totalMsgs — the applier's start>=end no-op path).
func TestResolveKeepCounts_ShortConversation_KeepsEverything(t *testing.T) {
	kp, ks := resolveKeepCounts(pinnedPrefixMessages+recentWindowMessages, 5, 10)
	if kp != 0 || ks != pinnedPrefixMessages+recentWindowMessages {
		t.Errorf("resolveKeepCounts(short) = (%d,%d), want (0,%d)", kp, ks, pinnedPrefixMessages+recentWindowMessages)
	}
}

// TestResolveKeepCounts_ProportionalMiddleEviction verifies the middle
// eviction scales with the eligible-entry fraction and always preserves the
// pinned prefix.
func TestResolveKeepCounts_ProportionalMiddleEviction(t *testing.T) {
	// 20 total msgs: prefix 2, suffix 6, middle 12. Evicting 5/10 entries
	// (50%) should evict ~6 of the 12 middle messages.
	totalMsgs := 20
	kp, ks := resolveKeepCounts(totalMsgs, 5, 10)
	if kp != pinnedPrefixMessages {
		t.Errorf("keepPrefix = %d, want %d (always preserved)", kp, pinnedPrefixMessages)
	}
	evicted := totalMsgs - kp - ks
	if evicted != 6 {
		t.Errorf("evicted middle messages = %d, want 6 (50%% of the 12-message middle)", evicted)
	}
}

// TestResolveKeepCounts_ZeroEvictedOrTotalEntries_NoMiddleEviction verifies
// the degenerate divisor guards: no entries evicted, or an empty ledger,
// leaves the middle untouched.
func TestResolveKeepCounts_ZeroEvictedOrTotalEntries_NoMiddleEviction(t *testing.T) {
	totalMsgs := 20
	for _, tc := range []struct {
		name           string
		evicted, total int
	}{
		{"zero evicted", 0, 10},
		{"zero total entries", 5, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kp, ks := resolveKeepCounts(totalMsgs, tc.evicted, tc.total)
			if kp != pinnedPrefixMessages || ks != totalMsgs-pinnedPrefixMessages {
				t.Errorf("resolveKeepCounts(%s) = (%d,%d), want (%d,%d)", tc.name, kp, ks, pinnedPrefixMessages, totalMsgs-pinnedPrefixMessages)
			}
		})
	}
}

// TestResolveKeepCounts_FullEvictionFractionCapsAtMiddle verifies a 100%
// eviction fraction never evicts past the middle boundary (the recent
// window and pinned prefix are never eaten into).
func TestResolveKeepCounts_FullEvictionFractionCapsAtMiddle(t *testing.T) {
	totalMsgs := 20
	kp, ks := resolveKeepCounts(totalMsgs, 10, 10) // 100% of entries evicted
	middle := totalMsgs - pinnedPrefixMessages - recentWindowMessages
	if kp != pinnedPrefixMessages {
		t.Errorf("keepPrefix = %d, want %d", kp, pinnedPrefixMessages)
	}
	if ks != recentWindowMessages {
		t.Errorf("keepSuffix = %d, want %d (evicted capped at the %d-message middle)", ks, recentWindowMessages, middle)
	}
}
