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

// capturingProvider records every Request it receives so tests can inspect
// which content blocks are replayed on subsequent turns.
type capturingProvider struct {
	mu       sync.Mutex
	requests []provider.Request
	scripts  []mock.Script
	cursor   int
}

func (c *capturingProvider) Name() string            { return "capturing" }
func (c *capturingProvider) MaxContext(_ string) int { return 200000 }

func (c *capturingProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	if c.cursor >= len(c.scripts) {
		c.mu.Unlock()
		return &provider.FinalResponse{StopReason: "end_turn"}, nil
	}
	script := c.scripts[c.cursor]
	c.cursor++
	c.mu.Unlock()

	for _, ev := range script.Events {
		switch ev.Kind {
		case mock.EventText:
			sink.OnTextDelta(ev.Text)
		case mock.EventToolUseStart:
			sink.OnToolUseStart(ev.ToolUseID, ev.ToolName)
		case mock.EventToolUseStop:
			sink.OnToolUseStop(ev.ToolUseID, ev.FullInput)
		case mock.EventThinking:
			sink.OnThinkingDelta(ev.Text)
		case mock.EventUsage:
			sink.OnUsage(ev.Usage)
		}
	}
	if script.Err != nil {
		return nil, script.Err
	}
	final := script.Final
	return &final, nil
}

func (c *capturingProvider) Requests() []provider.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]provider.Request, len(c.requests))
	copy(out, c.requests)
	return out
}

// TestRunner_ThinkingReplay_TwoTurn verifies that thinking and redacted_thinking
// blocks from turn-1 are included verbatim in the assistant message replayed to
// turn-2, independent of the captureThinking display gate.
func TestRunner_ThinkingReplay_TwoTurn(t *testing.T) {
	thinkBlock := provider.ContentBlock{
		Type:      "thinking",
		Text:      "my thoughts",
		Signature: "sig-test",
	}
	toolInput := json.RawMessage(`{"x":1}`)
	handler := &recordingHandler{name: "echo", output: "done"}

	prov := &capturingProvider{
		scripts: []mock.Script{
			{
				Events: []mock.SinkEvent{
					{Kind: mock.EventThinking, Text: "my thoughts"},
					{Kind: mock.EventToolUseStart, ToolUseID: "t1", ToolName: "echo"},
					{Kind: mock.EventToolUseStop, ToolUseID: "t1", FullInput: toolInput},
				},
				Final: provider.FinalResponse{
					StopReason: "tool_use",
					Content: []provider.ContentBlock{
						thinkBlock,
						{Type: "tool_use", ToolUseID: "t1", ToolName: "echo", Input: toolInput},
					},
				},
			},
			{
				Final: provider.FinalResponse{StopReason: "end_turn"},
			},
		},
	}

	sink := &recordingSink{}
	r := NewRunner(Config{
		Provider:        prov,
		Sink:            sink,
		InitialPrompt:   "go",
		MaxIterations:   5,
		MaxContext:      200000,
		CaptureThinking: false, // display gate OFF — replay must still happen
		Deadline:        time.Now().Add(5 * time.Second),
		Handlers:        Registry{"echo": handler},
	})

	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS; sink=%+v", proc.FinalStatus(), sink.Calls())
	}

	reqs := prov.Requests()
	if len(reqs) != 2 {
		t.Fatalf("provider received %d requests, want 2", len(reqs))
	}

	// Find the assistant message in turn-2 messages
	var assistantMsg *provider.Message
	for i := range reqs[1].Messages {
		if reqs[1].Messages[i].Role == "assistant" {
			assistantMsg = &reqs[1].Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatalf("no assistant message found in turn-2 messages: %+v", reqs[1].Messages)
	}

	// thinking block must be present verbatim — Text and Signature preserved
	foundThinking := false
	for _, cb := range assistantMsg.Content {
		if cb.Type == "thinking" && cb.Text == "my thoughts" && cb.Signature == "sig-test" {
			foundThinking = true
			break
		}
	}
	if !foundThinking {
		t.Errorf("turn-2 assistant message missing thinking block; content=%+v", assistantMsg.Content)
	}
}

