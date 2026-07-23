package spawner

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
)

// TestDispatchContextCompaction_ResetsContextAndRecordsSystemMessage verifies
// dispatchContextCompaction resets context_left to 100 via exactly one
// UpdateContextLeft call, records exactly one `system`-category message, and
// emits EventContextCompacted.
func TestDispatchContextCompaction_ResetsContextAndRecordsSystemMessage(t *testing.T) {
	sink := &testSink{}
	var events []EngineEvent
	emit := func(ev EngineEvent) { events = append(events, ev) }

	dispatchContextCompaction("sess-compact-1", sink, emit)

	if len(sink.contextUpdates) != 1 {
		t.Fatalf("UpdateContextLeft called %d times, want 1: %v", len(sink.contextUpdates), sink.contextUpdates)
	}
	if sink.contextUpdates[0] != 100 {
		t.Errorf("UpdateContextLeft pct = %d, want 100", sink.contextUpdates[0])
	}

	if len(sink.recordedMsgs) != 1 {
		t.Fatalf("RecordHookMessage called %d times, want 1: %+v", len(sink.recordedMsgs), sink.recordedMsgs)
	}
	if sink.recordedMsgs[0].category != "system" {
		t.Errorf("recorded message category = %q, want \"system\": %+v", sink.recordedMsgs[0].category, sink.recordedMsgs[0])
	}

	var sawCompacted bool
	for _, ev := range events {
		if ev.Type == EventContextCompacted {
			sawCompacted = true
			if ev.SessionID != "sess-compact-1" {
				t.Errorf("EventContextCompacted.SessionID = %q, want sess-compact-1", ev.SessionID)
			}
		}
	}
	if !sawCompacted {
		t.Errorf("EventContextCompacted not emitted: %+v", events)
	}
}

// TestDispatchContextCompaction_NilEmitter verifies dispatchContextCompaction
// still performs the sink-side reset when emit is nil (autonomous sink-only
// path), matching EventEmitter.emit's nil-safety.
func TestDispatchContextCompaction_NilEmitter(t *testing.T) {
	sink := &testSink{}
	dispatchContextCompaction("sess-compact-nil", sink, nil)

	if len(sink.contextUpdates) != 1 || sink.contextUpdates[0] != 100 {
		t.Errorf("UpdateContextLeft = %v, want [100]", sink.contextUpdates)
	}
	if len(sink.recordedMsgs) != 1 {
		t.Errorf("RecordHookMessage calls = %d, want 1", len(sink.recordedMsgs))
	}
}

