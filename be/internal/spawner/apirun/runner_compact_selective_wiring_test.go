package apirun

import (
	"context"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// TestRunTurns_Watcher_PreferredOverFallbackCompact asserts that when
// Config.Watcher offers a plan, runTurns applies it instead of the uniform
// maybeCompactInLoop fallback — even though the reported context-left would
// otherwise trigger the fallback.
func TestRunTurns_Watcher_PreferredOverFallbackCompact(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "probe", output: "ok"}
	lowUsage := provider.Usage{InputTokens: 900} // 1000-window -> 10% left, below CompactPct
	prov := newRecordingProvider(
		toolTurn("tu_1", "probe", lowUsage),
		toolTurn("tu_2", "probe", lowUsage),
		endTurn("SELECTIVE-DIGEST", provider.Usage{}), // consumed by applyCompactionPlan's summarize
		endTurn("done", provider.Usage{}),
	)
	watcher := &fakeWatcher{ok: true, plan: CompactionPlan{KeepPrefixMsgs: 1, KeepSuffixMsgs: 1, PolicyName: "selective"}}

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"probe": handler},
		InitialPrompt: "TASK",
		MaxIterations: 5,
		MaxContext:    1000,
		Watcher:       watcher,
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	if len(watcher.states) == 0 {
		t.Fatalf("watcher was never consulted")
	}
	foundSelective := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "selective") {
			foundSelective = true
		}
	}
	if !foundSelective {
		t.Errorf("expected a selective-compaction system row, got %+v", sink.Calls())
	}
}

// TestRunTurns_Watcher_Declines_FallsBackToUniformCompact asserts that when
// Config.Watcher declines (ok=false), the loop still falls back to
// maybeCompactInLoop — the watcher seam must never silently disable
// compaction altogether.
func TestRunTurns_Watcher_Declines_FallsBackToUniformCompact(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "probe", output: "ok"}
	lowUsage := provider.Usage{InputTokens: 900}
	prov := newRecordingProvider(
		toolTurn("tu_1", "probe", lowUsage),
		toolTurn("tu_2", "probe", lowUsage),
		endTurn("FALLBACK-SUMMARY", provider.Usage{}),
		endTurn("done", provider.Usage{}),
	)
	watcher := &fakeWatcher{ok: false}

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"probe": handler},
		InitialPrompt: "TASK",
		MaxIterations: 5,
		MaxContext:    1000,
		Watcher:       watcher,
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	if len(watcher.states) == 0 {
		t.Fatalf("watcher was never consulted")
	}
	foundFallback := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "conversation compacted") {
			foundFallback = true
		}
	}
	if !foundFallback {
		t.Errorf("expected the uniform fallback's 'conversation compacted' row, got %+v", sink.Calls())
	}
}

// TestConversation_SendTurn_WatcherGC_ConsultedBeforeEachTurn asserts
// Conversation.maybeWatcherGC consults Config.Watcher at the top of every
// SendTurn (the idle-gap cache-free rewrite point) in addition to runTurns'
// own mid-loop checkpoint — two consults per single-round SendTurn, so a
// declining watcher (ok=false) is consulted exactly twice per turn.
func TestConversation_SendTurn_WatcherGC_ConsultedBeforeEachTurn(t *testing.T) {
	sink := &recordingSink{}
	prov := newRecordingProvider(
		endTurn("reply one", provider.Usage{}),
		endTurn("reply two", provider.Usage{}),
	)
	watcher := &fakeWatcher{ok: false}

	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
		Watcher:       watcher,
	})
	proc := newConvTestProc()

	conv.SendTurn(context.Background(), proc, "turn one")
	conv.SendTurn(context.Background(), proc, "turn two")

	if len(watcher.states) != 4 {
		t.Fatalf("watcher consulted %d times, want 4 (pre-turn + mid-loop, once per SendTurn)", len(watcher.states))
	}
}

// TestConversation_SendTurn_WatcherGC_AppliesSelectivePlan asserts that when
// the watcher offers a plan, SendTurn applies it to the stored history before
// running the turn, and the compacted history (not the raw one) is what the
// provider sees.
func TestConversation_SendTurn_WatcherGC_AppliesSelectivePlan(t *testing.T) {
	sink := &recordingSink{}
	prov := newRecordingProvider(
		endTurn("reply one", provider.Usage{}),
		endTurn("reply two", provider.Usage{}),
		endTurn("SELECTIVE-DIGEST", provider.Usage{}), // consumed by applyCompactionPlan's summarize
		endTurn("reply three", provider.Usage{}),
	)

	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	conv.SendTurn(context.Background(), proc, "turn one")
	conv.SendTurn(context.Background(), proc, "turn two")
	// History is now 4 messages: [u1,a1,u2,a2]. Attach the watcher only now so
	// the first two turns aren't touched by it, then trigger a plan on turn 3.
	// fireLimit=1 mimics a real watcher's throttle: only the pre-turn consult
	// (maybeWatcherGC) fires; the mid-loop consult inside runTurns then
	// declines, so exactly one summarize call is consumed.
	conv.cfg.Watcher = &fakeWatcher{ok: true, fireLimit: 1, plan: CompactionPlan{KeepPrefixMsgs: 1, KeepSuffixMsgs: 1, PolicyName: "selective"}}

	status := conv.SendTurn(context.Background(), proc, "turn three")
	if status != "PASS" {
		t.Fatalf("turn 3 status = %q, want PASS", status)
	}
	if len(prov.requests) != 4 {
		t.Fatalf("provider calls = %d, want 4 (2 turns + selective summarize + turn 3)", len(prov.requests))
	}
	sumReq := prov.requests[2]
	if !strings.Contains(sumReq.System, "summarizer") {
		t.Errorf("summarize System = %q, want the compaction system prompt", sumReq.System)
	}
	// turn 3's request replays the compacted history (prefix + digest) plus
	// the new user turn — fewer messages than the raw 5 (4 history + 1 new).
	turnReq := prov.requests[3]
	if len(turnReq.Messages) >= 5 {
		t.Errorf("post-GC request has %d messages, want fewer than the uncompacted 5", len(turnReq.Messages))
	}
}
