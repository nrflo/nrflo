package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestLedger_AppendDedup_SupersedesBySHA verifies a second append with the
// same content sha marks the prior entry Superseded, keeps append order, and
// excludes the superseded entry from totals but not from the snapshot.
func TestLedger_AppendDedup_SupersedesBySHA(t *testing.T) {
	l := newLedger()
	sha := ledgerSHA([]byte("same content"))

	l.append(LedgerKindToolResult, 10, "tool-a", sha, false)
	l.append(LedgerKindToolResult, 20, "tool-a", sha, false)

	snap := l.snapshot("s1")
	if len(snap.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2 (superseded entry stays in snapshot)", len(snap.Entries))
	}
	if !snap.Entries[0].Superseded {
		t.Errorf("Entries[0].Superseded = false, want true")
	}
	if snap.Entries[1].Superseded {
		t.Errorf("Entries[1].Superseded = true, want false")
	}
	if got := snap.TotalsByKind[LedgerKindToolResult]; got != 20 {
		t.Errorf("TotalsByKind[tool_result] = %d, want 20 (superseded entry excluded)", got)
	}
}

// TestLedger_AppendDedup_FileReadSupersedesByPath verifies file_read entries
// dedup by source path even when content sha differs across reads.
func TestLedger_AppendDedup_FileReadSupersedesByPath(t *testing.T) {
	l := newLedger()

	l.append(LedgerKindFileRead, 10, "/repo/a.txt", ledgerSHA([]byte("v1")), false)
	l.append(LedgerKindFileRead, 15, "/repo/a.txt", ledgerSHA([]byte("v2 different content")), false)

	snap := l.snapshot("s1")
	if len(snap.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(snap.Entries))
	}
	if !snap.Entries[0].Superseded {
		t.Errorf("first file_read entry Superseded = false, want true (same path)")
	}
	if got := snap.TotalsByKind[LedgerKindFileRead]; got != 15 {
		t.Errorf("TotalsByKind[file_read] = %d, want 15", got)
	}
}

// TestLedger_AppendDedup_NoDedupWithoutKey verifies entries with no source
// and no sha (dedupKeyFor returns "") never supersede each other.
func TestLedger_AppendDedup_NoDedupWithoutKey(t *testing.T) {
	l := newLedger()
	l.append(LedgerKindDialog, 5, "", "", false)
	l.append(LedgerKindDialog, 7, "", "", false)

	snap := l.snapshot("s1")
	for i, e := range snap.Entries {
		if e.Superseded {
			t.Errorf("Entries[%d].Superseded = true, want false (no dedup key)", i)
		}
	}
	if got := snap.TotalsByKind[LedgerKindDialog]; got != 12 {
		t.Errorf("TotalsByKind[dialog] = %d, want 12", got)
	}
}

// TestLedger_MarkRef_SubstringBumpsLastRefTurn verifies markRef bumps
// LastRefTurn on entries whose Source matches either direction of the
// substring test, and never on Superseded or Source-less entries.
func TestLedger_MarkRef_SubstringBumpsLastRefTurn(t *testing.T) {
	l := newLedger()
	l.append(LedgerKindFileRead, 10, "/repo/pkg/foo.go", "", false)
	l.append(LedgerKindDialog, 5, "", "", false) // no source: must be skipped
	l.nextTurn()                                 // turn -> 1
	l.nextTurn()                                 // turn -> 2

	l.markRef("read /repo/pkg/foo.go for context")

	snap := l.snapshot("s1")
	if got := snap.Entries[0].LastRefTurn; got != 2 {
		t.Errorf("Entries[0].LastRefTurn = %d, want 2", got)
	}
	if got := snap.Entries[1].LastRefTurn; got != 0 {
		t.Errorf("Entries[1].LastRefTurn = %d, want 0 (no source, never matched)", got)
	}
}

// TestLedger_MarkRef_SkipsSupersededEntries verifies a superseded entry's
// LastRefTurn is never touched even when its Source matches.
func TestLedger_MarkRef_SkipsSupersededEntries(t *testing.T) {
	l := newLedger()
	l.append(LedgerKindFileRead, 10, "/repo/a.txt", "", false)
	l.append(LedgerKindFileRead, 12, "/repo/a.txt", "", false) // supersedes entry 0
	l.nextTurn()

	l.markRef("/repo/a.txt")

	snap := l.snapshot("s1")
	if snap.Entries[0].LastRefTurn != 0 {
		t.Errorf("superseded Entries[0].LastRefTurn = %d, want 0 (untouched)", snap.Entries[0].LastRefTurn)
	}
	if snap.Entries[1].LastRefTurn != 1 {
		t.Errorf("Entries[1].LastRefTurn = %d, want 1", snap.Entries[1].LastRefTurn)
	}
}

// TestLedger_ReconcileUsage_RescalesNonSupersededToActual verifies
// reconcileUsage rescales every non-superseded entry proportionally so their
// sum matches actual, leaves superseded entries alone, and no-ops when
// actual or the current estimate sum is <= 0.
func TestLedger_ReconcileUsage_RescalesNonSupersededToActual(t *testing.T) {
	l := newLedger()
	l.append(LedgerKindDialog, 10, "", ledgerSHA([]byte("a")), false)
	l.append(LedgerKindToolResult, 30, "", ledgerSHA([]byte("b")), false)
	l.append(LedgerKindToolResult, 100, "", ledgerSHA([]byte("b")), false) // supersedes prior via same sha

	l.reconcileUsage(400)

	snap := l.snapshot("s1")
	var total int
	for _, e := range snap.Entries {
		if !e.Superseded {
			total += e.TokensEst
		}
	}
	if total != 400 {
		t.Errorf("reconciled non-superseded total = %d, want 400", total)
	}
	if snap.Entries[1].TokensEst != 30 {
		t.Errorf("superseded entry TokensEst = %d, want untouched 30", snap.Entries[1].TokensEst)
	}

	// no-op guards
	l2 := newLedger()
	l2.append(LedgerKindDialog, 10, "", "", false)
	l2.reconcileUsage(0)
	if got := l2.snapshot("s2").Entries[0].TokensEst; got != 10 {
		t.Errorf("reconcileUsage(0) mutated estimate: got %d, want 10", got)
	}

	l3 := newLedger()
	l3.reconcileUsage(500) // empty ledger: est sum is 0, must no-op without panicking
	if got := len(l3.snapshot("s3").Entries); got != 0 {
		t.Errorf("reconcileUsage on empty ledger created entries: %d", got)
	}
}

