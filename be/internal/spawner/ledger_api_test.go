package spawner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// ledgerNoopSink satisfies apirun.MessageSink with no-op recording — the
// ledger observer tests only care about ledger side effects, not the trace
// timeline.
type ledgerNoopSink struct{}

func (ledgerNoopSink) TrackMessage(content, category string)               {}
func (ledgerNoopSink) TrackToolInvoke(content, category, toolUseID string) {}
func (ledgerNoopSink) CloseToolSpan(toolUseID string)                      {}

// ledgerReadHandler is a ToolHandler standing in for a file-read tool (e.g.
// Claude's builtin Read): it just returns a fixed output string.
type ledgerReadHandler struct{ output string }

func (h ledgerReadHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: "Read", InputSchema: json.RawMessage(`{}`)}
}

func (h ledgerReadHandler) Invoke(_ context.Context, _ apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	return h.output, false, nil
}

func apiLedgerToolUseBlock(id, name, input string) provider.ContentBlock {
	return provider.ContentBlock{Type: "tool_use", ToolUseID: id, ToolName: name, Input: json.RawMessage(input)}
}

// newAPILedgerTestProc builds a *processInfo + procStateAdapter pair for
// driving apirun.Runner in tests (mirrors api_backend_test.go's pattern).
func newAPILedgerTestProc(sessionID string) *procStateAdapter {
	return &procStateAdapter{proc: &processInfo{sessionID: sessionID, projectID: "p1", workflowInstanceID: "wfi-1"}}
}

// TestAPILedgerObserver_EntriesMatchAppendedBlocks drives a full apirun.Runner
// conversation (initial prompt -> tool_use Read "a.txt" -> tool_use Read
// "a.txt" again (duplicate path) -> end_turn) through an apiLedgerObserver
// wired as Config.Observer, and asserts the resulting ledger entries mirror
// exactly what was appended: kind sequence, sources, and the duplicate-path
// file_read superseding the first.
func TestAPILedgerObserver_EntriesMatchAppendedBlocks(t *testing.T) {
	store := newLedgerStore(clock.NewTest(time.Now()))
	sessionID := "sess-api-1"
	observer := &apiLedgerObserver{store: store, sessionID: sessionID}

	handler := ledgerReadHandler{output: "contents of a.txt"}
	prov := mock.New(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{apiLedgerToolUseBlock("tu1", "Read", `{"path":"a.txt"}`)},
			Usage:      provider.Usage{InputTokens: 10},
		}},
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{apiLedgerToolUseBlock("tu2", "Read", `{"path":"a.txt","note":"again"}`)},
			Usage:      provider.Usage{InputTokens: 10},
		}},
		mock.Script{Final: provider.FinalResponse{
			StopReason: "end_turn",
			Content:    []provider.ContentBlock{{Type: "text", Text: "done"}},
			Usage:      provider.Usage{InputTokens: 5000},
		}},
	)

	r := apirun.NewRunner(apirun.Config{
		Provider:      prov,
		Sink:          ledgerNoopSink{},
		Handlers:      apirun.Registry{"Read": handler},
		InitialPrompt: "please read a.txt twice",
		MaxIterations: 5,
		MaxContext:    200000,
		Deadline:      time.Now().Add(5 * time.Second),
		Observer:      observer,
	})
	proc := newAPILedgerTestProc(sessionID)
	r.Run(context.Background(), proc)

	if proc.proc.finalStatus != "PASS" {
		t.Fatalf("finalStatus = %q, want PASS", proc.proc.finalStatus)
	}

	snap, ok := store.snapshot(sessionID)
	if !ok {
		t.Fatalf("no ledger snapshot for %q", sessionID)
	}

	wantKinds := []LedgerKind{
		LedgerKindDialog,   // initial prompt
		LedgerKindToolUse,  // tu1 Read(a.txt)
		LedgerKindFileRead, // tu1 result -> superseded below
		LedgerKindToolUse,  // tu2 Read(a.txt) again
		LedgerKindFileRead, // tu2 result -> supersedes the first file_read
		LedgerKindDialog,   // final "done"
	}
	if len(snap.Entries) != len(wantKinds) {
		t.Fatalf("Entries = %d, want %d; entries=%+v", len(snap.Entries), len(wantKinds), snap.Entries)
	}
	for i, k := range wantKinds {
		if snap.Entries[i].Kind != k {
			t.Errorf("Entries[%d].Kind = %q, want %q", i, snap.Entries[i].Kind, k)
		}
	}

	firstFileRead, secondFileRead := snap.Entries[2], snap.Entries[4]
	if firstFileRead.Source != "a.txt" || secondFileRead.Source != "a.txt" {
		t.Errorf("file_read sources = %q, %q, want both a.txt", firstFileRead.Source, secondFileRead.Source)
	}
	if !firstFileRead.Superseded {
		t.Errorf("first file_read Superseded = false, want true (duplicate path re-entry)")
	}
	if secondFileRead.Superseded {
		t.Errorf("second file_read Superseded = true, want false")
	}

	toolUse1, toolUse2 := snap.Entries[1], snap.Entries[3]
	if toolUse1.Source != "a.txt" || toolUse2.Source != "a.txt" {
		t.Errorf("tool_use sources = %q, %q, want both a.txt (path hint)", toolUse1.Source, toolUse2.Source)
	}

	// Grand total (non-superseded) should track real provider usage: the
	// final OnUsage(5000) reconciles every still-live entry to 5000, and only
	// the trailing "done" dialog block (appended after that last reconcile)
	// adds any unreconciled tokens on top — so the total stays within a
	// couple of percent of the full turn-by-turn usage sum (10+10+5000).
	var grandTotal int
	for _, v := range snap.TotalsByKind {
		grandTotal += v
	}
	const wantUsageSum = 10 + 10 + 5000
	diff := grandTotal - wantUsageSum
	if diff < 0 {
		diff = -diff
	}
	if float64(diff) > 0.02*float64(wantUsageSum) {
		t.Errorf("grandTotal = %d, want within 2%% of summed provider usage %d (diff=%d)", grandTotal, wantUsageSum, diff)
	}
}