// TestDispatchCompletedItem_CompactedSignal table-tests every item/completed
// type: only a completed `contextCompaction` item reports compacted=true.
func TestDispatchCompletedItem_CompactedSignal(t *testing.T) {
	tests := []struct {
		name string
		item string
	}{
		{"agentMessage", `{"id":"i1","type":"agentMessage","text":"hi"}`},
		{"reasoning", `{"id":"i2","type":"reasoning","content":["thinking"]}`},
		{"commandExecution", `{"id":"i3","type":"commandExecution","command":"echo hi","exitCode":0}`},
		{"mcpToolCall", `{"id":"i4","type":"mcpToolCall","server":"nrflo","tool":"findings_add","arguments":{}}`},
		{"webSearch", `{"id":"i5","type":"webSearch","query":"golang"}`},
		{"userMessage", `{"id":"i6","type":"userMessage"}`},
		{"contextCompaction", `{"id":"i7","type":"contextCompaction"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sink := &testSink{}
			params := json.RawMessage(`{"item":` + tc.item + `}`)
			sig := dispatchAppServerEvent("sess-signal", rpcEnvelope{Method: "item/completed", Params: params}, sink, 200000, nil)

			want := tc.name == "contextCompaction"
			if sig.compacted != want {
				t.Errorf("appServerSignal.compacted for item type %q = %v, want %v", tc.name, sig.compacted, want)
			}
		})
	}
}

// TestDispatchTokenUsage_ZeroInputCheckpoint_NoContextOrEventUpdate covers the
// compaction-only tokenUsage checkpoint shape (last.inputTokens==0): no
// UpdateContextLeft call, no EventTokenUsage emission, and SessionCost
// unchanged (total is byte-identical to the pre-checkpoint event, so the
// SetSessionCostUsage overwrite is a no-op). Regression-guards the exact
// pre-fix value: the checkpoint's cumulative total (27268 in, 258400 window)
// would have computed ComputeContextLeftPct(27268, 258400)==90 had the
// used==0 fallback still read from `total` — that value must never be
// written to UpdateContextLeft.
func TestDispatchTokenUsage_ZeroInputCheckpoint_NoContextOrEventUpdate(t *testing.T) {
	pool := setupTestDB(t)
	sessionID := "sess-zero-input-checkpoint"
	insertCostTestSession(t, pool, sessionID, "gpt-5.6-terra")
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop(sessionID) })
	RegisterSessionCost(sessionID, "gpt-5.6-terra", pool, clk, nil)

	sink := &testSink{}
	var usageEvents int
	emit := func(ev EngineEvent) {
		if ev.Type == EventTokenUsage {
			usageEvents++
		}
	}

	// Pre-compaction real usage establishes the baseline SessionCost snapshot
	// and the cumulative total the checkpoint will repeat byte-identically.
	// last.inputTokens (5000) is deliberately distinct from total.inputTokens
	// (27268) so the forbidden-value check below cannot false-positive on this
	// legitimate prior context update.
	pre := json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":27268,"outputTokens":100,"cachedInputTokens":0,"cacheWriteInputTokens":0},"last":{"inputTokens":5000},"modelContextWindow":258400}}`)
	dispatchTokenUsage(sessionID, pre, sink, 258400, emit)
	before, ok := SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false after priming event")
	}
	contextUpdatesBefore := len(sink.contextUpdates)
	usageEventsBefore := usageEvents

	// Compaction checkpoint: last.inputTokens==0, total unchanged.
	checkpoint := json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":27268,"outputTokens":100,"cachedInputTokens":0,"cacheWriteInputTokens":0},"last":{"totalTokens":4972,"inputTokens":0},"modelContextWindow":258400}}`)
	dispatchTokenUsage(sessionID, checkpoint, sink, 258400, emit)

	if len(sink.contextUpdates) != contextUpdatesBefore {
		t.Errorf("UpdateContextLeft called on zero-input checkpoint: contextUpdates=%v", sink.contextUpdates)
	}
	if usageEvents != usageEventsBefore {
		t.Errorf("EventTokenUsage emitted on zero-input checkpoint: count=%d, want %d", usageEvents, usageEventsBefore)
	}
	forbidden := ComputeContextLeftPct(27268, 258400)
	for _, pct := range sink.contextUpdates {
		if pct == forbidden {
			t.Errorf("UpdateContextLeft received %d, the pre-fix value computed from cumulative total — must never be written", forbidden)
		}
	}

	after, ok := SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false after checkpoint")
	}
	if after != before {
		t.Errorf("SessionCost changed across zero-input checkpoint: before=%+v after=%+v", before, after)
	}
}

