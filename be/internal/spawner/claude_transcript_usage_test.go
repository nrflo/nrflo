package spawner

import (
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
)

// claudeTranscriptUsageLine builds one assistant JSONL transcript line
// carrying uuid + message.id + a usage block, using the exact Claude CLI key
// names (input_tokens/cache_read_input_tokens/cache_creation_input_tokens/
// output_tokens) that ingestClaudeTranscriptUsage parses.
func claudeTranscriptUsageLine(uuid, msgID string, in, out, cacheRead, cacheWrite int) string {
	return fmt.Sprintf(
		`{"type":"assistant","uuid":%q,"message":{"id":%q,"role":"assistant","content":[{"type":"text","text":"hi"}],`+
			`"usage":{"input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"output_tokens":%d}}}`+"\n",
		uuid, msgID, in, cacheRead, cacheWrite, out,
	)
}

// TestSpawnerIngestClaudeTranscript_UsageFeedsSessionCost drives the workflow
// tailer (Spawner.ingestClaudeTranscript) against a fixture transcript with
// several assistant entries carrying uuid+usage, and asserts the resulting
// SessionCost snapshot equals the summed usage (acceptance #1).
func TestSpawnerIngestClaudeTranscript_UsageFeedsSessionCost(t *testing.T) {
	pool := setupTestDB(t)
	sessionID := "sess-workflow-fixture"
	insertCostTestSession(t, pool, sessionID, "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop(sessionID) })
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
	RegisterSessionCost(sessionID, "sonnet-5", pool, clk, nil)

	dir := t.TempDir()
	path := dir + "/transcript.jsonl"
	content := claudeTranscriptUsageLine("u1", "m1", 6_000, 1_500, 1_000, 200) +
		claudeTranscriptUsageLine("u2", "m2", 4_000, 500, 100, 0) +
		claudeTranscriptUsageLine("u3", "m3", 2_000, 300, 0, 50)
	writeRawTranscript(t, path, content)

	s := New(Config{Clock: clk})
	s.ingestClaudeTranscript(sessionID, path)

	snap, ok := SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false after fixture ingest")
	}
	wantIn, wantOut, wantCacheRd, wantCacheWr := 12_000, 2_300, 1_100, 250
	if snap.InputTokens != wantIn || snap.OutputTokens != wantOut ||
		snap.CacheReadTokens != wantCacheRd || snap.CacheWriteTokens != wantCacheWr {
		t.Errorf("token snapshot = %+v, want in:%d out:%d cacheRd:%d cacheWr:%d",
			snap, wantIn, wantOut, wantCacheRd, wantCacheWr)
	}
	// sonnet-5: price_in=3, price_out=15, cache_write=3.75, cache_read=0.3 per MTok.
	want := float64(wantIn)/1e6*3 + float64(wantOut)/1e6*15 + float64(wantCacheWr)/1e6*3.75 + float64(wantCacheRd)/1e6*0.3
	if diff := snap.CostUSD - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("CostUSD = %v, want %v", snap.CostUSD, want)
	}
}

