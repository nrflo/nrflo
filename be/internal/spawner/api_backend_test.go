package spawner

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestAPIBackend_Identification verifies the apiBackend identifies as "api"
// and reports no resume / no take-control support.
func TestAPIBackend_Identification(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	b := newAPIBackend(s)
	if got := b.Name(); got != "api" {
		t.Errorf("Name() = %q, want api", got)
	}
	if b.SupportsResume() {
		t.Errorf("SupportsResume() = true, want false")
	}
	if b.SupportsTakeControl() {
		t.Errorf("SupportsTakeControl() = true, want false")
	}
}

// TestAPIBackend_Start_RejectsMissingProvider returns an error when the
// spawner has no Provider configured.
func TestAPIBackend_Start_RejectsMissingProvider(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now()), Provider: nil})
	b := newAPIBackend(s)
	err := b.Start(context.Background(), &processInfo{doneCh: make(chan struct{})}, &prepResult{})
	if err == nil {
		t.Fatalf("Start with nil Provider returned nil error")
	}
}

// TestAPIBackend_Start_RejectsMissingAgentSvc verifies missing AgentSvc fails fast.
func TestAPIBackend_Start_RejectsMissingAgentSvc(t *testing.T) {
	s := New(Config{
		Clock:    clock.NewTest(time.Now()),
		Provider: mock.New(),
	})
	b := newAPIBackend(s)
	err := b.Start(context.Background(), &processInfo{doneCh: make(chan struct{})}, &prepResult{})
	if err == nil {
		t.Fatalf("Start with nil AgentSvc returned nil error")
	}
}

// TestAPIBackend_Kill_NilSafe verifies Kill before Start is a no-op.
func TestAPIBackend_Kill_NilSafe(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	b := newAPIBackend(s)
	if err := b.Kill(context.Background(), &processInfo{}, syscall.SIGTERM); err != nil {
		t.Errorf("Kill(no cancel) = %v, want nil", err)
	}
}

// TestMapFinalStatus_AllCases ensures each finalStatus maps to the correct
// (result, reason) pair persisted to agent_sessions.
func TestMapFinalStatus_AllCases(t *testing.T) {
	cases := []struct {
		in         string
		wantResult string
		wantReason string
	}{
		{in: "PASS", wantResult: "pass", wantReason: "implicit"},
		{in: "CANCELLED", wantResult: "fail", wantReason: "cancelled"},
		{in: "FAIL", wantResult: "fail", wantReason: "api_error"},
		{in: "", wantResult: "fail", wantReason: "no_status"},
		{in: "WEIRD", wantResult: "fail", wantReason: "weird"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gotR, gotReason := mapFinalStatus(tc.in)
			if gotR != tc.wantResult || gotReason != tc.wantReason {
				t.Errorf("mapFinalStatus(%q) = (%q, %q), want (%q, %q)",
					tc.in, gotR, gotReason, tc.wantResult, tc.wantReason)
			}
		})
	}
}

// TestProcStateAdapter verifies the adapter reads and writes the correct
// processInfo fields used by monitorAll.
func TestProcStateAdapter(t *testing.T) {
	proc := &processInfo{
		sessionID:          "s-1",
		projectID:          "p-1",
		workflowInstanceID: "wfi-1",
	}
	a := &procStateAdapter{proc: proc}

	if a.SessionID() != "s-1" {
		t.Errorf("SessionID() = %q, want s-1", a.SessionID())
	}
	if a.ProjectID() != "p-1" {
		t.Errorf("ProjectID() = %q, want p-1", a.ProjectID())
	}
	if a.WorkflowInstanceID() != "wfi-1" {
		t.Errorf("WorkflowInstanceID() = %q, want wfi-1", a.WorkflowInstanceID())
	}

	a.SetFinalStatus("PASS")
	if proc.finalStatus != "PASS" {
		t.Errorf("SetFinalStatus did not propagate; finalStatus = %q", proc.finalStatus)
	}
	a.SetContextLeft(42)
	if proc.contextLeft != 42 {
		t.Errorf("SetContextLeft did not propagate; contextLeft = %d", proc.contextLeft)
	}
}

