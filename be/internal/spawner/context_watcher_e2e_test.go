package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// e2eRecordingProvider is a local scripted provider.Provider double (the
// e2e test lives in package spawner, so it cannot reuse apirun's unexported
// test-only recordingProvider). It replays FinalResponses in order and
// records every request it receives so the test can inspect how the request
// shrinks after a real selective GC.
type e2eRecordingProvider struct {
	mu       sync.Mutex
	scripts  []provider.FinalResponse
	cursor   int
	requests []provider.Request
}

func (p *e2eRecordingProvider) Name() string          { return "e2e-recording" }
func (p *e2eRecordingProvider) MaxContext(string) int { return 200000 }
func (p *e2eRecordingProvider) Run(_ context.Context, req provider.Request, _ provider.EventSink) (*provider.FinalResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	if p.cursor >= len(p.scripts) {
		return nil, context.DeadlineExceeded
	}
	resp := p.scripts[p.cursor]
	p.cursor++
	return &resp, nil
}

func (p *e2eRecordingProvider) peakMessages() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	peak := 0
	for _, r := range p.requests {
		if len(r.Messages) > peak {
			peak = len(r.Messages)
		}
	}
	return peak
}

func (p *e2eRecordingProvider) lastMessages() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		return 0
	}
	return len(p.requests[len(p.requests)-1].Messages)
}

// e2eProcState is a minimal apirun.ProcState double, shaped like backend.go's
// procStateAdapter but standalone (no *processInfo dependency).
type e2eProcState struct {
	mu          sync.Mutex
	sessionID   string
	finalStatus string
	contextLeft int
}

func (p *e2eProcState) SessionID() string          { return p.sessionID }
func (p *e2eProcState) ProjectID() string          { return "proj" }
func (p *e2eProcState) WorkflowInstanceID() string { return "wfi-1" }
func (p *e2eProcState) SetFinalStatus(s string) {
	p.mu.Lock()
	p.finalStatus = s
	p.mu.Unlock()
}
func (p *e2eProcState) SetContextLeft(pct int) {
	p.mu.Lock()
	p.contextLeft = pct
	p.mu.Unlock()
}
func (p *e2eProcState) SetCallbackLevel(int) {}
func (p *e2eProcState) SetProviderHardFail() {}
func (p *e2eProcState) FinalStatus() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finalStatus
}

// e2eSink is a minimal apirun.MessageSink double, shaped like backend.go's
// procMessageSink, recording every TrackMessage call so the test can assert
// on the selective-compaction system row.
type e2eSink struct {
	mu    sync.Mutex
	calls []string
}

func (s *e2eSink) TrackMessage(content, category string) {
	s.mu.Lock()
	s.calls = append(s.calls, category+":"+content)
	s.mu.Unlock()
}
func (s *e2eSink) TrackToolInvoke(string, string, string, []byte) {}
func (s *e2eSink) CloseToolSpan(string)                           {}
func (s *e2eSink) hasSelectiveRow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		if strings.HasPrefix(c, "system:") && strings.Contains(c, "selective") {
			return true
		}
	}
	return false
}

// e2eProbeHandler is a trivial apirun.ToolHandler double.
type e2eProbeHandler struct{}

func (e2eProbeHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: "probe", Description: "test probe", InputSchema: json.RawMessage(`{}`)}
}
func (e2eProbeHandler) Invoke(context.Context, apirun.ToolEnv, json.RawMessage) (string, bool, error) {
	return "ok", false, nil
}

// e2eToolTurn builds a scripted tool_use turn carrying both a dialog (text)
// block and a tool_use block, so the ledger gets an immediately GC-eligible
// dialog entry every turn (tool_result entries are only eligible once stale
// by context_decay_turns, which this short-lived test never reaches).
func e2eToolTurn(id, dialogText string) provider.FinalResponse {
	return provider.FinalResponse{
		StopReason: "tool_use",
		Content: []provider.ContentBlock{
			{Type: "text", Text: dialogText},
			{Type: "tool_use", ToolUseID: id, ToolName: "probe", Input: json.RawMessage(`{}`)},
		},
	}
}

// e2eSmallTurns is the number of near-zero-token tool turns run before the
// burst turn — kept deliberately small (estTokens truncates len/4, and a
// 1-byte text rounds to 0) so cumulative ledger tokens stay comfortably
// under e2eBudgetTokens until the burst turn lands.
const e2eSmallTurns = 6