// TestSpawnerIngestClaudeTranscript_OffsetResetDedupsByUUID verifies acceptance
// #2 (offset reset): tail once, then truncate/rewrite the file smaller than
// the stored offset (forcing a restart-at-0 re-read of the same uuid'd line),
// tail again — the cost counters must be unchanged since the dedup key was
// already seen.
func TestSpawnerIngestClaudeTranscript_OffsetResetDedupsByUUID(t *testing.T) {
	pool := setupTestDB(t)
	sessionID := "sess-offset-reset"
	insertCostTestSession(t, pool, sessionID, "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop(sessionID) })
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
	RegisterSessionCost(sessionID, "sonnet-5", pool, clk, nil)

	dir := t.TempDir()
	path := dir + "/transcript.jsonl"
	// A long padding line first so the file is large enough that the single
	// usage line alone (rewritten below) is guaranteed smaller than the
	// current offset.
	padding := claudeTranscriptUsageLine("u-pad", "m-pad", 100, 100, 0, 0)
	usageLine := claudeTranscriptUsageLine("u1", "m1", 6_000, 1_500, 1_000, 200)
	writeRawTranscript(t, path, padding+usageLine)

	s := New(Config{Clock: clk})
	s.ingestClaudeTranscript(sessionID, path)

	snap, ok := SessionCost(sessionID)
	if !ok || snap.InputTokens != 6_100 || snap.OutputTokens != 1_600 {
		t.Fatalf("after first tail: snapshot=%+v ok=%v, want in:6100 out:1600", snap, ok)
	}

	// Rotate: replace with just the (already-billed) usage line — shorter than
	// the stored offset, forcing a restart-at-0 re-read of the same uuid.
	if len(usageLine) >= len(padding+usageLine) {
		t.Fatal("test fixture invariant broken: rotated content must be shorter than the original")
	}
	writeRawTranscript(t, path, usageLine)

	s.ingestClaudeTranscript(sessionID, path)

	snap, ok = SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false after offset-reset re-tail")
	}
	if snap.InputTokens != 6_100 || snap.OutputTokens != 1_600 || snap.CacheReadTokens != 1_000 || snap.CacheWriteTokens != 200 {
		t.Errorf("token snapshot after offset-reset re-read = %+v, want unchanged in:6100 out:1600 cacheRd:1000 cacheWr:200 (dedup by uuid)", snap)
	}
}

