package apirun

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// recordingHandler is a ToolHandler that records each Invoke call and emits
// a configurable result. If terminal is set, it returns it as the err so the
// runner short-circuits.
type recordingHandler struct {
	mu       sync.Mutex
	name     string
	output   string
	isError  bool
	terminal *TerminalSignal
	calls    []json.RawMessage
}

func (h *recordingHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: h.name, InputSchema: json.RawMessage(`{}`)}
}

func (h *recordingHandler) Invoke(_ context.Context, _ ToolEnv, input json.RawMessage) (string, bool, error) {
	h.mu.Lock()
	cp := make(json.RawMessage, len(input))
	copy(cp, input)
	h.calls = append(h.calls, cp)
	h.mu.Unlock()
	if h.terminal != nil {
		return "", false, *h.terminal
	}
	return h.output, h.isError, nil
}

func (h *recordingHandler) Calls() []json.RawMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]json.RawMessage, len(h.calls))
	copy(out, h.calls)
	return out
}

func toolUseBlock(id, name, input string) provider.ContentBlock {
	return provider.ContentBlock{
		Type:      "tool_use",
		ToolUseID: id,
		ToolName:  name,
		Input:     json.RawMessage(input),
	}
}

func TestRunner_ToolUse_HappyPath(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "findings_add", output: "ok"}

	prov := mock.New(
		mock.Script{
			Final: provider.FinalResponse{
				StopReason: "tool_use",
				Content:    []provider.ContentBlock{toolUseBlock("tu_1", "findings_add", `{"key":"k","value":"v"}`)},
			},
		},
		mock.Script{
			Final: provider.FinalResponse{StopReason: "end_turn"},
		},
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
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	calls := handler.Calls()
	if len(calls) != 1 {
		t.Fatalf("handler invoked %d times, want 1", len(calls))
	}
	if !strings.Contains(string(calls[0]), `"key":"k"`) {
		t.Errorf("input = %s, want key:k", string(calls[0]))
	}
	// expect a tool_result message in sink
	foundResult := false
	for _, c := range sink.Calls() {
		if c.category == "tool_result" && strings.Contains(c.content, "findings_add") {
			foundResult = true
		}
	}
	if !foundResult {
		t.Errorf("no tool_result message in sink: %+v", sink.Calls())
	}
}

func TestRunner_ToolUse_AgentFailTerminal(t *testing.T) {
	sink := &recordingSink{}
	terminalFail := TerminalSignal{Status: "FAIL", Reason: "broken"}
	handler := &recordingHandler{name: "agent_fail", terminal: &terminalFail}

	// Only one script: runner must short-circuit after the terminal signal
	// without requesting a second turn.
	prov := mock.New(mock.Script{
		Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{toolUseBlock("tu_1", "agent_fail", `{}`)},
		},
	})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"agent_fail": handler},
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "FAIL" {
		t.Errorf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	if len(handler.Calls()) != 1 {
		t.Errorf("handler calls = %d, want 1", len(handler.Calls()))
	}
}

func TestRunner_ToolUse_AgentContinueTerminal(t *testing.T) {
	sink := &recordingSink{}
	terminal := TerminalSignal{Status: "CONTINUE"}
	handler := &recordingHandler{name: "agent_continue", terminal: &terminal}

	prov := mock.New(mock.Script{
		Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{toolUseBlock("tu_1", "agent_continue", `{}`)},
		},
	})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"agent_continue": handler},
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "CONTINUE" {
		t.Errorf("FinalStatus = %q, want CONTINUE", proc.FinalStatus())
	}
}

func TestRunner_ToolUse_AgentCallbackTerminal(t *testing.T) {
	sink := &recordingSink{}
	terminal := TerminalSignal{Status: "CALLBACK", Level: 1}
	handler := &recordingHandler{name: "agent_callback", terminal: &terminal}

	prov := mock.New(mock.Script{
		Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{toolUseBlock("tu_1", "agent_callback", `{"level":1}`)},
		},
	})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"agent_callback": handler},
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "CALLBACK" {
		t.Errorf("FinalStatus = %q, want CALLBACK", proc.FinalStatus())
	}
	if proc.CallbackLevel() != 1 {
		t.Errorf("CallbackLevel = %d, want 1", proc.CallbackLevel())
	}
}

