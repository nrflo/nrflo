package apirun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// altMsgs builds n strictly alternating user/assistant messages (user first),
// each with distinguishable text so byte-identity assertions can pinpoint a
// mismatched message.
func altMsgs(n int) []provider.Message {
	out := make([]provider.Message, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		out[i] = provider.Message{Role: role, Content: []provider.ContentBlock{{Type: "text", Text: fmt.Sprintf("m%d", i)}}}
	}
	return out
}

// fakeWatcher returns a fixed plan/ok pair on every PlanGC call, recording
// the WatcherState it was consulted with. When fireLimit > 0, only the first
// fireLimit calls return ok (mimicking a real watcher's throttle); further
// calls decline, so a test can attach a persistently-installed watcher
// without every loop checkpoint re-firing a fresh GC.
type fakeWatcher struct {
	plan      CompactionPlan
	ok        bool
	fireLimit int
	calls     int
	states    []WatcherState
}

func (w *fakeWatcher) PlanGC(state WatcherState) (CompactionPlan, bool) {
	w.states = append(w.states, state)
	w.calls++
	if w.fireLimit > 0 && w.calls > w.fireLimit {
		return CompactionPlan{}, false
	}
	return w.plan, w.ok
}

// assertNoAdjacentSameRole fails the test if any two consecutive messages
// share a Role — the strict alternation the provider APIs require.
func assertNoAdjacentSameRole(t *testing.T, msgs []provider.Message) {
	t.Helper()
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Role == msgs[i-1].Role {
			t.Fatalf("messages[%d] and [%d] both have role %q, want alternation: %+v", i-1, i, msgs[i].Role, msgs)
		}
	}
}

// TestApplyCompactionPlan_PinnedPrefixByteIdentical is the acceptance's
// cache-stability assert: msgs[:KeepPrefixMsgs] must be byte-identical
// (serialized) before and after applyCompactionPlan.
func TestApplyCompactionPlan_PinnedPrefixByteIdentical(t *testing.T) {
	msgs := altMsgs(10)
	wantPrefix := append([]provider.Message{}, msgs[:2]...)
	wantPrefixBytes, err := json.Marshal(wantPrefix)
	if err != nil {
		t.Fatalf("marshal want prefix: %v", err)
	}

	prov := newRecordingProvider(endTurn("DIGEST-SUMMARY", provider.Usage{}))
	cfg := Config{Provider: prov, Sink: &recordingSink{}}
	plan := CompactionPlan{KeepPrefixMsgs: 2, KeepSuffixMsgs: 2, PolicyName: "selective", ReferenceDigest: "[Evicted context — recoverable references:]\n- file.go (tool_result, sha:abc123)\n"}

	out := applyCompactionPlan(context.Background(), cfg, msgs, plan)

	if len(out) < 2 {
		t.Fatalf("output too short: %+v", out)
	}
	gotPrefixBytes, err := json.Marshal(out[:2])
	if err != nil {
		t.Fatalf("marshal got prefix: %v", err)
	}
	if !bytes.Equal(wantPrefixBytes, gotPrefixBytes) {
		t.Errorf("pinned prefix changed:\nwant %s\ngot  %s", wantPrefixBytes, gotPrefixBytes)
	}
}

// TestApplyCompactionPlan_ReplacesMiddleWithSingleDigestMessage asserts the
// evicted range collapses to exactly one message and alternation is
// preserved end-to-end.
func TestApplyCompactionPlan_ReplacesMiddleWithSingleDigestMessage(t *testing.T) {
	msgs := altMsgs(10)
	prov := newRecordingProvider(endTurn("DIGEST-SUMMARY", provider.Usage{}))
	cfg := Config{Provider: prov, Sink: &recordingSink{}}
	plan := CompactionPlan{KeepPrefixMsgs: 2, KeepSuffixMsgs: 2, PolicyName: "selective", TokensEvicted: 123}

	out := applyCompactionPlan(context.Background(), cfg, msgs, plan)

	// Original: 10 messages, keep 2 prefix + 2 suffix requested. The middle
	// range [2,8) has length 6 (even) so the applier grows it by one to keep
	// odd parity → evicted range becomes [2,9), collapsing to 1 digest
	// message, leaving prefix(2) + digest(1) + suffix(1) = 4 messages.
	if len(out) != 4 {
		t.Fatalf("output length = %d, want 4: %+v", len(out), out)
	}
	digest := out[2]
	if digest.Content[0].Type != "text" || !strings.Contains(digest.Content[0].Text, "DIGEST-SUMMARY") {
		t.Errorf("digest message = %+v, want text containing the summarize response", digest)
	}
	assertNoAdjacentSameRole(t, out)
}