// TestApirunErrorAdapter_PassesThroughAndNilSafe verifies the adapter wraps a
// non-nil ErrorRecorder and short-circuits to nil when the input is nil.
func TestApirunErrorAdapter_PassesThroughAndNilSafe(t *testing.T) {
	if got := apirunErrorAdapter(nil); got != nil {
		t.Errorf("apirunErrorAdapter(nil) = %v, want nil", got)
	}

	rec := &mockErrRec{}
	adp := apirunErrorAdapter(rec)
	if adp == nil {
		t.Fatalf("apirunErrorAdapter(rec) returned nil")
	}
	if err := adp.RecordError("p", "agent", "i", "msg"); err != nil {
		t.Errorf("RecordError = %v, want nil", err)
	}
	if rec.calls != 1 {
		t.Errorf("rec.calls = %d, want 1", rec.calls)
	}
}

// TestProcMessageSink_DelegatesToTrackMessage verifies the spawner-side sink
// adapter pushes messages into the proc's pendingMessages queue with the
// supplied category, matching what the runner needs for WS broadcast parity.
func TestProcMessageSink_DelegatesToTrackMessage(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	s := New(Config{Clock: clk})
	proc := &processInfo{}
	sink := &procMessageSink{s: s, proc: proc}

	sink.TrackMessage("hello", "text")
	sink.TrackMessage("[tool_use:start] id=t1 name=Bash", "tool_use_start")

	if len(proc.pendingMessages) != 2 {
		t.Fatalf("pendingMessages = %d, want 2", len(proc.pendingMessages))
	}
	if proc.pendingMessages[0].Content != "hello" || proc.pendingMessages[0].Category != "text" {
		t.Errorf("pendingMessages[0] = %+v, want {hello, text}", proc.pendingMessages[0])
	}
	if proc.pendingMessages[1].Category != "tool_use_start" {
		t.Errorf("pendingMessages[1].Category = %q, want tool_use_start", proc.pendingMessages[1].Category)
	}
	if !proc.hasReceivedMessage {
		t.Errorf("hasReceivedMessage = false, want true (stall detection requires this)")
	}
}

type mockErrRec struct {
	mu    sync.Mutex
	calls int
}

func (m *mockErrRec) RecordError(projectID, errorType, instanceID, message string) error {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return nil
}

// TestAPIBackend_FullLoop_NoDB exercises the runner end-to-end via the
// apiBackend without any DB / session row. The backend goroutine attempts
// saveMessages + registerAgentStopWithReason after the runner returns; both
// are no-ops when proc has no DB-resolvable session, so the test only checks
// that finalStatus is set, doneCh closes, and Kill cancels promptly.
func TestAPIBackend_FullLoop_RunsAndKills(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	prov := &blockingMockProvider{started: make(chan struct{})}
	s := New(Config{
		Clock:    clk,
		Provider: prov,
		AgentSvc: &noopAgentSvc{},
	})
	b := newAPIBackend(s)

	proc := &processInfo{
		sessionID:    "sess-1",
		projectID:    "p1",
		ticketID:     "t1",
		workflowName: "wf",
		agentID:      "ag-1",
		agentType:    "test-agent",
		modelID:      "claude:sonnet",
		startTime:    clk.Now(),
		doneCh:       make(chan struct{}),
		maxContext:   1000,
	}
	prep := &prepResult{
		executionMode:    "api",
		apiInitialPrompt: "hi",
		apiMaxIterations: 1,
		apiMaxContext:    1000,
		apiDeadline:      time.Now().Add(5 * time.Second),
	}

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-prov.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("provider Run() not called")
	}

	if err := b.Kill(context.Background(), proc, syscall.SIGTERM); err != nil {
		t.Errorf("Kill: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("doneCh not closed within 2s after Kill")
	}

	if proc.finalStatus != "CANCELLED" {
		t.Errorf("finalStatus = %q, want CANCELLED", proc.finalStatus)
	}
	if proc.spawnCommand == "" {
		t.Errorf("spawnCommand should be set, got empty")
	}
}

// blockingMockProvider blocks Run() until ctx is cancelled.
type blockingMockProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *blockingMockProvider) Name() string                { return "blocking" }
func (p *blockingMockProvider) MaxContext(model string) int { return 200000 }
func (p *blockingMockProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

// noopAgentSvc satisfies apirun.AgentSvc with no DB writes.
type noopAgentSvc struct{}

func (n *noopAgentSvc) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	return "", "", "", nil
}
