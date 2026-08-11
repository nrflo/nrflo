package apirun

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// queueSteer is a SteerSource fed by tests; Drain empties it.
type queueSteer struct {
	mu    sync.Mutex
	texts []string
}

func (q *queueSteer) Add(t string) {
	q.mu.Lock()
	q.texts = append(q.texts, t)
	q.mu.Unlock()
}

func (q *queueSteer) Drain() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.texts
	q.texts = nil
	return out
}

// TestRunner_SteerDrainedAtToolBoundary: text pending in Config.Steer rides
// the next tool-results user message as its own text block, exactly once.
func TestRunner_SteerDrainedAtToolBoundary(t *testing.T) {
	steer := &queueSteer{}
	steer.Add("mid-turn question")
	prov := &captureProvider{inner: mock.New(
		toolUseScript("tu_1"),
		toolUseScript("tu_2"),
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)}

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          &recordingSink{},
		Handlers:      Registry{"tool_a": &gateHandler{name: "tool_a", out: "ok"}},
		InitialPrompt: "go",
		MaxIterations: 10,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
		Steer:         steer,
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	reqs := prov.Requests()
	if len(reqs) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(reqs))
	}

	// Turn 0's tool-results message carries tool_result + the steered block.
	steered := reqs[1].Messages[len(reqs[1].Messages)-1]
	if got := len(steered.Content); got != 2 {
		t.Fatalf("turn-0 tool_results blocks = %d, want tool_result + steered text", got)
	}
	tail := steered.Content[1]
	if tail.Type != "text" || !strings.Contains(tail.Text, "mid-turn question") || !strings.Contains(tail.Text, "delivered mid-turn") {
		t.Errorf("steered block = %+v, want labeled mid-turn user text", tail)
	}
	// Drained once: turn 1's tool-results message is clean.
	clean := reqs[2].Messages[len(reqs[2].Messages)-1]
	if got := len(clean.Content); got != 1 {
		t.Errorf("turn-1 tool_results blocks = %d, want 1 (steer buffer already drained)", got)
	}
}
