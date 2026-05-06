package spawner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// opencodeTestSink records Sink method invocations for assertions.
type opencodeTestSink struct {
	mu             sync.Mutex
	recordedMsgs   []recordedMsg
	bumpCount      int
	turnCompletes  int
	contextUpdates []int
	errors         []string
}

type recordedMsg struct {
	content  string
	category string
}

func (s *opencodeTestSink) RecordHookMessage(sessionID, content, category, payload string) (string, string, string, error) {
	s.mu.Lock()
	s.recordedMsgs = append(s.recordedMsgs, recordedMsg{content, category})
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}
func (s *opencodeTestSink) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	s.mu.Lock()
	s.contextUpdates = append(s.contextUpdates, pct)
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}
func (s *opencodeTestSink) BumpLastMessage(sessionID string) {
	s.mu.Lock()
	s.bumpCount++
	s.mu.Unlock()
}
func (s *opencodeTestSink) SetLastMessage(sessionID, content string) {}
func (s *opencodeTestSink) OnTurnComplete(sessionID string) {
	s.mu.Lock()
	s.turnCompletes++
	s.mu.Unlock()
}
func (s *opencodeTestSink) BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID string) {}
func (s *opencodeTestSink) RecordError(projectID, errType, sessionID, msg string) {
	s.mu.Lock()
	s.errors = append(s.errors, msg)
	s.mu.Unlock()
}

// =============================================================================
// dispatchSSEEvent: unit tests per event type
// =============================================================================

func TestDispatchSSEEvent_ToolRunning_CallsRecordHookMessage(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := newSSEState()
	payload := `{
		"type": "message.part.updated",
		"properties": {
			"part": {
				"type": "tool", "id": "p1", "messageID": "m1",
				"tool": "bash",
				"state": {"status": "running", "input": {"command": "ls -la"}}
			}
		}
	}`
	dispatchSSEEvent(context.Background(), payload, "sess-1", sink, state)

	sink.mu.Lock()
	msgs := append([]recordedMsg{}, sink.recordedMsgs...)
	bumps := sink.bumpCount
	sink.mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("RecordHookMessage not called for tool running event")
	}
	if !strings.Contains(msgs[0].content, "ls -la") {
		t.Errorf("RecordHookMessage content = %q, want to contain 'ls -la'", msgs[0].content)
	}
	if bumps < 1 {
		t.Errorf("BumpLastMessage not called for tool running event")
	}
}

func TestDispatchSSEEvent_ToolCompleted_CallsRecordHookMessage(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := newSSEState()
	payload := `{
		"type": "message.part.updated",
		"properties": {
			"part": {
				"type": "tool", "id": "p1", "messageID": "m1",
				"tool": "read",
				"state": {"status": "completed", "output": "file content here"}
			}
		}
	}`
	dispatchSSEEvent(context.Background(), payload, "sess-1", sink, state)

	sink.mu.Lock()
	msgs := append([]recordedMsg{}, sink.recordedMsgs...)
	sink.mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("RecordHookMessage not called for tool completed event")
	}
	if !strings.Contains(msgs[0].content, "result") {
		t.Errorf("tool completed content = %q, want to contain 'result'", msgs[0].content)
	}
	if !strings.Contains(msgs[0].content, "file content here") {
		t.Errorf("tool completed content = %q, want to contain output text", msgs[0].content)
	}
}

func TestDispatchSSEEvent_PartDelta_AccumulatesInBuffer(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := newSSEState()

	dispatch := func(delta string) {
		payload := fmt.Sprintf(`{
			"type": "message.part.delta",
			"properties": {"field": "text", "partID": "part-abc", "delta": %q}
		}`, delta)
		dispatchSSEEvent(context.Background(), payload, "sess-1", sink, state)
	}

	dispatch("Hello ")
	state.mu.Lock()
	got := state.textBuf["part-abc"]
	state.mu.Unlock()
	if got != "Hello " {
		t.Errorf("after first delta, textBuf = %q, want %q", got, "Hello ")
	}

	dispatch("world")
	state.mu.Lock()
	got = state.textBuf["part-abc"]
	state.mu.Unlock()
	if got != "Hello world" {
		t.Errorf("after second delta, textBuf = %q, want %q", got, "Hello world")
	}
}

