package spawner

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
)

// TestClassifyTranscriptEntry_Precedence verifies assistant-entry bucket
// classification precedence (tool_use > thinking > text), user tool_result
// mapping to tool-wait, and edge cases (empty content, unknown block types,
// malformed JSON) all degrade to ok=false rather than a wrong bucket.
func TestClassifyTranscriptEntry_Precedence(t *testing.T) {
	tests := []struct {
		name       string
		entryType  string
		raw        string
		wantBucket TimingBucket
		wantOK     bool
	}{
		{
			name:      "assistant tool_use wins over thinking and text",
			entryType: "assistant",
			raw: `[{"type":"text","text":"hi"},{"type":"thinking","thinking":"hmm"},` +
				`{"type":"tool_use","id":"tu1","name":"Read"}]`,
			wantBucket: TimingBucketToolArg,
			wantOK:     true,
		},
		{
			name:       "assistant thinking wins over text",
			entryType:  "assistant",
			raw:        `[{"type":"text","text":"hi"},{"type":"thinking","thinking":"hmm"}]`,
			wantBucket: TimingBucketThinking,
			wantOK:     true,
		},
		{
			name:       "assistant text alone",
			entryType:  "assistant",
			raw:        `[{"type":"text","text":"hi"}]`,
			wantBucket: TimingBucketText,
			wantOK:     true,
		},
		{
			name:       "user tool_result maps to tool wait",
			entryType:  "user",
			raw:        `[{"type":"tool_result","tool_use_id":"tu1","content":"result"}]`,
			wantBucket: TimingBucketToolWait,
			wantOK:     true,
		},
		{
			name:      "user non-tool_result block classifies as nothing",
			entryType: "user",
			raw:       `[{"type":"text","text":"plain reply"}]`,
			wantOK:    false,
		},
		{
			name:      "empty content array",
			entryType: "assistant",
			raw:       `[]`,
			wantOK:    false,
		},
		{
			name:      "unknown block type only",
			entryType: "assistant",
			raw:       `[{"type":"redacted_thinking"}]`,
			wantOK:    false,
		},
		{
			name:      "unknown entry type",
			entryType: "system",
			raw:       `[{"type":"text","text":"hi"}]`,
			wantOK:    false,
		},
		{
			name:      "malformed JSON",
			entryType: "assistant",
			raw:       `not json`,
			wantOK:    false,
		},
		{
			name:      "empty raw",
			entryType: "assistant",
			raw:       ``,
			wantOK:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bucket, ok := classifyTranscriptEntry(tc.entryType, json.RawMessage(tc.raw))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && bucket != tc.wantBucket {
				t.Errorf("bucket = %v, want %v", bucket, tc.wantBucket)
			}
		})
	}
}

// TestRecordTranscriptTiming_CraftedSequence drives a full thinking ->
// tool_use -> tool_result -> text sequence with known timestamps through
// recordTranscriptTiming and asserts each delta lands in the expected
// bucket (thinking/tool-arg/tool-wait/text), matching the acceptance check
// that summed buckets ~= (lastTS - firstTS).
func TestRecordTranscriptTiming_CraftedSequence(t *testing.T) {
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-transcript-timing")

	clk := clock.NewTest(time.Now())
	globalTimingStore.register("sess-transcript-timing", pool, clk)
	t.Cleanup(func() { globalTimingStore.drop("sess-transcript-timing") })

	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	// t0: thinking (seeds anchor, no delta yet).
	recordTranscriptTiming("sess-transcript-timing", "assistant", base, "u0",
		json.RawMessage(`[{"type":"thinking","thinking":"pondering"}]`))
	// t0+2s: tool_use -> the 2s gap is attributed to tool_use's own bucket
	// (ToolArg), per the "delta rides with the following block" rule.
	recordTranscriptTiming("sess-transcript-timing", "assistant", base.Add(2*time.Second), "u1",
		json.RawMessage(`[{"type":"tool_use","id":"tu1","name":"Read"}]`))
	// t0+7s: tool_result -> 5s tool-wait.
	recordTranscriptTiming("sess-transcript-timing", "user", base.Add(7*time.Second), "u2",
		json.RawMessage(`[{"type":"tool_result","tool_use_id":"tu1","content":"data"}]`))
	// t0+8s: text -> 1s text.
	recordTranscriptTiming("sess-transcript-timing", "assistant", base.Add(8*time.Second), "u3",
		json.RawMessage(`[{"type":"text","text":"done"}]`))

	snap, ok := globalTimingStore.snapshot("sess-transcript-timing")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap.ThinkingSec != 0 {
		t.Errorf("ThinkingSec = %v, want 0 (first event only seeds the anchor)", snap.ThinkingSec)
	}
	if snap.ToolArgSec != 2 {
		t.Errorf("ToolArgSec = %v, want 2", snap.ToolArgSec)
	}
	if snap.ToolWaitSec != 5 {
		t.Errorf("ToolWaitSec = %v, want 5", snap.ToolWaitSec)
	}
	if snap.TextSec != 1 {
		t.Errorf("TextSec = %v, want 1", snap.TextSec)
	}
	total := snap.ThinkingSec + snap.ToolArgSec + snap.TextSec + snap.ToolWaitSec
	wantTotal := base.Add(8 * time.Second).Sub(base).Seconds()
	if total != wantTotal {
		t.Errorf("sum of buckets = %v, want %v (lastTS - firstTS)", total, wantTotal)
	}
}

