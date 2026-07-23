package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestDispatchAppServer_TokenUsage0145Fixture replays
// testdata/codex_appserver/token_usage_0145.jsonl (captured 0.145 shape: three
// upstream responses, cumulative `total`, per-response `last`) through
// dispatchAppServerEvent with a capturing EventEmitter, and asserts each
// EventTokenUsage carries `last`'s exact InputTokens (not `total`'s), and the
// final SessionCost reflects the last cumulative total's fresh/cache split.
func TestDispatchAppServer_TokenUsage0145Fixture(t *testing.T) {
	notifs := loadFixtureNotifications(t, "token_usage_0145.jsonl")
	if len(notifs) != 3 {
		t.Fatalf("loaded %d notifications, want 3", len(notifs))
	}

	pool := setupTestDB(t)
	sessionID := "sess-0145-fixture"
	insertCostTestSession(t, pool, sessionID, "gpt-5.6-terra")
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop(sessionID) })
	RegisterSessionCost(sessionID, "gpt-5.6-terra", pool, clk, nil)

	sink := &testSink{}
	var usages []*EngineUsage
	emit := func(ev EngineEvent) {
		if ev.Type == EventTokenUsage {
			usages = append(usages, ev.Usage)
		}
	}
	for _, n := range notifs {
		dispatchAppServerEvent(sessionID, n, sink, 258400, emit)
	}

	wantLast := []int{15457, 15611, 15748}
	if len(usages) != len(wantLast) {
		t.Fatalf("EventTokenUsage emissions = %d, want %d", len(usages), len(wantLast))
	}
	for i, u := range usages {
		if u == nil {
			t.Fatalf("usages[%d] = nil, want populated Usage", i)
		}
		if u.InputTokens != wantLast[i] {
			t.Errorf("usages[%d].InputTokens = %d, want %d (last, not cumulative total)", i, u.InputTokens, wantLast[i])
		}
	}

	snap, ok := SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false after replaying fixture")
	}
	// Final cumulative total: inputTokens=46816, cachedInputTokens=30208,
	// cacheWriteInputTokens=0, outputTokens=199 -> fresh = 46816-30208-0 = 16608.
	if snap.InputTokens != 16608 {
		t.Errorf("SessionCost.InputTokens = %d, want 16608 (fresh from final cumulative total)", snap.InputTokens)
	}
	if snap.CacheReadTokens != 30208 {
		t.Errorf("SessionCost.CacheReadTokens = %d, want 30208", snap.CacheReadTokens)
	}
	if snap.OutputTokens != 199 {
		t.Errorf("SessionCost.OutputTokens = %d, want 199", snap.OutputTokens)
	}
}

// TestCodexLedgerEmitter_EndToEnd_ExactParity appends a few blocks via the
// codex ledger emitter, then feeds a single EventTokenUsage reconciliation,
// and asserts the ticket's parity invariant: the sum of every non-superseded
// entry's TokensEst equals the reconciled actual exactly, and every entry
// (before and after reconciliation) is Approx==false.
func TestCodexLedgerEmitter_EndToEnd_ExactParity(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-e2e-parity"
	proc := newLedgerCodexTestProc(t, sessionID, 258400)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventToolInvoke, ToolName: "Read", ToolInput: map[string]any{"file_path": "/repo/a.go"}})
	emit(EngineEvent{Type: EventToolResult, ToolName: "Read", Text: "package spawner\nfunc main(){}"})
	emit(EngineEvent{Type: EventText, Text: "some assistant reply text"})

	const actual = 15457
	emit(EngineEvent{Type: EventTokenUsage, Usage: &EngineUsage{InputTokens: actual}})

	snap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok {
		t.Fatal("no ledger snapshot")
	}
	if len(snap.Entries) != 3 {
		t.Fatalf("Entries = %d, want 3", len(snap.Entries))
	}
	var sum int
	for _, e := range snap.Entries {
		if e.Approx {
			t.Errorf("Entries Approx = true, want false (codex ledger is EXACT): %+v", e)
		}
		if !e.Superseded {
			sum += e.TokensEst
		}
	}
	if sum != actual {
		t.Errorf("sum of non-superseded TokensEst = %d, want %d (exact parity with reconciled usage)", sum, actual)
	}
}