// TestSpawnerIngestClaudeTranscript_SessionSwapNoBleed verifies acceptance #2
// (relaunch/session swap): two sessionIDs, each with its own transcript, each
// bill only their own usage — and finalizing the first session (as a relaunch
// would) does not disturb the second's running cost.
func TestSpawnerIngestClaudeTranscript_SessionSwapNoBleed(t *testing.T) {
	pool := setupTestDB(t)
	sessA, sessB := "sess-swap-a", "sess-swap-b"
	insertCostTestSession(t, pool, sessA, "sonnet-5")
	insertCostTestSession(t, pool, sessB, "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop(sessA) })
	t.Cleanup(func() { globalCostStore.drop(sessB) })
	t.Cleanup(func() { globalLedgerStore.drop(sessA) })
	t.Cleanup(func() { globalLedgerStore.drop(sessB) })
	RegisterSessionCost(sessA, "sonnet-5", pool, clk, nil)
	RegisterSessionCost(sessB, "sonnet-5", pool, clk, nil)

	dir := t.TempDir()
	pathA := dir + "/a/transcript.jsonl"
	pathB := dir + "/b/transcript.jsonl"
	writeRawTranscript(t, pathA, claudeTranscriptUsageLine("ua1", "ma1", 1_000, 200, 0, 0))
	writeRawTranscript(t, pathB, claudeTranscriptUsageLine("ub1", "mb1", 5_000, 900, 0, 0))

	s := New(Config{Clock: clk})
	s.ingestClaudeTranscript(sessA, pathA)
	s.ingestClaudeTranscript(sessB, pathB)

	snapA, okA := SessionCost(sessA)
	snapB, okB := SessionCost(sessB)
	if !okA || snapA.InputTokens != 1_000 || snapA.OutputTokens != 200 {
		t.Fatalf("session A snapshot = %+v ok=%v, want in:1000 out:200", snapA, okA)
	}
	if !okB || snapB.InputTokens != 5_000 || snapB.OutputTokens != 900 {
		t.Fatalf("session B snapshot = %+v ok=%v, want in:5000 out:900", snapB, okB)
	}

	// Finalize (relaunch/kill) session A — must not touch B's running cost.
	FinalizeSessionCost(sessA)
	if _, ok := SessionCost(sessA); ok {
		t.Error("SessionCost ok = true for session A after FinalizeSessionCost, want false")
	}
	snapB2, okB2 := SessionCost(sessB)
	if !okB2 || snapB2.InputTokens != 5_000 || snapB2.OutputTokens != 900 {
		t.Errorf("session B snapshot after A's finalize = %+v ok=%v, want unchanged in:5000 out:900", snapB2, okB2)
	}
}

// TestSpawnerIngestClaudeTranscript_NoUsageAndEmptyUUIDLines guards the
// existing console-fixture behavior end to end through the workflow tailer: a
// line with no usage block leaves cost untouched, and a line with usage but no
// uuid/message.id (empty dedup key) still bills — usage must never be dropped
// just because the transcript entry has no stable id.
func TestSpawnerIngestClaudeTranscript_NoUsageAndEmptyUUIDLines(t *testing.T) {
	pool := setupTestDB(t)
	sessionID := "sess-no-usage-empty-uuid"
	insertCostTestSession(t, pool, sessionID, "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop(sessionID) })
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
	RegisterSessionCost(sessionID, "sonnet-5", pool, clk, nil)

	dir := t.TempDir()
	path := dir + "/transcript.jsonl"
	noUsageLine := `{"type":"assistant","uuid":"u-no-usage","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}` + "\n"
	emptyUUIDLine := claudeTranscriptUsageLine("", "", 3_000, 700, 0, 0)
	writeRawTranscript(t, path, noUsageLine+emptyUUIDLine)

	s := New(Config{Clock: clk})
	s.ingestClaudeTranscript(sessionID, path)

	snap, ok := SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false")
	}
	if snap.InputTokens != 3_000 || snap.OutputTokens != 700 {
		t.Errorf("token snapshot = %+v, want in:3000 out:700 (no-usage line untouched, empty-uuid line billed)", snap)
	}
}

// TestIngestClaudeTranscriptUsage_DedupIdempotent is the shared-helper unit
// test (Rule 6): calling ingestClaudeTranscriptUsage twice with the exact same
// line (same uuid) must bill exactly once, independent of either tailer.
func TestIngestClaudeTranscriptUsage_DedupIdempotent(t *testing.T) {
	pool := setupTestDB(t)
	sessionID := "sess-shared-helper-dedup"
	insertCostTestSession(t, pool, sessionID, "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop(sessionID) })
	RegisterSessionCost(sessionID, "sonnet-5", pool, clk, nil)

	line := []byte(claudeTranscriptUsageLine("u-dup", "m-dup", 2_000, 400, 100, 10))

	ingestClaudeTranscriptUsage(sessionID, line)
	ingestClaudeTranscriptUsage(sessionID, line)
	ingestClaudeTranscriptUsage(sessionID, line)

	snap, ok := SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false")
	}
	if snap.InputTokens != 2_000 || snap.OutputTokens != 400 || snap.CacheReadTokens != 100 || snap.CacheWriteTokens != 10 {
		t.Errorf("token snapshot after 3x identical line = %+v, want billed exactly once (in:2000 out:400 cacheRd:100 cacheWr:10)", snap)
	}
}

// TestIngestClaudeTranscriptUsage_NonAssistantAndMalformedLinesIgnored
// verifies the shared helper never bills or panics on a non-assistant type or
// malformed JSON.
func TestIngestClaudeTranscriptUsage_NonAssistantAndMalformedLinesIgnored(t *testing.T) {
	pool := setupTestDB(t)
	sessionID := "sess-shared-helper-ignored"
	insertCostTestSession(t, pool, sessionID, "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop(sessionID) })
	RegisterSessionCost(sessionID, "sonnet-5", pool, clk, nil)

	userLine := `{"type":"user","uuid":"u-user","message":{"id":"m-user","usage":{"input_tokens":9999,"output_tokens":9999}}}` + "\n"
	ingestClaudeTranscriptUsage(sessionID, []byte(userLine))
	ingestClaudeTranscriptUsage(sessionID, []byte(`{not valid json`))

	snap, ok := SessionCost(sessionID)
	if !ok {
		t.Fatal("SessionCost ok = false")
	}
	if snap.InputTokens != 0 || snap.OutputTokens != 0 {
		t.Errorf("token snapshot = %+v, want all zero (non-assistant/malformed lines never bill)", snap)
	}
}