// TestRunner_ThinkingReplay_RedactedBlock verifies that redacted_thinking blocks
// are replayed to turn-2 unchanged even when the display gate is off.
func TestRunner_ThinkingReplay_RedactedBlock(t *testing.T) {
	toolInput := json.RawMessage(`{}`)
	handler := &recordingHandler{name: "echo", output: "ok"}

	prov := &capturingProvider{
		scripts: []mock.Script{
			{
				Events: []mock.SinkEvent{
					{Kind: mock.EventToolUseStart, ToolUseID: "t1", ToolName: "echo"},
					{Kind: mock.EventToolUseStop, ToolUseID: "t1", FullInput: toolInput},
				},
				Final: provider.FinalResponse{
					StopReason: "tool_use",
					Content: []provider.ContentBlock{
						{Type: "redacted_thinking", Data: "secret-blob"},
						{Type: "tool_use", ToolUseID: "t1", ToolName: "echo", Input: toolInput},
					},
				},
			},
			{
				Final: provider.FinalResponse{StopReason: "end_turn"},
			},
		},
	}

	sink := &recordingSink{}
	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    200000,
		Deadline:      time.Now().Add(5 * time.Second),
		Handlers:      Registry{"echo": handler},
	})

	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}

	reqs := prov.Requests()
	if len(reqs) != 2 {
		t.Fatalf("provider received %d requests, want 2", len(reqs))
	}

	var assistantMsg *provider.Message
	for i := range reqs[1].Messages {
		if reqs[1].Messages[i].Role == "assistant" {
			assistantMsg = &reqs[1].Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatalf("no assistant message in turn-2: %+v", reqs[1].Messages)
	}

	foundRedacted := false
	for _, cb := range assistantMsg.Content {
		if cb.Type == "redacted_thinking" && cb.Data == "secret-blob" {
			foundRedacted = true
			break
		}
	}
	if !foundRedacted {
		t.Errorf("turn-2 assistant message missing redacted_thinking block; content=%+v", assistantMsg.Content)
	}
}

// TestMockProvider_ThinkingEvent verifies the mock dispatches EventThinking to
// OnThinkingDelta in the correct position.
func TestMockProvider_ThinkingEvent(t *testing.T) {
	rec := &recordingEventSink{}
	prov := mock.New(mock.Script{
		Events: []mock.SinkEvent{
			{Kind: mock.EventThinking, Text: "deep thought"},
			{Kind: mock.EventText, Text: "answer"},
		},
		Final: provider.FinalResponse{StopReason: "end_turn"},
	})
	if _, err := prov.Run(context.Background(), provider.Request{}, rec); err != nil {
		t.Fatalf("Run: %v", err)
	}

	foundThink, foundText := false, false
	for _, ev := range rec.events {
		if ev == "think:deep thought" {
			foundThink = true
		}
		if ev == "text:answer" {
			foundText = true
		}
	}
	if !foundThink {
		t.Errorf("OnThinkingDelta not fired; events=%v", rec.events)
	}
	if !foundText {
		t.Errorf("OnTextDelta not fired; events=%v", rec.events)
	}
	// think must precede text
	thinkIdx, textIdx := -1, -1
	for i, ev := range rec.events {
		if ev == "think:deep thought" && thinkIdx < 0 {
			thinkIdx = i
		}
		if ev == "text:answer" && textIdx < 0 {
			textIdx = i
		}
	}
	if thinkIdx >= textIdx {
		t.Errorf("think event (%d) must precede text event (%d)", thinkIdx, textIdx)
	}
}

// recordingEventSink captures provider.EventSink callbacks for runner tests.
type recordingEventSink struct {
	events []string
}

func (r *recordingEventSink) OnTextDelta(s string)     { r.events = append(r.events, "text:"+s) }
func (r *recordingEventSink) OnThinkingDelta(s string) { r.events = append(r.events, "think:"+s) }
func (r *recordingEventSink) OnToolUseStart(id, name string) {
	r.events = append(r.events, "tool_start:"+id+":"+name)
}
func (r *recordingEventSink) OnToolUseInputDelta(id, p string)          {}
func (r *recordingEventSink) OnToolUseStop(_ string, _ json.RawMessage) {}
func (r *recordingEventSink) OnUsage(_ provider.Usage)                  {}
