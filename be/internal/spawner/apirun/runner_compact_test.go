package apirun

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// toolTurn scripts one tool_use response invoking `name` with the given usage.
func toolTurn(id, name string, usage provider.Usage) mock.Script {
	return mock.Script{Final: provider.FinalResponse{
		StopReason: "tool_use",
		Content:    []provider.ContentBlock{toolUseBlock(id, name, `{}`)},
		Usage:      usage,
	}}
}

// TestRunner_InLoopCompact_ReplacesHistoryKeepsInitialPrompt drives a
// single-shot Run through two low-context tool turns, then asserts the third
// iteration first summarizes and replays a single user message carrying the
// original task prompt plus the summary — the mid-loop analog of
// Conversation.maybeCompact, whose replacement must end on a user message.
func TestRunner_InLoopCompact_ReplacesHistoryKeepsInitialPrompt(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "probe", output: "ok"}
	lowUsage := provider.Usage{InputTokens: 900} // 1000-window → 10% left
	prov := newRecordingProvider(
		toolTurn("tu_1", "probe", lowUsage),         // history after: 3 msgs (< compactMinMessages)
		toolTurn("tu_2", "probe", lowUsage),         // history after: 5 msgs — next turn compacts
		endTurn("SUMMARY-RUN-42", provider.Usage{}), // consumed by summarize
		endTurn("done", provider.Usage{}),
	)

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"probe": handler},
		InitialPrompt: "TASK-PROMPT-99",
		MaxIterations: 5,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	if len(prov.requests) != 4 {
		t.Fatalf("provider calls = %d, want 4 (2 turns + summarize + final)", len(prov.requests))
	}

	sumReq := prov.requests[2]
	if !strings.Contains(sumReq.System, "summarizer") {
		t.Errorf("summarize System = %q, want the compaction system prompt", sumReq.System)
	}

	// The post-compaction request is ONE user message: [initial prompt block,
	// summary+continue block] — the loop's next request must end on user.
	turnReq := prov.requests[3]
	if len(turnReq.Messages) != 1 || turnReq.Messages[0].Role != "user" {
		t.Fatalf("post-compaction request = %+v, want a single user message", turnReq.Messages)
	}
	blocks := turnReq.Messages[0].Content
	if len(blocks) != 2 || blocks[0].Text != "TASK-PROMPT-99" {
		t.Fatalf("post-compaction blocks = %+v, want [initial prompt, summary]", blocks)
	}
	if !strings.Contains(blocks[1].Text, "SUMMARY-RUN-42") || !strings.Contains(blocks[1].Text, "Continue the work") {
		t.Errorf("summary block = %q, want summary + continue instruction", blocks[1].Text)
	}

	// The compaction resets the reported context-left optimistically so the
	// spawner's low-context kill doesn't fire on the stale value.
	if got := proc.ContextLeft(); got != 100 {
		t.Errorf("context left after compaction = %d, want 100", got)
	}
	foundRow := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "conversation compacted") {
			foundRow = true
		}
	}
	if !foundRow {
		t.Errorf("expected 'conversation compacted' system row, got %+v", sink.Calls())
	}
}

// TestRunner_InLoopCompact_AboveThreshold_NoCompaction: plenty of window left
// → the loop never summarizes, even across many tool turns.
func TestRunner_InLoopCompact_AboveThreshold_NoCompaction(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "probe", output: "ok"}
	okUsage := provider.Usage{InputTokens: 100} // 90% left
	prov := newRecordingProvider(
		toolTurn("tu_1", "probe", okUsage),
		toolTurn("tu_2", "probe", okUsage),
		toolTurn("tu_3", "probe", okUsage),
		endTurn("done", provider.Usage{}),
	)

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"probe": handler},
		InitialPrompt: "task",
		MaxIterations: 6,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "PASS" {
		t.Fatalf("FinalStatus = %q, want PASS", proc.FinalStatus())
	}
	if len(prov.requests) != 4 {
		t.Fatalf("provider calls = %d, want 4 (no summarize call)", len(prov.requests))
	}
	for _, c := range sink.Calls() {
		if strings.Contains(c.content, "compacted") {
			t.Errorf("unexpected compaction row: %+v", c)
		}
	}
}
