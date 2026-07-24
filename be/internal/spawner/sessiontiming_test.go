package spawner

import (
	"database/sql"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// insertTimingTestSession inserts an agent_sessions row for timing tests,
// reusing setupTestDB's seeded proj/T-1/wfi-1 parents.
func insertTimingTestSession(t *testing.T, pool *db.Pool, id string) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, 'proj', 'T-1', 'wfi-1', 'phase1', 'analyzer', 'sonnet-5', 'running', datetime('now'), datetime('now'))`,
		id)
}

// TestTimingStore_RecordEvent_AccumulatesIntoFourBuckets drives an ordered
// sequence of events across all four buckets and asserts each delta lands in
// the correct cumulative bucket, with the first event only seeding the
// anchor (no delta recorded).
func TestTimingStore_RecordEvent_AccumulatesIntoFourBuckets(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-timing-buckets")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newTimingStore(clk)
	store.register("sess-timing-buckets", pool, clk)

	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	// First event only seeds the anchor.
	store.recordEvent("sess-timing-buckets", "u0", base, TimingBucketThinking)
	// +2s thinking, +3s tool-arg, +1s text, +4s tool-wait.
	store.recordEvent("sess-timing-buckets", "u1", base.Add(2*time.Second), TimingBucketThinking)
	store.recordEvent("sess-timing-buckets", "u2", base.Add(5*time.Second), TimingBucketToolArg)
	store.recordEvent("sess-timing-buckets", "u3", base.Add(6*time.Second), TimingBucketText)
	store.recordEvent("sess-timing-buckets", "u4", base.Add(10*time.Second), TimingBucketToolWait)

	snap, ok := store.snapshot("sess-timing-buckets")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap.ThinkingSec != 2 || snap.ToolArgSec != 3 || snap.TextSec != 1 || snap.ToolWaitSec != 4 {
		t.Errorf("snapshot = %+v, want thinking:2 toolArg:3 text:1 toolWait:4", snap)
	}
}

// TestTimingStore_RecordEvent_OffsetResetDedupIsIdempotent verifies a
// re-read of the same key (e.g. an offset-reset re-tail) does not double
// count the delta, mirroring costEntry.seenUsage semantics.
func TestTimingStore_RecordEvent_OffsetResetDedupIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-timing-dedup")

	clk := clock.NewTest(time.Now())
	store := newTimingStore(clk)
	store.register("sess-timing-dedup", pool, clk)

	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	store.recordEvent("sess-timing-dedup", "u0", base, TimingBucketThinking)
	store.recordEvent("sess-timing-dedup", "u1", base.Add(3*time.Second), TimingBucketThinking)

	snapBefore, _ := store.snapshot("sess-timing-dedup")

	// Re-read the exact same (key, ts, bucket) event — must be a no-op.
	store.recordEvent("sess-timing-dedup", "u1", base.Add(3*time.Second), TimingBucketThinking)

	snapAfter, ok := store.snapshot("sess-timing-dedup")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snapAfter != snapBefore {
		t.Errorf("snapshot after dedup re-read = %+v, want unchanged %+v", snapAfter, snapBefore)
	}
	if snapAfter.ThinkingSec != 3 {
		t.Errorf("ThinkingSec = %v, want 3 (not double counted)", snapAfter.ThinkingSec)
	}
}

// TestTimingStore_RecordEvent_EmptyKeyAlwaysRecords verifies an empty dedup
// key (e.g. codex's coarse wall-clock events) is never treated as a repeat.
func TestTimingStore_RecordEvent_EmptyKeyAlwaysRecords(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-timing-emptykey")

	clk := clock.NewTest(time.Now())
	store := newTimingStore(clk)
	store.register("sess-timing-emptykey", pool, clk)

	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	store.recordEvent("sess-timing-emptykey", "", base, TimingBucketText)
	store.recordEvent("sess-timing-emptykey", "", base.Add(2*time.Second), TimingBucketText)
	store.recordEvent("sess-timing-emptykey", "", base.Add(5*time.Second), TimingBucketText)

	snap, ok := store.snapshot("sess-timing-emptykey")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap.TextSec != 5 {
		t.Errorf("TextSec = %v, want 5 (2 events summed: +2s, +3s)", snap.TextSec)
	}
}

// TestTimingStore_RecordEvent_OutOfOrderEdgeDropped verifies an entry
// timestamp that does not advance past the anchor (clock skew / delivery
// out of order) still advances the anchor but records no negative delta.
func TestTimingStore_RecordEvent_OutOfOrderEdgeDropped(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-timing-outoforder")

	clk := clock.NewTest(time.Now())
	store := newTimingStore(clk)
	store.register("sess-timing-outoforder", pool, clk)

	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	store.recordEvent("sess-timing-outoforder", "u0", base, TimingBucketThinking)
	// Goes backwards relative to the anchor: must not accumulate a negative delta.
	store.recordEvent("sess-timing-outoforder", "u1", base.Add(-1*time.Second), TimingBucketThinking)
	// Forward again, but relative to the *original* anchor since the
	// out-of-order event never advanced it forward.
	store.recordEvent("sess-timing-outoforder", "u2", base.Add(4*time.Second), TimingBucketThinking)

	snap, ok := store.snapshot("sess-timing-outoforder")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap.ThinkingSec != 4 {
		t.Errorf("ThinkingSec = %v, want 4 (out-of-order edge dropped, not negative or double)", snap.ThinkingSec)
	}
}

// TestTimingStore_RecordEvent_UnknownSessionIsNoOp verifies recordEvent on a
// session with no registered entry never panics and stays a no-op.
func TestTimingStore_RecordEvent_UnknownSessionIsNoOp(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	store := newTimingStore(clk)

	store.recordEvent("sess-timing-unknown", "u0", time.Now(), TimingBucketText)

	if _, ok := store.snapshot("sess-timing-unknown"); ok {
		t.Error("snapshot ok = true for a never-registered session, want false")
	}
}

// TestTimingStore_MaybeFlush_Debounces verifies the DB write is debounced
// (only the first call and calls past the debounce window write), mirroring
// TestCostStore_AddUsage_ComputesCostAndDebouncesFlush.
func TestTimingStore_MaybeFlush_Debounces(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-timing-flush")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newTimingStore(clk)
	store.register("sess-timing-flush", pool, clk)

	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	store.recordEvent("sess-timing-flush", "u0", base, TimingBucketThinking)
	// First real delta: lastFlush is the zero Time -> flush fires immediately.
	store.recordEvent("sess-timing-flush", "u1", base.Add(2*time.Second), TimingBucketThinking)

	var raw sql.NullString
	if err := pool.QueryRow(`SELECT time_buckets_json FROM agent_sessions WHERE id = ?`, "sess-timing-flush").Scan(&raw); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !raw.Valid {
		t.Fatal("time_buckets_json is NULL after first flush, want a value")
	}
	first := raw.String

	// Inside the debounce window: DB row must be unchanged even though the
	// in-memory snapshot grows.
	store.recordEvent("sess-timing-flush", "u2", base.Add(3*time.Second), TimingBucketThinking)
	if err := pool.QueryRow(`SELECT time_buckets_json FROM agent_sessions WHERE id = ?`, "sess-timing-flush").Scan(&raw); err != nil {
		t.Fatalf("query: %v", err)
	}
	if raw.String != first {
		t.Errorf("time_buckets_json changed within debounce window: %q -> %q", first, raw.String)
	}

	clk.Advance(timingFlushDebounce)
	store.recordEvent("sess-timing-flush", "u3", base.Add(4*time.Second), TimingBucketThinking)
	if err := pool.QueryRow(`SELECT time_buckets_json FROM agent_sessions WHERE id = ?`, "sess-timing-flush").Scan(&raw); err != nil {
		t.Fatalf("query: %v", err)
	}
	if raw.String == first {
		t.Errorf("time_buckets_json unchanged after debounce window elapsed, want an updated flush")
	}
}

// TestFinalizeSessionTiming_FlushesImmediatelyAndDrops verifies the
// process-global wrappers (Register/Record/Finalize) mirror
// FinalizeSessionCost's flush-then-drop semantics: the last pending delta
// (inside the debounce window) is not lost.
func TestFinalizeSessionTiming_FlushesImmediatelyAndDrops(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertTimingTestSession(t, pool, "sess-timing-finalize")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalTimingStore.drop("sess-timing-finalize") })

	RegisterSessionTiming("sess-timing-finalize", pool, clk)
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	RecordSessionTimingEvent("sess-timing-finalize", "u0", base, TimingBucketToolWait)
	RecordSessionTimingEvent("sess-timing-finalize", "u1", base.Add(6*time.Second), TimingBucketToolWait)
	// A second event within the same debounce window would normally be
	// dropped from the DB — prove FinalizeSessionTiming force-flushes it.
	RecordSessionTimingEvent("sess-timing-finalize", "u2", base.Add(9*time.Second), TimingBucketToolWait)

	FinalizeSessionTiming("sess-timing-finalize")

	var raw sql.NullString
	if err := pool.QueryRow(`SELECT time_buckets_json FROM agent_sessions WHERE id = ?`, "sess-timing-finalize").Scan(&raw); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !raw.Valid {
		t.Fatal("time_buckets_json is NULL after FinalizeSessionTiming, want a flushed value")
	}
	if want := `"tool_wait_sec":9`; !containsSubstr(raw.String, want) {
		t.Errorf("time_buckets_json = %q, want it to contain %q (6+3=9s summed)", raw.String, want)
	}

	if _, ok := globalTimingStore.snapshot("sess-timing-finalize"); ok {
		t.Error("snapshot ok = true after FinalizeSessionTiming, want false (entry dropped)")
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