// e2eBudgetTokens sits strictly between the small-turns' cumulative total
// (~2 from the initial prompt, since the 1-byte per-turn text rounds to 0
// tokens) and the total once the burst turn's ~500-token dialog block
// lands — so PlanGC declines through the small turns and fires exactly once
// the burst is observed.
const e2eBudgetTokens = 100

// TestContextWatcher_SeededBudget_DerivesFromFraction is the "watcher picks
// up seeded budget" acceptance case: seed context_budget_fraction, derive
// the default against a known maxContext, and confirm the constructed
// watcher's budgetTokens matches round(fraction*maxContext) exactly.
func TestContextWatcher_SeededBudget_DerivesFromFraction(t *testing.T) {
	pool := setupTestDB(t)
	if err := pool.SetConfig("context_budget_fraction", "0.65"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	const maxContext = 200000
	def := deriveContextBudgetDefault(pool, maxContext)

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newAPIContextWatcher(pool, clk, "sess-seeded-budget", "claude-x", resolveContextBudget(nil, def))

	want := 130000 // round(0.65*200000)
	if w.budgetTokens <= 0 {
		t.Fatalf("budgetTokens = %d, want > 0", w.budgetTokens)
	}
	if w.budgetTokens != want {
		t.Errorf("budgetTokens = %d, want %d (round(0.65*200000))", w.budgetTokens, want)
	}
}

// TestContextWatcher_E2E_RealRunnerShrinksProviderRequest is the full
// acceptance test: a real apirun.Runner, driven by a real apiContextWatcher
// (over an isolated ledgerStore/apiLedgerObserver pair, not the process
// global) and a real ledgerStore-backed budget, against a scripted
// recording provider that never leaves the process. It asserts the run
// PASSes and that a later provider request carries strictly fewer messages
// than the pre-GC peak — the previously-unproven end-to-end shrink.
func TestContextWatcher_E2E_RealRunnerShrinksProviderRequest(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newLedgerStore(clk)
	sessionID := "sess-e2e-gc"

	w := newAPIContextWatcher(nil, clk, sessionID, "claude-x", e2eBudgetTokens)
	w.store = store
	w.minInterval = 1 // no throttle: isolate the assert to the budget/message-count gates

	observer := &apiLedgerObserver{store: store, sessionID: sessionID} // broadcast nil-safe

	// e2eSmallTurns near-zero-token turns keep the ledger under budget while
	// building up message count, then one burst turn (large dialog block)
	// pushes cumulative tokens far over e2eBudgetTokens in a single step —
	// PlanGC's target then comfortably exceeds every dialog entry seen so
	// far, so it evicts several of them at once instead of just enough to
	// creep back under budget (which would evict too few messages to shrink
	// the request net of the replacement digest message).
	scripts := make([]provider.FinalResponse, 0, e2eSmallTurns+3)
	for i := 0; i < e2eSmallTurns; i++ {
		scripts = append(scripts, e2eToolTurn(fmt.Sprintf("tu_small_%d", i), "x"))
	}
	scripts = append(scripts, e2eToolTurn("tu_burst", strings.Repeat("x", 2000)))
	// Consumed by applyCompactionPlan's summarize call once the watcher fires.
	scripts = append(scripts, provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: "SELECTIVE-DIGEST"}},
	})
	// The real post-GC turn, wrapping up the run.
	scripts = append(scripts, provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: "done"}},
	})
	prov := &e2eRecordingProvider{scripts: scripts}
	sink := &e2eSink{}

	r := apirun.NewRunner(apirun.Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      apirun.Registry{"probe": e2eProbeHandler{}},
		InitialPrompt: "E2E-TASK",
		MaxIterations: e2eSmallTurns + 5,
		MaxContext:    200000,
		Observer:      observer,
		Watcher:       w,
	})
	proc := &e2eProcState{sessionID: sessionID}
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}

	peak := prov.peakMessages()
	last := prov.lastMessages()
	if peak == 0 {
		t.Fatalf("provider never received a request")
	}
	if last >= peak {
		t.Errorf("last request messages = %d, peak pre-GC messages = %d; want last strictly fewer (selective GC should have shrunk the request)", last, peak)
	}
	if !sink.hasSelectiveRow() {
		t.Errorf("expected a selective-compaction system row, got %+v", sink.calls)
	}
}