// TestApplyCompactionPlan_DigestCarriesReferenceDigest asserts the plan's
// ReferenceDigest text (recoverable references to evicted entries) rides
// along in the spliced digest message.
func TestApplyCompactionPlan_DigestCarriesReferenceDigest(t *testing.T) {
	msgs := altMsgs(10)
	prov := newRecordingProvider(endTurn("SUMMARY-TEXT", provider.Usage{}))
	cfg := Config{Provider: prov, Sink: &recordingSink{}}
	digestRefs := "[Evicted context — recoverable references:]\n- src/foo.go (file_read, sha:deadbeef1234)\n"
	plan := CompactionPlan{KeepPrefixMsgs: 2, KeepSuffixMsgs: 2, PolicyName: "selective", ReferenceDigest: digestRefs}

	out := applyCompactionPlan(context.Background(), cfg, msgs, plan)

	found := false
	for _, m := range out {
		for _, b := range m.Content {
			if strings.Contains(b.Text, "src/foo.go") && strings.Contains(b.Text, "deadbeef1234") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a message carrying the reference digest, got %+v", out)
	}
}

// TestApplyCompactionPlan_OddEvictionRange_NoGrowthNeeded exercises the
// branch where the requested range is already odd-length, so no extra
// message is folded in beyond the plan's suffix.
func TestApplyCompactionPlan_OddEvictionRange_NoGrowthNeeded(t *testing.T) {
	msgs := altMsgs(9) // odd total: [2,6) has length 4... use KeepSuffix=2 -> end=7, range [2,7) length 5 (odd)
	prov := newRecordingProvider(endTurn("SUMMARY", provider.Usage{}))
	cfg := Config{Provider: prov, Sink: &recordingSink{}}
	plan := CompactionPlan{KeepPrefixMsgs: 2, KeepSuffixMsgs: 2, PolicyName: "selective"}

	out := applyCompactionPlan(context.Background(), cfg, msgs, plan)

	// prefix(2) + digest(1) + suffix(2) = 5
	if len(out) != 5 {
		t.Fatalf("output length = %d, want 5: %+v", len(out), out)
	}
	assertNoAdjacentSameRole(t, out)
}

// TestApplyCompactionPlan_DegenerateRange_NoOp: when the plan's keep counts
// leave nothing to evict (start >= end), the original messages are returned
// unchanged and no summarize call is made.
func TestApplyCompactionPlan_DegenerateRange_NoOp(t *testing.T) {
	msgs := altMsgs(4)
	prov := newRecordingProvider() // no scripts queued — a Run call fails the test via mock's underflow
	cfg := Config{Provider: prov, Sink: &recordingSink{}}
	plan := CompactionPlan{KeepPrefixMsgs: 2, KeepSuffixMsgs: 2}

	out := applyCompactionPlan(context.Background(), cfg, msgs, plan)

	if len(out) != len(msgs) {
		t.Fatalf("output length = %d, want %d (no-op)", len(out), len(msgs))
	}
	for i := range msgs {
		if out[i].Content[0].Text != msgs[i].Content[0].Text {
			t.Errorf("message %d mutated: got %+v, want %+v", i, out[i], msgs[i])
		}
	}
	if len(prov.requests) != 0 {
		t.Errorf("provider.Run called %d times, want 0 (no summarize on a no-op range)", len(prov.requests))
	}
}

// TestApplyCompactionPlan_SummarizeFailure_ReturnsOriginal mirrors the
// fallback compactor's non-fatal-failure behavior: a summarize error leaves
// msgs untouched and logs a system row instead of panicking or evicting
// blind.
func TestApplyCompactionPlan_SummarizeFailure_ReturnsOriginal(t *testing.T) {
	msgs := altMsgs(10)
	prov := newRecordingProvider(mock.Script{Err: context.DeadlineExceeded})
	sink := &recordingSink{}
	cfg := Config{Provider: prov, Sink: sink}
	plan := CompactionPlan{KeepPrefixMsgs: 2, KeepSuffixMsgs: 2, PolicyName: "selective"}

	out := applyCompactionPlan(context.Background(), cfg, msgs, plan)

	if len(out) != len(msgs) {
		t.Fatalf("output length = %d, want %d (unchanged on summarize failure)", len(out), len(msgs))
	}
	foundRow := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "selective compaction failed") {
			foundRow = true
		}
	}
	if !foundRow {
		t.Errorf("expected 'selective compaction failed' system row, got %+v", sink.Calls())
	}
}
