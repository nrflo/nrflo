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

func TestRunner_ToolUse_ParallelDispatchOrderedResults(t *testing.T) {
	// Two tool_use blocks in one turn run CONCURRENTLY (tool_a's handler
	// blocks until tool_b's has started — sequential dispatch would time the
	// gate out) while the tool_result blocks replayed to the provider keep
	// the original block order regardless of completion order.
	sink := &recordingSink{}
	bStarted := make(chan struct{})
	handlerA := &gateHandler{name: "tool_a", out: "a", wait: bStarted}
	handlerB := &gateHandler{name: "tool_b", out: "b", signal: bStarted}

	prov := &captureProvider{inner: mock.New(
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
	)}

	r := NewRunner(Config{
		Provider: prov,
		Sink:     sink,
		Handlers: Registry{
			"tool_a": handlerA,
			"tool_b": handlerB,
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

	reqs := prov.Requests()
	if len(reqs) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if last.Role != "user" || len(last.Content) != 2 {
		t.Fatalf("tool_result message = %+v, want user message with 2 blocks", last)
	}
	if last.Content[0].ToolUseID != "tu_a" || last.Content[0].Output != "a" ||
		last.Content[1].ToolUseID != "tu_b" || last.Content[1].Output != "b" {
		t.Errorf("tool_result order = [%s=%q %s=%q], want [tu_a=a tu_b=b]",
			last.Content[0].ToolUseID, last.Content[0].Output,
			last.Content[1].ToolUseID, last.Content[1].Output)
	}
	if last.Content[0].IsError || last.Content[1].IsError {
		t.Errorf("gate timed out — dispatch ran sequentially: %+v", last.Content)
	}
}

// gateHandler proves concurrency: a `wait` handler blocks until its gate is
// signaled by the other handler's start; sequential dispatch would never
// signal and the wait times out into an error result.
type gateHandler struct {
	name   string
	out    string
	wait   chan struct{} // block until closed (nil = no wait)
	signal chan struct{} // closed on invoke (nil = no signal)
}

func (h *gateHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: h.name, InputSchema: json.RawMessage(`{}`)}
}

func (h *gateHandler) Invoke(_ context.Context, _ ToolEnv, _ json.RawMessage) (string, bool, error) {
	if h.signal != nil {
		close(h.signal)
	}
	if h.wait != nil {
		select {
		case <-h.wait:
		case <-time.After(2 * time.Second):
			return "gate timed out (sequential dispatch?)", true, nil
		}
	}
	return h.out, false, nil
}

// captureProvider records every request forwarded to the wrapped provider so
// tests can assert the exact replayed message history.
type captureProvider struct {
	inner provider.Provider
	mu    sync.Mutex
	reqs  []provider.Request
}

func (c *captureProvider) Name() string                { return c.inner.Name() }
func (c *captureProvider) MaxContext(model string) int { return c.inner.MaxContext(model) }

func (c *captureProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	c.mu.Unlock()
	return c.inner.Run(ctx, req, sink)
}

func (c *captureProvider) Requests() []provider.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]provider.Request, len(c.reqs))
	copy(out, c.reqs)
	return out
}
