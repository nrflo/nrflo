package apirun

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// recordingSink captures TrackMessage/TrackToolInvoke/CloseToolSpan calls.
// Shared by the runner/sink tests across this package.
type recordingSink struct {
	mu          sync.Mutex
	calls       []recordedMsg
	closedSpans []string
}

type recordedMsg struct {
	content   string
	category  string
	toolUseID string
}

func (r *recordingSink) TrackMessage(content, category string) {
	r.mu.Lock()
	r.calls = append(r.calls, recordedMsg{content: content, category: category})
	r.mu.Unlock()
}

func (r *recordingSink) TrackToolInvoke(content, category, toolUseID string) {
	r.mu.Lock()
	r.calls = append(r.calls, recordedMsg{content: content, category: category, toolUseID: toolUseID})
	r.mu.Unlock()
}

func (r *recordingSink) CloseToolSpan(toolUseID string) {
	r.mu.Lock()
	r.closedSpans = append(r.closedSpans, toolUseID)
	r.mu.Unlock()
}

func (r *recordingSink) ClosedSpans() []string {
	r.mu.Lock()
	out := make([]string, len(r.closedSpans))
	copy(out, r.closedSpans)
	r.mu.Unlock()
	return out
}

func (r *recordingSink) Calls() []recordedMsg {
	r.mu.Lock()
	out := make([]recordedMsg, len(r.calls))
	copy(out, r.calls)
	r.mu.Unlock()
	return out
}

// TestRunner_ToolUse_ClosesSpan verifies dispatchTools closes the tool span
// after the handler returns, so the trace timeline gets a duration bar.
func TestRunner_ToolUse_ClosesSpan(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "findings_add", output: "ok"}
	prov := mock.New(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{toolUseBlock("tu_span", "findings_add", `{}`)},
		}},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)
	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"findings_add": handler},
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	r.Run(context.Background(), newTestProc())

	closed := sink.ClosedSpans()
	if len(closed) != 1 || closed[0] != "tu_span" {
		t.Errorf("ClosedSpans = %v, want [tu_span]", closed)
	}
}

// TestRunnerSink_ToolUseStopTracksToolUseID verifies the streaming sink emits
// the invoke row through TrackToolInvoke with the tool_use_id attached.
func TestRunnerSink_ToolUseStopTracksToolUseID(t *testing.T) {
	rec := &recordingSink{}
	s := newRunnerSink(rec, false, nil)

	s.OnToolUseStart("tu_9", "Bash")
	s.OnToolUseStop("tu_9", json.RawMessage(`{"command":"ls"}`))

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if calls[0].toolUseID != "tu_9" || calls[0].category != "tool" {
		t.Errorf("call = %+v, want toolUseID tu_9 / category tool", calls[0])
	}
}