func TestRunner_ToolUse_UnknownTool_ContinuesAndPasses(t *testing.T) {
	sink := &recordingSink{}

	prov := mock.New(
		mock.Script{
			Final: provider.FinalResponse{
				StopReason: "tool_use",
				Content:    []provider.ContentBlock{toolUseBlock("tu_x", "no_such_tool", `{}`)},
			},
		},
		mock.Script{
			Final: provider.FinalResponse{StopReason: "end_turn"},
		},
	)

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{}, // empty registry
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Errorf("FinalStatus = %q, want PASS (unknown tool returns is_error and loop continues)", proc.FinalStatus())
	}
	// Sink should record a tool_error message.
	found := false
	for _, c := range sink.Calls() {
		if c.category == "tool_error" && strings.Contains(c.content, "unknown tool") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tool_error message, got %+v", sink.Calls())
	}
}

func TestRunner_ToolUse_HandlerError_NonTerminal(t *testing.T) {
	// A handler returning a non-TerminalSignal error: runner should propagate
	// it as a tool_result with isError=true and continue the loop.
	sink := &recordingSink{}
	handler := &errReturningHandler{name: "oops"}

	prov := mock.New(
		mock.Script{
			Final: provider.FinalResponse{
				StopReason: "tool_use",
				Content:    []provider.ContentBlock{toolUseBlock("tu_1", "oops", `{}`)},
			},
		},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"oops": handler},
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Errorf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	foundErr := false
	for _, c := range sink.Calls() {
		if c.category == "tool_result" && strings.Contains(c.content, "[tool_result:error]") {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected [tool_result:error] in sink, got %+v", sink.Calls())
	}
}

type errReturningHandler struct{ name string }

func (h *errReturningHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: h.name, InputSchema: json.RawMessage(`{}`)}
}

func (h *errReturningHandler) Invoke(_ context.Context, _ ToolEnv, _ json.RawMessage) (string, bool, error) {
	return "", false, errPlain("disk full")
}

type errPlain string

func (e errPlain) Error() string { return string(e) }

func TestRunner_ToolUse_SequentialDispatchOrder(t *testing.T) {
	// Two tool_use blocks in one turn — handler invocation order must match
	// block order. We use a single handler shared by both names and check the
	// order of recorded inputs.
	sink := &recordingSink{}
	// Track total order via a shared slice
	order := orderRecorder{}
	wrappedA := orderingHandler{recorder: &order, name: "tool_a", out: "a"}
	wrappedB := orderingHandler{recorder: &order, name: "tool_b", out: "b"}

	prov := mock.New(
		mock.Script{
			Final: provider.FinalResponse{
				StopReason: "tool_use",
				Content: []provider.ContentBlock{
					toolUseBlock("tu_a", "tool_a", `{"i":1}`),
					toolUseBlock("tu_b", "tool_b", `{"i":2}`),
				},
			},
		},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)

	r := NewRunner(Config{
		Provider: prov,
		Sink:     sink,
		Handlers: Registry{
			"tool_a": &wrappedA,
			"tool_b": &wrappedB,
		},
		InitialPrompt: "go",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Errorf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	got := order.Names()
	if len(got) != 2 || got[0] != "tool_a" || got[1] != "tool_b" {
		t.Errorf("dispatch order = %v, want [tool_a tool_b]", got)
	}
}

type orderRecorder struct {
	mu    sync.Mutex
	names []string
}

func (o *orderRecorder) record(name string) {
	o.mu.Lock()
	o.names = append(o.names, name)
	o.mu.Unlock()
}

func (o *orderRecorder) Names() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, len(o.names))
	copy(out, o.names)
	return out
}

type orderingHandler struct {
	recorder *orderRecorder
	name     string
	out      string
}

func (h *orderingHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: h.name, InputSchema: json.RawMessage(`{}`)}
}

func (h *orderingHandler) Invoke(_ context.Context, _ ToolEnv, _ json.RawMessage) (string, bool, error) {
	h.recorder.record(h.name)
	return h.out, false, nil
}

func TestRunner_ToolUse_NoBlocksInResponse_Fails(t *testing.T) {
	// stop_reason=tool_use but no tool_use blocks — runner emits FAIL.
	sink := &recordingSink{}
	prov := mock.New(mock.Script{
		Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{{Type: "text", Text: "lol"}},
		},
	})
	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{},
		InitialPrompt: "go",
		MaxIterations: 5,
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
		if c.category == "system" && strings.Contains(c.content, "no tool_use blocks") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'no tool_use blocks' message, got %+v", sink.Calls())
	}
}

