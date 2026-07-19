package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestClaudeEngine_FlushTranscript_AssistantUsageFeedsSessionCost verifies
// processTranscriptLine's usage feed (console_engine_claude_transcript.go)
// pushes an assistant entry's message.usage into the session's running cost
// via the real flushTranscript path, using the exact Claude CLI usage key
// names (input_tokens/cache_read_input_tokens/cache_creation_input_tokens/
// output_tokens).
func TestClaudeEngine_FlushTranscript_AssistantUsageFeedsSessionCost(t *testing.T) {
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-tail-cost", "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop("sess-tail-cost") })
	RegisterSessionCost("sess-tail-cost", "sonnet-5", pool, clk, nil)

	cfg, workDir, sid := t.TempDir(), "/work/console-cost", "sess-tail-cost"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)

	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],` +
		`"usage":{"input_tokens":6000,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"output_tokens":1500}}}` + "\n"
	writeRawTranscript(t, path, line)

	e.flushTranscript()

	snap, ok := SessionCost("sess-tail-cost")
	if !ok {
		t.Fatal("SessionCost ok = false after flushTranscript with usage")
	}
	if snap.InputTokens != 6_000 || snap.OutputTokens != 1_500 || snap.CacheReadTokens != 1_000 || snap.CacheWriteTokens != 200 {
		t.Errorf("token snapshot = %+v, want in:6000 out:1500 cacheRd:1000 cacheWr:200", snap)
	}
	// sonnet-5: price_in=3, price_out=15, cache_write=3.75, cache_read=0.3 per MTok.
	want := 6_000.0/1e6*3 + 1_500.0/1e6*15 + 200.0/1e6*3.75 + 1_000.0/1e6*0.3
	if diff := snap.CostUSD - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("CostUSD = %v, want %v", snap.CostUSD, want)
	}
}

// TestClaudeEngine_FlushTranscript_NoUsageDoesNotTouchSessionCost verifies an
// assistant entry with no usage field leaves the session cost untouched.
func TestClaudeEngine_FlushTranscript_NoUsageDoesNotTouchSessionCost(t *testing.T) {
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-tail-no-usage", "sonnet-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop("sess-tail-no-usage") })
	RegisterSessionCost("sess-tail-no-usage", "sonnet-5", pool, clk, nil)

	cfg, workDir, sid := t.TempDir(), "/work/console-no-usage", "sess-tail-no-usage"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)
	writeRawTranscript(t, path, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`+"\n")

	e.flushTranscript()

	snap, ok := SessionCost("sess-tail-no-usage")
	if !ok {
		t.Fatal("SessionCost ok = false")
	}
	if snap.InputTokens != 0 || snap.OutputTokens != 0 || snap.CostUSD != 0 {
		t.Errorf("snapshot = %+v, want all zero (no usage field)", snap)
	}
}
