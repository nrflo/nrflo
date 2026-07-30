package apirun

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

func toolUseScript(id string) mock.Script {
	return mock.Script{Final: provider.FinalResponse{
		StopReason: "tool_use",
		Content:    []provider.ContentBlock{toolUseBlock(id, "tool_a", `{}`)},
	}}
}

// TestRunner_CapWarningInjected: with MaxIterations=4, the turn that leaves
// exactly capWarningTurns turns remaining gets a wrap-up text block appended
// to its tool-results user message — and only that turn.
func TestRunner_CapWarningInjected(t *testing.T) {
	sink := &recordingSink{}
	prov := &captureProvider{inner: mock.New(
		toolUseScript("tu_1"),
		toolUseScript("tu_2"),
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)}

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"tool_a": &gateHandler{name: "tool_a", out: "ok"}},
		InitialPrompt: "go",
		MaxIterations: 4,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
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

	// Turn 0 leaves 3 turns -> its tool-results message (last of reqs[1]) is
	// warning-free; turn 1 leaves 2 -> warning rides on reqs[2]'s last message.
	clean := reqs[1].Messages[len(reqs[1].Messages)-1]
	if got := len(clean.Content); got != 1 {
		t.Errorf("turn-0 tool_results blocks = %d, want 1 (no warning)", got)
	}
	warned := reqs[2].Messages[len(reqs[2].Messages)-1]
	if got := len(warned.Content); got != 2 {
		t.Fatalf("turn-1 tool_results blocks = %d, want tool_result + warning text", got)
	}
	tail := warned.Content[1]
	if tail.Type != "text" || !strings.Contains(tail.Text, "Iteration cap") {
		t.Errorf("warning block = %+v, want text mentioning Iteration cap", tail)
	}

	found := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "Iteration cap") {
			found = true
		}
	}
	if !found {
		t.Error("expected system sink message for the cap warning")
	}
}

// TestRunner_CapWarningSkippedForTinyCaps: MaxIterations <= capWarningTurns+1
// never injects — a 3-turn agent would otherwise be warned on its first
// tool result.
func TestRunner_CapWarningSkippedForTinyCaps(t *testing.T) {
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
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	for _, req := range prov.Requests() {
		for _, msg := range req.Messages {
			for _, block := range msg.Content {
				if block.Type == "text" && strings.Contains(block.Text, "Iteration cap") {
					t.Fatal("cap warning must not fire when MaxIterations <= capWarningTurns+1")
				}
			}
		}
	}
}