// TestLedger_ShouldBroadcast_Debounces verifies the debounce window blocks a
// second broadcast until it elapses, then re-fires exactly once per elapsed
// window using a test clock (no time.Sleep).
func TestLedger_ShouldBroadcast_Debounces(t *testing.T) {
	l := newLedger()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	if !l.shouldBroadcast(now) {
		t.Fatalf("first shouldBroadcast() = false, want true (no prior broadcast)")
	}
	if l.shouldBroadcast(now.Add(100 * time.Millisecond)) {
		t.Errorf("shouldBroadcast() within debounce window = true, want false")
	}
	if l.shouldBroadcast(now.Add(ledgerBroadcastDebounce - time.Nanosecond)) {
		t.Errorf("shouldBroadcast() just under the window = true, want false")
	}
	if !l.shouldBroadcast(now.Add(ledgerBroadcastDebounce)) {
		t.Errorf("shouldBroadcast() at exactly the window = false, want true")
	}
	if l.shouldBroadcast(now.Add(ledgerBroadcastDebounce)) {
		t.Errorf("shouldBroadcast() immediately after a fire = true, want false")
	}
}

// TestLedgerStore_GetCreatesOnFirstAccess_DropRemoves verifies get() lazily
// creates a session's ledger and drop() removes it (a subsequent get()
// starts fresh, and drop on a missing session never panics).
func TestLedgerStore_GetCreatesOnFirstAccess_DropRemoves(t *testing.T) {
	s := newLedgerStore(clock.NewTest(time.Now()))
	s.drop("never-existed") // must not panic

	l := s.get("sess-1")
	l.append(LedgerKindDialog, 5, "", "", false)

	l2 := s.get("sess-1")
	if len(l2.snapshot("sess-1").Entries) != 1 {
		t.Fatalf("second get() returned a different/empty ledger for the same session")
	}

	s.drop("sess-1")
	if _, ok := s.snapshot("sess-1"); ok {
		t.Errorf("snapshot() after drop = ok:true, want ok:false")
	}

	l3 := s.get("sess-1")
	if len(l3.snapshot("sess-1").Entries) != 0 {
		t.Errorf("ledger recreated after drop is not empty")
	}
}

// TestLedgerStore_EpochSummary_TotalsByKindExcludeSuperseded verifies the
// store-level epochSummary aggregates entry count (including superseded) and
// per-kind totals (excluding superseded), and reports ok:false for an
// unknown session.
func TestLedgerStore_EpochSummary_TotalsByKindExcludeSuperseded(t *testing.T) {
	s := newLedgerStore(clock.NewTest(time.Now()))

	if _, ok := s.epochSummary("missing"); ok {
		t.Errorf("epochSummary(missing) ok = true, want false")
	}

	l := s.get("sess-1")
	l.append(LedgerKindDialog, 10, "", ledgerSHA([]byte("x")), false)
	l.append(LedgerKindToolUse, 20, "path", ledgerSHA([]byte("y")), false)
	l.append(LedgerKindFileRead, 30, "path", "", false)
	l.append(LedgerKindFileRead, 40, "path", "", false) // supersedes the 30-token entry

	sum, ok := s.epochSummary("sess-1")
	if !ok {
		t.Fatalf("epochSummary(sess-1) ok = false, want true")
	}
	if sum.EntryCount != 4 {
		t.Errorf("EntryCount = %d, want 4", sum.EntryCount)
	}
	if sum.TotalTokens != 70 {
		t.Errorf("TotalTokens = %d, want 70 (10+20+40, 30 superseded)", sum.TotalTokens)
	}
	if got := sum.TotalsByKind[LedgerKindFileRead]; got != 40 {
		t.Errorf("TotalsByKind[file_read] = %d, want 40", got)
	}
}

// TestLedgerStore_ShouldBroadcast_UnknownSessionIsFalse verifies the store's
// shouldBroadcast helper never fires for a session with no ledger.
func TestLedgerStore_ShouldBroadcast_UnknownSessionIsFalse(t *testing.T) {
	s := newLedgerStore(clock.NewTest(time.Now()))
	if s.shouldBroadcast("missing") {
		t.Errorf("shouldBroadcast(missing) = true, want false")
	}
}

// TestEstTokens_BytesOverFourHeuristic covers the bytes/4 estimate boundary
// cases, including the <=0 guard.
func TestEstTokens_BytesOverFourHeuristic(t *testing.T) {
	cases := []struct {
		nbytes int
		want   int
	}{
		{0, 0},
		{-5, 0},
		{4, 1},
		{7, 1},
		{8, 2},
	}
	for _, tc := range cases {
		if got := estTokens(tc.nbytes); got != tc.want {
			t.Errorf("estTokens(%d) = %d, want %d", tc.nbytes, got, tc.want)
		}
	}
}