// TestRecordTranscriptTiming_ZeroTimestampDropped verifies a missing/
// unparseable timestamp (IsZero) is dropped rather than corrupting the
// running anchor — the render-nothing degrade path for malformed lines.
func TestRecordTranscriptTiming_ZeroTimestampDropped(t *testing.T) {
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-transcript-zero-ts")

	clk := clock.NewTest(time.Now())
	globalTimingStore.register("sess-transcript-zero-ts", pool, clk)
	t.Cleanup(func() { globalTimingStore.drop("sess-transcript-zero-ts") })

	recordTranscriptTiming("sess-transcript-zero-ts", "assistant", time.Time{}, "u0",
		json.RawMessage(`[{"type":"text","text":"hi"}]`))

	snap, ok := globalTimingStore.snapshot("sess-transcript-zero-ts")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap != (TimingSnapshot{}) {
		t.Errorf("snapshot = %+v, want zero value (zero timestamp dropped, anchor untouched)", snap)
	}
}

// TestIngestClaudeTranscriptLine_FeedsTimingFromTopLevelFields verifies the
// full line parse (ingestClaudeTranscriptLine) extracts the top-level
// timestamp+uuid siblings of type/message and feeds them into the timing
// store, exercising the real call path (not the unit helper directly).
func TestIngestClaudeTranscriptLine_FeedsTimingFromTopLevelFields(t *testing.T) {
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-line-timing")

	clk := clock.NewTest(time.Now())
	globalTimingStore.register("sess-line-timing", pool, clk)
	t.Cleanup(func() { globalTimingStore.drop("sess-line-timing") })
	t.Cleanup(func() { globalLedgerStore.drop("sess-line-timing") })

	l := newLedger()
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	line1 := []byte(`{"type":"assistant","uuid":"u0","timestamp":"` + base.Format(time.RFC3339Nano) +
		`","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}` + "\n")
	line2 := []byte(`{"type":"assistant","uuid":"u1","timestamp":"` + base.Add(3*time.Second).Format(time.RFC3339Nano) +
		`","message":{"role":"assistant","content":[{"type":"text","text":"more"}]}}` + "\n")

	ingestClaudeTranscriptLine("sess-line-timing", l, line1)
	ingestClaudeTranscriptLine("sess-line-timing", l, line2)

	snap, ok := globalTimingStore.snapshot("sess-line-timing")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap.TextSec != 3 {
		t.Errorf("TextSec = %v, want 3", snap.TextSec)
	}
}

// TestIngestClaudeTranscriptLine_MissingTimestampSkipsTimingNoPanic verifies
// a line with no timestamp field at all degrades gracefully (no timing
// recorded, no panic) while still ingesting the ledger entry normally.
func TestIngestClaudeTranscriptLine_MissingTimestampSkipsTimingNoPanic(t *testing.T) {
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-line-no-ts")

	clk := clock.NewTest(time.Now())
	globalTimingStore.register("sess-line-no-ts", pool, clk)
	t.Cleanup(func() { globalTimingStore.drop("sess-line-no-ts") })
	t.Cleanup(func() { globalLedgerStore.drop("sess-line-no-ts") })

	l := newLedger()
	line := []byte(ledgerCliAssistantLine(`{"type":"text","text":"no timestamp here"}`))
	ingestClaudeTranscriptLine("sess-line-no-ts", l, line)

	snap, ok := globalTimingStore.snapshot("sess-line-no-ts")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap != (TimingSnapshot{}) {
		t.Errorf("snapshot = %+v, want zero value (no timestamp -> no timing recorded)", snap)
	}
	if got := len(l.snapshot("sess-line-no-ts").Entries); got != 1 {
		t.Errorf("ledger entries = %d, want 1 (ledger ingest unaffected by missing timestamp)", got)
	}
}