// TestDispatchAppServer_Compaction0145Fixture replays a captured 0.145
// contextCompaction sequence (item/started -> zero-input tokenUsage checkpoint
// -> item/completed{contextCompaction} -> agentMessage -> real tokenUsage) and
// asserts the end-to-end shape: exactly one context_left=100 reset from the
// compaction, followed by exactly one real percentage from the post-compaction
// usage report, with no context update ever derived from the checkpoint's
// cumulative total.
func TestDispatchAppServer_Compaction0145Fixture(t *testing.T) {
	notifs := loadFixtureNotifications(t, "compaction_0145.jsonl")
	if len(notifs) == 0 {
		t.Fatal("no notifications loaded from fixture")
	}

	sink := &testSink{}
	var sawCompacted bool
	var compactedSignals int
	for _, n := range notifs {
		sig := dispatchAppServerEvent("sess-compaction-fixture", n, sink, 258400, nil)
		if sig.compacted {
			sawCompacted = true
			compactedSignals++
		}
	}

	if !sawCompacted {
		t.Fatal("no appServerSignal.compacted seen while replaying compaction_0145.jsonl")
	}
	if compactedSignals != 1 {
		t.Errorf("compacted signals = %d, want 1", compactedSignals)
	}

	if len(sink.contextUpdates) != 2 {
		t.Fatalf("UpdateContextLeft calls = %d, want 2 (compaction reset + one real report): %v", len(sink.contextUpdates), sink.contextUpdates)
	}
	if sink.contextUpdates[0] != 100 {
		t.Errorf("first UpdateContextLeft = %d, want 100 (compaction reset)", sink.contextUpdates[0])
	}
	forbidden := ComputeContextLeftPct(27268, 258400)
	for _, pct := range sink.contextUpdates {
		if pct == forbidden {
			t.Errorf("UpdateContextLeft received %d, the value the compaction checkpoint's cumulative total would produce pre-fix", forbidden)
		}
	}

	var systemMsgs int
	for _, m := range sink.recordedMsgs {
		if m.category == "system" {
			systemMsgs++
		}
	}
	if systemMsgs != 1 {
		t.Errorf("system-category messages = %d, want 1", systemMsgs)
	}
}

// TestCodexLedgerEmitter_CompactionResetsThenReconciles verifies EventContextCompacted
// supersedes every pre-compaction ledger entry, and the following real
// EventTokenUsage reconciles the non-superseded sum to exactly that event's
// InputTokens.
func TestCodexLedgerEmitter_CompactionResetsThenReconciles(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-compaction-reset"
	proc := newLedgerCodexTestProc(t, sessionID, 258400)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventToolInvoke, ToolName: "Read", ToolInput: map[string]any{"file_path": "/repo/a.go"}})
	emit(EngineEvent{Type: EventToolResult, ToolName: "Read", Text: "package spawner\nfunc main(){}"})
	emit(EngineEvent{Type: EventText, Text: "pre-compaction reply"})

	preSnap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok || len(preSnap.Entries) != 3 {
		t.Fatalf("pre-compaction snapshot = %+v ok=%v, want 3 entries", preSnap, ok)
	}

	emit(EngineEvent{Type: EventContextCompacted})

	afterReset, ok := globalLedgerStore.snapshot(sessionID)
	if !ok {
		t.Fatal("no ledger snapshot after EventContextCompacted")
	}
	if len(afterReset.Entries) != 4 {
		t.Fatalf("Entries after reset = %d, want 4 (3 superseded + 1 placeholder)", len(afterReset.Entries))
	}
	for i, e := range afterReset.Entries[:3] {
		if !e.Superseded {
			t.Errorf("pre-compaction Entries[%d].Superseded = false, want true: %+v", i, e)
		}
	}
	placeholder := afterReset.Entries[3]
	if placeholder.Superseded {
		t.Errorf("placeholder entry Superseded = true, want false: %+v", placeholder)
	}
	if placeholder.Kind != LedgerKindInjected {
		t.Errorf("placeholder entry Kind = %q, want %q", placeholder.Kind, LedgerKindInjected)
	}

	const actual = 13864
	emit(EngineEvent{Type: EventTokenUsage, Usage: &EngineUsage{InputTokens: actual}})

	finalSnap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok {
		t.Fatal("no ledger snapshot after post-compaction EventTokenUsage")
	}
	var sum int
	for _, e := range finalSnap.Entries {
		if !e.Superseded {
			sum += e.TokensEst
		}
	}
	if sum != actual {
		t.Errorf("sum of non-superseded TokensEst after post-compaction reconcile = %d, want %d", sum, actual)
	}
	for i, e := range finalSnap.Entries[:3] {
		if !e.Superseded {
			t.Errorf("pre-compaction Entries[%d] un-superseded by reconcile, want it to stay superseded: %+v", i, e)
		}
	}
}