// TestAPILedgerObserver_OnUsage_NilBroadcastIsSafe verifies a nil broadcast
// func (test construction, per the implementor's guidance) never panics.
func TestAPILedgerObserver_OnUsage_NilBroadcastIsSafe(t *testing.T) {
	store := newLedgerStore(clock.NewTest(time.Now()))
	observer := &apiLedgerObserver{store: store, sessionID: "sess-nil-bcast"}

	observer.OnMessage("user", []provider.ContentBlock{{Type: "text", Text: "hi"}})
	observer.OnUsage(provider.Usage{InputTokens: 100})

	snap, ok := store.snapshot("sess-nil-bcast")
	if !ok || len(snap.Entries) != 1 {
		t.Fatalf("snapshot = %+v, ok=%v; want 1 entry", snap, ok)
	}
}

// TestAPILedgerObserver_OnMessage_EmptyBlocksIgnored verifies an empty block
// slice is a complete no-op — it never even creates the session's ledger.
func TestAPILedgerObserver_OnMessage_EmptyBlocksIgnored(t *testing.T) {
	store := newLedgerStore(clock.NewTest(time.Now()))
	observer := &apiLedgerObserver{store: store, sessionID: "sess-empty"}

	observer.OnMessage("assistant", nil)

	if _, ok := store.snapshot("sess-empty"); ok {
		t.Errorf("snapshot ok = true after empty OnMessage, want false (ledger never created)")
	}
}

// TestAPILedgerObserver_ToolResult_ImageClassifiesAsImage verifies a
// tool_result carrying OutputMedia classifies as LedgerKindImage regardless
// of the invoking tool name.
func TestAPILedgerObserver_ToolResult_ImageClassifiesAsImage(t *testing.T) {
	store := newLedgerStore(clock.NewTest(time.Now()))
	observer := &apiLedgerObserver{store: store, sessionID: "sess-img"}

	observer.OnMessage("assistant", []provider.ContentBlock{apiLedgerToolUseBlock("tu_img", "read_document", `{"name":"scan.pdf"}`)})
	observer.OnMessage("user", []provider.ContentBlock{
		{Type: "tool_result", ToolUseID: "tu_img", OutputMedia: []provider.MediaBlock{{Kind: "document", DataB64: "YWJjZA=="}}},
	})

	snap, _ := store.snapshot("sess-img")
	if len(snap.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(snap.Entries))
	}
	if snap.Entries[1].Kind != LedgerKindImage {
		t.Errorf("Entries[1].Kind = %q, want image", snap.Entries[1].Kind)
	}
}
