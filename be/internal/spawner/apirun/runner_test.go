package apirun

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// fakeProc satisfies ProcState. SetFinalStatus and SetContextLeft are
// guarded so tests can read them concurrently with the runner goroutine.
type fakeProc struct {
	mu            sync.Mutex
	sessionID     string
	projectID     string
	wfiID         string
	finalStatus   string
	contextLeft   int
	callbackLevel int
}

func (p *fakeProc) SessionID() string          { return p.sessionID }
func (p *fakeProc) ProjectID() string          { return p.projectID }
func (p *fakeProc) WorkflowInstanceID() string { return p.wfiID }
func (p *fakeProc) SetFinalStatus(s string) {
	p.mu.Lock()
	p.finalStatus = s
	p.mu.Unlock()
}
func (p *fakeProc) SetContextLeft(pct int) {
	p.mu.Lock()
	p.contextLeft = pct
	p.mu.Unlock()
}
func (p *fakeProc) SetCallbackLevel(level int) {
	p.mu.Lock()
	p.callbackLevel = level
	p.mu.Unlock()
}
func (p *fakeProc) CallbackLevel() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callbackLevel
}
func (p *fakeProc) FinalStatus() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finalStatus
}
func (p *fakeProc) ContextLeft() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.contextLeft
}

// recordingSink captures TrackMessage calls.
type recordingSink struct {
	mu    sync.Mutex
	calls []recordedMsg
}

type recordedMsg struct {
	content  string
	category string
}

func (r *recordingSink) TrackMessage(content, category string) {
	r.mu.Lock()
	r.calls = append(r.calls, recordedMsg{content: content, category: category})
	r.mu.Unlock()
}

func (r *recordingSink) Calls() []recordedMsg {
	r.mu.Lock()
	out := make([]recordedMsg, len(r.calls))
	copy(out, r.calls)
	r.mu.Unlock()
	return out
}

// recordingAgentSvc captures UpdateContextLeft calls.
type recordingAgentSvc struct {
	mu    sync.Mutex
	calls []agentSvcCall
}

type agentSvcCall struct {
	sessionID string
	pct       int
}

func (a *recordingAgentSvc) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	a.mu.Lock()
	a.calls = append(a.calls, agentSvcCall{sessionID: sessionID, pct: pct})
	a.mu.Unlock()
	return "p1", "T-1", "wf", nil
}

func (a *recordingAgentSvc) Calls() []agentSvcCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]agentSvcCall, len(a.calls))
	copy(out, a.calls)
	return out
}

// recordingErrSvc records errors via ErrorRecorder.
type recordingErrSvc struct {
	mu     sync.Mutex
	errors []errCall
}

type errCall struct {
	projectID, errorType, instanceID, message string
}

func (e *recordingErrSvc) RecordError(projectID, errorType, instanceID, message string) error {
	e.mu.Lock()
	e.errors = append(e.errors, errCall{projectID, errorType, instanceID, message})
	e.mu.Unlock()
	return nil
}

func (e *recordingErrSvc) Errors() []errCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]errCall, len(e.errors))
	copy(out, e.errors)
	return out
}

func newTestProc() *fakeProc {
	return &fakeProc{sessionID: "sess-1", projectID: "p1", wfiID: "wfi-1"}
}

// TestRunner_HappyPath_EndTurn covers the basic success case: text deltas,
// usage event, end_turn final reason, PASS status, context left propagated.
func TestRunner_HappyPath_EndTurn(t *testing.T) {
	sink := &recordingSink{}
	agentSvc := &recordingAgentSvc{}
	prov := mock.New(mock.Script{
		Events: []mock.SinkEvent{
			{Kind: mock.EventText, Text: "hello"},
			{Kind: mock.EventText, Text: " world"},
			{Kind: mock.EventUsage, Usage: provider.Usage{InputTokens: 100, OutputTokens: 5}},
		},
		Final: provider.FinalResponse{
			StopReason: "end_turn",
			Usage:      provider.Usage{InputTokens: 100, OutputTokens: 5},
		},
	})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		AgentSvc:      agentSvc,
		InitialPrompt: "say hi",
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})

	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Errorf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}

	// 100 input / 1000 max = 90% remaining
	if got := proc.ContextLeft(); got != 90 {
		t.Errorf("ContextLeft = %d, want 90", got)
	}

	calls := agentSvc.Calls()
	if len(calls) != 1 || calls[0].sessionID != "sess-1" || calls[0].pct != 90 {
		t.Errorf("AgentSvc calls = %+v, want 1 call sess-1 / 90", calls)
	}

	// Verify text was emitted (any of the recorded calls should contain "hello" or "world")
	allText := ""
	for _, c := range sink.Calls() {
		if c.category == "text" {
			allText += c.content
		}
	}
	if !strings.Contains(allText, "hello") || !strings.Contains(allText, "world") {
		t.Errorf("expected text deltas in sink calls, got %+v", sink.Calls())
	}
}

