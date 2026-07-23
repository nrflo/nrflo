package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// newLedgerCodexTestProc builds a processInfo suitable for driving
// codexLedgerEmitter, plus a t.Cleanup that drops its ledger from the
// process-global store (codexLedgerEmitter always writes through
// globalLedgerStore, so tests cannot inject a local store).
func newLedgerCodexTestProc(t *testing.T, sessionID string, maxContext int) *processInfo {
	t.Helper()
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
	return &processInfo{sessionID: sessionID, projectID: "p1", workflowInstanceID: "wfi-1", maxContext: maxContext}
}

// TestCodexLedgerEmitter_ToolInvokeAndFileReadResult verifies a
// EventToolInvoke naming a read tool with a path input, followed by its
// EventToolResult, produces a tool_use entry then a file_read entry — both
// marked Approx=true — with Source set to the path hint.
func TestCodexLedgerEmitter_ToolInvokeAndFileReadResult(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-fileread"
	proc := newLedgerCodexTestProc(t, sessionID, 100000)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventToolInvoke, ToolName: "Read", ToolInput: map[string]any{"file_path": "/repo/a.go"}})
	emit(EngineEvent{Type: EventToolResult, ToolName: "Read", Text: "package spawner"})

	snap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok {
		t.Fatalf("no ledger snapshot for %q", sessionID)
	}
	wantKinds := []LedgerKind{LedgerKindToolUse, LedgerKindFileRead}
	if len(snap.Entries) != len(wantKinds) {
		t.Fatalf("Entries = %d, want %d; entries=%+v", len(snap.Entries), len(wantKinds), snap.Entries)
	}
	for i, k := range wantKinds {
		if snap.Entries[i].Kind != k {
			t.Errorf("Entries[%d].Kind = %q, want %q", i, snap.Entries[i].Kind, k)
		}
		if snap.Entries[i].Approx {
			t.Errorf("Entries[%d].Approx = true, want false (codex is EXACT)", i)
		}
		if snap.Entries[i].Source != "/repo/a.go" {
			t.Errorf("Entries[%d].Source = %q, want /repo/a.go", i, snap.Entries[i].Source)
		}
	}
}

// TestCodexLedgerEmitter_NonReadToolResultClassifiesAsToolResult verifies a
// tool invoke/result pair for a non-read tool produces a generic tool_result
// entry, not file_read, even though it carries a path-ish input.
func TestCodexLedgerEmitter_NonReadToolResultClassifiesAsToolResult(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-toolresult"
	proc := newLedgerCodexTestProc(t, sessionID, 100000)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventToolInvoke, ToolName: "bash", ToolInput: map[string]any{"path": "/repo"}})
	emit(EngineEvent{Type: EventToolResult, ToolName: "bash", Text: "ok"})

	snap, _ := globalLedgerStore.snapshot(sessionID)
	if len(snap.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2; entries=%+v", len(snap.Entries), snap.Entries)
	}
	if snap.Entries[1].Kind != LedgerKindToolResult {
		t.Errorf("Entries[1].Kind = %q, want tool_result", snap.Entries[1].Kind)
	}
}

// TestCodexLedgerEmitter_TextAndThinkingBecomeDialog verifies EventText and
// completed EventThinking append dialog entries, an empty Text is a no-op, and
// a streaming thinking delta (EventThinking with ItemID set) is skipped so
// reasoning is not double-counted against its completed block.
func TestCodexLedgerEmitter_TextAndThinkingBecomeDialog(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-dialog"
	proc := newLedgerCodexTestProc(t, sessionID, 100000)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventText, Text: "hello"})
	emit(EngineEvent{Type: EventThinking, ItemID: "r1", Text: "pond"}) // delta, skipped
	emit(EngineEvent{Type: EventThinking, Text: "pondering"})          // completed
	emit(EngineEvent{Type: EventText, Text: ""})                       // no-op

	snap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok || len(snap.Entries) != 2 {
		t.Fatalf("snapshot=%+v ok=%v, want 2 entries (empty text ignored)", snap, ok)
	}
	for i, e := range snap.Entries {
		if e.Kind != LedgerKindDialog {
			t.Errorf("Entries[%d].Kind = %q, want dialog", i, e.Kind)
		}
	}
}

