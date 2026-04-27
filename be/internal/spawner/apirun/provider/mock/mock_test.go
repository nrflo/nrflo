package mock

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// recordingSink captures all EventSink callbacks in invocation order so tests
// can assert event sequencing.
type recordingSink struct {
	events []string
}

func (s *recordingSink) OnTextDelta(text string) {
	s.events = append(s.events, "text:"+text)
}

func (s *recordingSink) OnToolUseStart(id, name string) {
	s.events = append(s.events, "tool_start:"+id+":"+name)
}

func (s *recordingSink) OnToolUseInputDelta(id, partial string) {
	s.events = append(s.events, "tool_delta:"+id+":"+partial)
}

func (s *recordingSink) OnToolUseStop(id string, full json.RawMessage) {
	s.events = append(s.events, "tool_stop:"+id+":"+string(full))
}

func (s *recordingSink) OnUsage(u provider.Usage) {
	s.events = append(s.events, "usage")
}

func TestMockProvider_NameAndMaxContext(t *testing.T) {
	m := New()
	if got := m.Name(); got != "mock" {
		t.Errorf("Name() = %q, want %q", got, "mock")
	}
	if got := m.MaxContext("anything"); got != 200000 {
		t.Errorf("MaxContext() = %d, want 200000", got)
	}
}

func TestMockProvider_RunReplaysEventsInOrder(t *testing.T) {
	final := provider.FinalResponse{
		StopReason: "end_turn",
		Content: []provider.ContentBlock{
			{Type: "text", Text: "hello world"},
		},
		Usage: provider.Usage{InputTokens: 5, OutputTokens: 3},
	}
	m := New(Script{
		Events: []SinkEvent{
			{Kind: EventText, Text: "hello "},
			{Kind: EventText, Text: "world"},
			{Kind: EventToolUseStart, ToolUseID: "t1", ToolName: "Bash"},
			{Kind: EventToolUseInputDelta, ToolUseID: "t1", PartialJSON: `{"cmd":`},
			{Kind: EventToolUseInputDelta, ToolUseID: "t1", PartialJSON: `"ls"}`},
			{Kind: EventToolUseStop, ToolUseID: "t1", FullInput: json.RawMessage(`{"cmd":"ls"}`)},
			{Kind: EventUsage, Usage: provider.Usage{InputTokens: 5, OutputTokens: 3}},
		},
		Final: final,
	})

	sink := &recordingSink{}
	got, err := m.Run(context.Background(), provider.Request{}, sink)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	want := []string{
		"text:hello ",
		"text:world",
		"tool_start:t1:Bash",
		`tool_delta:t1:{"cmd":`,
		`tool_delta:t1:"ls"}`,
		`tool_stop:t1:{"cmd":"ls"}`,
		"usage",
	}
	if len(sink.events) != len(want) {
		t.Fatalf("event count = %d, want %d (events=%v)", len(sink.events), len(want), sink.events)
	}
	for i, w := range want {
		if sink.events[i] != w {
			t.Errorf("event[%d] = %q, want %q", i, sink.events[i], w)
		}
	}

	if got == nil {
		t.Fatalf("Run() returned nil FinalResponse")
	}
	if got.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", got.StopReason)
	}
	if len(got.Content) != 1 || got.Content[0].Text != "hello world" {
		t.Errorf("Content = %+v, want one text block", got.Content)
	}
	if got.Usage.InputTokens != 5 || got.Usage.OutputTokens != 3 {
		t.Errorf("Usage = %+v, want {Input:5 Output:3}", got.Usage)
	}
}

func TestMockProvider_RunPropagatesError(t *testing.T) {
	want := errors.New("boom")
	m := New(Script{
		Events: []SinkEvent{
			{Kind: EventText, Text: "partial"},
		},
		Err: want,
	})
	sink := &recordingSink{}
	resp, err := m.Run(context.Background(), provider.Request{}, sink)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	if resp != nil {
		t.Errorf("resp = %+v, want nil on error", resp)
	}
	// Events still fire even though final returns an error.
	if len(sink.events) != 1 || sink.events[0] != "text:partial" {
		t.Errorf("events = %v, want [text:partial]", sink.events)
	}
}

func TestMockProvider_MultiTurnAdvancesCursor(t *testing.T) {
	m := New(
		Script{
			Events: []SinkEvent{{Kind: EventText, Text: "turn1"}},
			Final:  provider.FinalResponse{StopReason: "tool_use"},
		},
		Script{
			Events: []SinkEvent{{Kind: EventText, Text: "turn2"}},
			Final:  provider.FinalResponse{StopReason: "end_turn"},
		},
	)

	sink1 := &recordingSink{}
	r1, err := m.Run(context.Background(), provider.Request{}, sink1)
	if err != nil {
		t.Fatalf("Run #1 error: %v", err)
	}
	if r1.StopReason != "tool_use" {
		t.Errorf("turn1 StopReason = %q, want tool_use", r1.StopReason)
	}
	if len(sink1.events) != 1 || sink1.events[0] != "text:turn1" {
		t.Errorf("turn1 events = %v", sink1.events)
	}

	sink2 := &recordingSink{}
	r2, err := m.Run(context.Background(), provider.Request{}, sink2)
	if err != nil {
		t.Fatalf("Run #2 error: %v", err)
	}
	if r2.StopReason != "end_turn" {
		t.Errorf("turn2 StopReason = %q, want end_turn", r2.StopReason)
	}
	if len(sink2.events) != 1 || sink2.events[0] != "text:turn2" {
		t.Errorf("turn2 events = %v", sink2.events)
	}
}

func TestMockProvider_RunAfterExhausted(t *testing.T) {
	m := New(Script{Final: provider.FinalResponse{StopReason: "end_turn"}})
	sink := &recordingSink{}
	if _, err := m.Run(context.Background(), provider.Request{}, sink); err != nil {
		t.Fatalf("first Run error: %v", err)
	}
	resp, err := m.Run(context.Background(), provider.Request{}, sink)
	if err == nil {
		t.Fatalf("expected error after scripts exhausted")
	}
	if !strings.Contains(err.Error(), "no script") {
		t.Errorf("err = %v, want it to mention 'no script'", err)
	}
	if resp != nil {
		t.Errorf("resp = %+v, want nil on exhausted", resp)
	}
}