// TestRunner_FailStopReasons covers max_tokens, stop_sequence, tool_use, and
// unknown stop reasons — all should map to FAIL with a system message.
func TestRunner_FailStopReasons(t *testing.T) {
	cases := []struct {
		name       string
		stopReason string
		wantSubstr string
	}{
		{name: "max_tokens", stopReason: "max_tokens", wantSubstr: "stop_reason=max_tokens"},
		{name: "stop_sequence", stopReason: "stop_sequence", wantSubstr: "stop_reason=stop_sequence"},
		{name: "tool_use", stopReason: "tool_use", wantSubstr: "tool_use"},
		{name: "unknown", stopReason: "weird", wantSubstr: "unexpected stop_reason"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &recordingSink{}
			errSvc := &recordingErrSvc{}
			prov := mock.New(mock.Script{
				Final: provider.FinalResponse{StopReason: tc.stopReason},
			})

			r := NewRunner(Config{
				Provider:      prov,
				Sink:          sink,
				ErrorSvc:      errSvc,
				InitialPrompt: "hi",
				MaxIterations: 5,
				MaxContext:    1000,
				Deadline:      time.Now().Add(5 * time.Second),
			})
			proc := newTestProc()
			r.Run(context.Background(), proc)

			if proc.FinalStatus() != "FAIL" {
				t.Errorf("FinalStatus = %q, want FAIL", proc.FinalStatus())
			}

			// expect exactly one system message with the substring
			found := false
			for _, c := range sink.Calls() {
				if c.category == "system" && strings.Contains(c.content, tc.wantSubstr) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected system message containing %q, got %+v", tc.wantSubstr, sink.Calls())
			}

			// ErrorSvc should record the failure
			if len(errSvc.Errors()) != 1 {
				t.Errorf("ErrorSvc.Errors = %d calls, want 1", len(errSvc.Errors()))
			}
		})
	}
}

// TestRunner_DeadlineExceeded_NoProviderCall verifies the runner FAILs without
// calling the provider when the deadline is in the past.
func TestRunner_DeadlineExceeded_NoProviderCall(t *testing.T) {
	sink := &recordingSink{}
	errSvc := &recordingErrSvc{}
	// scripts will panic if exhausted: New() with no scripts returns "no script for turn 1"
	prov := mock.New() // no script -> any Run() call fails

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		ErrorSvc:      errSvc,
		InitialPrompt: "hi",
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(-1 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "FAIL" {
		t.Errorf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	// No provider call should have happened
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "no script for turn") {
			t.Fatalf("provider was called despite deadline; got %+v", sink.Calls())
		}
	}
	// Should have a system message about deadline
	found := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "deadline exceeded") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected deadline message, got %+v", sink.Calls())
	}
}

// TestRunner_ProviderError covers a scripted error: status=FAIL, system
// message includes the error, ErrorSvc records it.
func TestRunner_ProviderError(t *testing.T) {
	sink := &recordingSink{}
	errSvc := &recordingErrSvc{}
	prov := mock.New(mock.Script{Err: errors.New("boom")})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		ErrorSvc:      errSvc,
		InitialPrompt: "hi",
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "FAIL" {
		t.Errorf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	found := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "boom") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected boom in sink, got %+v", sink.Calls())
	}
	if len(errSvc.Errors()) != 1 {
		t.Errorf("ErrorSvc.Errors = %d, want 1", len(errSvc.Errors()))
	}
}

// TestRunner_Cancellation cancels mid-stream; runner should exit promptly with
// CANCELLED status. The mock script blocks on a chan that the test closes
// after cancelling so the cancel hits between provider Run starts and finish.
func TestRunner_Cancellation(t *testing.T) {
	sink := &recordingSink{}
	prov := &cancelProvider{started: make(chan struct{})}

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		InitialPrompt: "hi",
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, proc)
		close(done)
	}()

	// wait for the provider to start, then cancel
	<-prov.started
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runner did not exit within 2s after cancel")
	}

	if got := proc.FinalStatus(); got != "CANCELLED" {
		t.Errorf("FinalStatus = %q, want CANCELLED", got)
	}
}

// cancelProvider blocks Run() until ctx is cancelled, then returns ctx.Err().
type cancelProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *cancelProvider) Name() string                { return "cancel" }
func (p *cancelProvider) MaxContext(model string) int { return 200000 }
func (p *cancelProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestRunner_MaxIterations_BoundaryEndTurn confirms that with MaxIterations=1
// and one end_turn script, the runner runs exactly one turn and PASSES.
func TestRunner_MaxIterations_BoundaryEndTurn(t *testing.T) {
	sink := &recordingSink{}
	prov := mock.New(mock.Script{
		Final: provider.FinalResponse{StopReason: "end_turn"},
	})
	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		InitialPrompt: "hi",
		MaxIterations: 1,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)
	if proc.FinalStatus() != "PASS" {
		t.Errorf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
}

// TestRunner_NoSinkConfigured ensures the runner fails-soft without a sink.
func TestRunner_NoSinkConfigured(t *testing.T) {
	r := NewRunner(Config{
		Provider:      mock.New(),
		Sink:          nil,
		InitialPrompt: "hi",
		MaxIterations: 1,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)
	if proc.FinalStatus() != "FAIL" {
		t.Errorf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
}

// TestRunner_NoProviderConfigured ensures the runner fails when Provider is nil.
func TestRunner_NoProviderConfigured(t *testing.T) {
	sink := &recordingSink{}
	r := NewRunner(Config{
		Provider:      nil,
		Sink:          sink,
		InitialPrompt: "hi",
		MaxIterations: 1,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)
	if proc.FinalStatus() != "FAIL" {
		t.Errorf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	found := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "provider") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected provider not configured message, got %+v", sink.Calls())
	}
}

// TestRunner_ContextLeft_ZeroOnOverflow verifies pct floors at 0 when total
// tokens exceed MaxContext.
func TestRunner_ContextLeft_ZeroOnOverflow(t *testing.T) {
	sink := &recordingSink{}
	agentSvc := &recordingAgentSvc{}
	prov := mock.New(mock.Script{
		Final: provider.FinalResponse{
			StopReason: "end_turn",
			Usage:      provider.Usage{InputTokens: 5000},
		},
	})
	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		AgentSvc:      agentSvc,
		InitialPrompt: "hi",
		MaxIterations: 1,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)
	if got := proc.ContextLeft(); got != 0 {
		t.Errorf("ContextLeft = %d, want 0 (floor)", got)
	}
}
