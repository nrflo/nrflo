package spawner

import (
	"fmt"
	"strings"
)

// pinnedPrefixMessages is the number of leading messages a selective GC never
// touches — the initial task framing that anchors the prompt-cache prefix.
const pinnedPrefixMessages = 2

// recentWindowMessages is the minimum number of trailing messages a selective
// GC always keeps verbatim, so the message a caller is about to build on (the
// loop's next request, or SendTurn's next appended user turn) is never the
// synthetic digest itself.
const recentWindowMessages = 6

// evictionResult is one policy pass's eviction selection: how many tokens
// (estimated) and ledger entries it picked, plus the recoverable-reference
// digest text for the entries it evicted.
type evictionResult struct {
	tokensEvicted int
	evictCount    int
	digest        string
}

// selectEviction orders entries by eviction priority — superseded (dead
// weight a fresher read/dedup already replaced) first, then stale
// tool_result/file_read entries unreferenced for >= decayTurns, then plain
// dialog once nothing cheaper remains — and picks entries off that ordering
// until targetTokens is reached (targetTokens<=0 means take everything
// eligible, the idle-gap "evict everything deferred" case).
func selectEviction(entries []LedgerEntry, currentTurn, decayTurns, targetTokens int) evictionResult {
	tiers := make([][]LedgerEntry, 3)
	for _, e := range entries {
		switch {
		case e.Superseded:
			tiers[0] = append(tiers[0], e)
		case (e.Kind == LedgerKindToolResult || e.Kind == LedgerKindFileRead) && currentTurn-e.LastRefTurn >= decayTurns:
			tiers[1] = append(tiers[1], e)
		case e.Kind == LedgerKindDialog:
			tiers[2] = append(tiers[2], e)
		}
	}

	var picked []LedgerEntry
	var evicted int
outer:
	for _, tier := range tiers {
		for _, e := range tier {
			if targetTokens > 0 && evicted >= targetTokens {
				break outer
			}
			picked = append(picked, e)
			evicted += e.TokensEst
		}
	}

	return evictionResult{tokensEvicted: evicted, evictCount: len(picked), digest: buildReferenceDigest(picked)}
}

// buildReferenceDigest renders the evicted entries as a recoverable-reference
// block: each line names the entry's source (path/tool name) and content
// hash so the model can re-request it if the summary omitted something it
// turns out to need.
func buildReferenceDigest(entries []LedgerEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Evicted context — recoverable references:]\n")
	for _, e := range entries {
		ref := e.Source
		if ref == "" {
			ref = string(e.Kind)
		}
		if e.SHA != "" {
			sha := e.SHA
			if len(sha) > 12 {
				sha = sha[:12]
			}
			fmt.Fprintf(&b, "- %s (%s, sha:%s)\n", ref, e.Kind, sha)
		} else {
			fmt.Fprintf(&b, "- %s (%s)\n", ref, e.Kind)
		}
	}
	return b.String()
}

// resolveKeepCounts translates a ledger-level eviction decision into message
// counts for a CompactionPlan: the pinned prefix and recent window are always
// kept verbatim, and the space between them is evicted in proportion to how
// much of the ledger's eligible entries the policy selected (ledger entries
// track content blocks, not whole messages, so this is a proportional
// estimate, consistent with the ledger's own bytes/4 token heuristic).
func resolveKeepCounts(totalMsgs, evictedEntries, totalEntries int) (keepPrefix, keepSuffix int) {
	if totalMsgs <= pinnedPrefixMessages+recentWindowMessages {
		return 0, totalMsgs
	}
	keepPrefix = pinnedPrefixMessages
	middle := totalMsgs - pinnedPrefixMessages - recentWindowMessages
	if totalEntries <= 0 || evictedEntries <= 0 {
		return keepPrefix, totalMsgs - keepPrefix
	}
	frac := float64(evictedEntries) / float64(totalEntries)
	evictMsgs := int(float64(middle)*frac + 0.5)
	if evictMsgs <= 0 {
		return keepPrefix, totalMsgs - keepPrefix
	}
	if evictMsgs > middle {
		evictMsgs = middle
	}
	keepSuffix = totalMsgs - keepPrefix - evictMsgs
	return keepPrefix, keepSuffix
}