// TestCodexLedgerEmitter_TurnCompletedAdvancesTurn verifies EventTurnCompleted
// bumps the ledger's turn counter without appending an entry: an entry born
// before the turn boundary keeps its earlier BornTurn, and one appended after
// gets the bumped turn.
func TestCodexLedgerEmitter_TurnCompletedAdvancesTurn(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-turn"
	proc := newLedgerCodexTestProc(t, sessionID, 100000)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventText, Text: "turn zero"})
	emit(EngineEvent{Type: EventTurnCompleted})
	emit(EngineEvent{Type: EventText, Text: "turn one"})

	snap, _ := globalLedgerStore.snapshot(sessionID)
	if len(snap.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(snap.Entries))
	}
	if snap.Entries[0].BornTurn != 0 {
		t.Errorf("Entries[0].BornTurn = %d, want 0", snap.Entries[0].BornTurn)
	}
	if snap.Entries[1].BornTurn != 1 {
		t.Errorf("Entries[1].BornTurn = %d, want 1 (after EventTurnCompleted)", snap.Entries[1].BornTurn)
	}
}

// TestCodexLedgerEmitter_TokenUsageReconciles verifies EventTokenUsage
// rescales the ledger's estimate to exactly ev.Usage.InputTokens, independent
// of proc.maxContext, and that a nil Usage or a non-positive InputTokens both
// leave the estimate untouched (the reconcile basis is codex's own exact
// usage report, not a derived context-window percentage).
func TestCodexLedgerEmitter_TokenUsageReconciles(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-usage"
	proc := newLedgerCodexTestProc(t, sessionID, 1000)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventText, Text: "some text to estimate"})
	emit(EngineEvent{Type: EventTokenUsage, Usage: &EngineUsage{InputTokens: 100}})

	snap, _ := globalLedgerStore.snapshot(sessionID)
	if len(snap.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(snap.Entries))
	}
	if got := snap.Entries[0].TokensEst; got != 100 {
		t.Errorf("TokensEst after reconcile = %d, want 100 (exact, ignoring proc.maxContext=1000)", got)
	}

	// Guard: a nil Usage must not mutate the estimate.
	emit(EngineEvent{Type: EventTokenUsage, Usage: nil})
	if got := globalLedgerStore.get(sessionID).snapshot(sessionID).Entries[0].TokensEst; got != 100 {
		t.Errorf("TokensEst mutated by nil Usage: got %d, want 100", got)
	}

	// Guard: Usage.InputTokens == 0 must not mutate the estimate either.
	emit(EngineEvent{Type: EventTokenUsage, Usage: &EngineUsage{InputTokens: 0}})
	if got := globalLedgerStore.get(sessionID).snapshot(sessionID).Entries[0].TokensEst; got != 100 {
		t.Errorf("TokensEst mutated by Usage.InputTokens=0: got %d, want 100", got)
	}
}

// TestCodexLedgerEmitter_UnknownEventTypeIsNoOp verifies an event type the
// switch doesn't recognize (e.g. EventApprovalRequest) never appends an entry
// or broadcasts.
func TestCodexLedgerEmitter_UnknownEventTypeIsNoOp(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-codex-unknown"
	proc := newLedgerCodexTestProc(t, sessionID, 100000)
	emit := s.codexLedgerEmitter(proc)

	emit(EngineEvent{Type: EventApprovalRequest})

	if snap, ok := globalLedgerStore.snapshot(sessionID); ok && len(snap.Entries) != 0 {
		t.Errorf("snapshot=%+v ok=%v, want empty ledger for an unhandled event type", snap, ok)
	}
}