func TestDispatchSSEEvent_MessageUpdated_FlushesTextAndUpdatesContext(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := newSSEState()

	// Pre-populate state: part "p1" maps to message "m1" with buffered text.
	state.mu.Lock()
	state.textBuf["p1"] = "Buffered text content"
	state.partMsg["p1"] = "m1"
	state.msgParts["m1"] = []string{"p1"}
	state.mu.Unlock()

	// tokens.total=1000 → ComputeContextLeftPct(1000, 0) = 100-(1000*100/200000) = 99
	expectedPct := ComputeContextLeftPct(1000, 0)

	payload := `{
		"type": "message.updated",
		"properties": {
			"info": {"id": "m1", "tokens": {"total": 1000}}
		}
	}`
	dispatchSSEEvent(context.Background(), payload, "sess-1", sink, state)

	sink.mu.Lock()
	msgs := append([]recordedMsg{}, sink.recordedMsgs...)
	updates := append([]int{}, sink.contextUpdates...)
	sink.mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("RecordHookMessage not called on message.updated text flush")
	}
	if msgs[0].content != "Buffered text content" {
		t.Errorf("flushed content = %q, want %q", msgs[0].content, "Buffered text content")
	}
	if msgs[0].category != "text" {
		t.Errorf("flushed category = %q, want %q", msgs[0].category, "text")
	}
	if len(updates) == 0 {
		t.Fatal("UpdateContextLeft not called on message.updated")
	}
	if updates[0] != expectedPct {
		t.Errorf("UpdateContextLeft pct = %d, want %d", updates[0], expectedPct)
	}
}

func TestDispatchSSEEvent_SessionIdle_BumpsAndCompletesTheTurn(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := newSSEState()
	payload := `{"type": "session.idle", "properties": {}}`
	dispatchSSEEvent(context.Background(), payload, "sess-1", sink, state)

	sink.mu.Lock()
	bumps, turns := sink.bumpCount, sink.turnCompletes
	sink.mu.Unlock()

	if bumps < 1 {
		t.Errorf("BumpLastMessage not called for session.idle")
	}
	if turns < 1 {
		t.Errorf("OnTurnComplete not called for session.idle")
	}
}

func TestDispatchSSEEvent_SessionError_RecordsMessageAndError(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := newSSEState()
	payload := `{
		"type": "session.error",
		"properties": {
			"error": {"name": "AuthError", "data": {"message": "token expired"}}
		}
	}`
	dispatchSSEEvent(context.Background(), payload, "sess-1", sink, state)

	sink.mu.Lock()
	msgs := append([]recordedMsg{}, sink.recordedMsgs...)
	errs := append([]string{}, sink.errors...)
	sink.mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("RecordHookMessage not called for session.error")
	}
	if !strings.Contains(msgs[0].content, "AuthError") {
		t.Errorf("error message = %q, want to contain 'AuthError'", msgs[0].content)
	}
	if len(errs) == 0 {
		t.Error("RecordError not called for session.error")
	}
}

// =============================================================================
// opencodeSSELoop: context cancel exits goroutine
// =============================================================================

func TestOpencodeSSELoop_CtxCancel_Exits(t *testing.T) {
	t.Parallel()
	// Server blocks until client disconnects.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	port, _ := strconv.Atoi(addr[strings.LastIndex(addr, ":")+1:])

	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	go func() {
		opencodeSSELoop(ctx, port, "sess-cancel", "/work", &opencodeTestSink{})
		close(exited)
	}()

	cancel()

	select {
	case <-exited:
	case <-time.After(500 * time.Millisecond):
		t.Error("opencodeSSELoop did not exit within 500ms after context cancel")
	}
}

// =============================================================================
// opencodeSSELoop: reconnects after clean EOF
// =============================================================================

func TestOpencodeSSELoop_ReconnectsAfterEOF(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	reqCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := reqCount
		reqCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Each connection sends a session.idle event.
		fmt.Fprintf(w, "data: {\"type\":\"session.idle\",\"properties\":{}}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if n == 0 {
			return // first request: close connection (EOF) to trigger reconnect
		}
		<-r.Context().Done() // second request: hold open until test ends
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	port, _ := strconv.Atoi(addr[strings.LastIndex(addr, ":")+1:])

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := &opencodeTestSink{}
	go opencodeSSELoop(ctx, port, "sess-reconnect", "/work", sink)

	// Poll until BumpLastMessage has been called at least twice (one per connection).
	// session.idle → BumpLastMessage + OnTurnComplete; two connections → ≥ 2 bumps.
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			sink.mu.Lock()
			bumps := sink.bumpCount
			sink.mu.Unlock()
			t.Errorf("BumpLastMessage called %d times after 2s, want >= 2 (one per reconnection)", bumps)
			return
		case <-ticker.C:
			sink.mu.Lock()
			bumps := sink.bumpCount
			sink.mu.Unlock()
			if bumps >= 2 {
				return // reconnect confirmed — both events processed
			}
		}
	}
}

// =============================================================================
// Helper function unit tests
// =============================================================================

func TestCapitalize(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"bash", "Bash"},
		{"Bash", "Bash"},
		{"", ""},
		{"r", "R"},
	}
	for _, tc := range cases {
		if got := capitalize(tc.in); got != tc.want {
			t.Errorf("capitalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTruncateStr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
	}
	for _, tc := range cases {
		if got := truncateStr(tc.in, tc.max); got != tc.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
}
