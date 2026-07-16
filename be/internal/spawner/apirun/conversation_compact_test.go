package apirun

import (
	"context"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

func endTurn(text string, usage provider.Usage) mock.Script {
	return mock.Script{Final: provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: text}},
		Usage:      usage,
	}}
}

// TestConversation_AutoCompact_ReplacesHistoryWithSummary drives the window
// low over two turns, then asserts the third SendTurn first summarizes
// (consuming its own provider script) and replays only
// [summary user, ack assistant, new user] to the provider, plus emits the
// "conversation compacted" system row.
func TestConversation_AutoCompact_ReplacesHistoryWithSummary(t *testing.T) {
	sink := &recordingSink{}
	lowUsage := provider.Usage{InputTokens: 900} // 1000-window → 10% left
	prov := newRecordingProvider(
		endTurn("reply one", lowUsage),
		endTurn("reply two", lowUsage),
		endTurn("SUMMARY-XYZ", provider.Usage{}), // consumed by summarize
		endTurn("reply three", provider.Usage{}),
	)

	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	if s := conv.SendTurn(context.Background(), proc, "turn one"); s != "PASS" {
		t.Fatalf("turn 1 status = %q", s)
	}
	// 2 messages < compactMinMessages — turn 2 must NOT compact.
	if s := conv.SendTurn(context.Background(), proc, "turn two"); s != "PASS" {
		t.Fatalf("turn 2 status = %q", s)
	}
	if len(prov.requests) != 2 {
		t.Fatalf("provider calls after turn 2 = %d, want 2 (no compaction yet)", len(prov.requests))
	}

	if s := conv.SendTurn(context.Background(), proc, "turn three"); s != "PASS" {
		t.Fatalf("turn 3 status = %q", s)
	}
	if len(prov.requests) != 4 {
		t.Fatalf("provider calls = %d, want 4 (summarize + turn)", len(prov.requests))
	}

	// Call 3 is the summarize: full history + the summarize instruction.
	sumReq := prov.requests[2]
	if !strings.Contains(sumReq.System, "summarizer") {
		t.Errorf("summarize System = %q, want the compaction system prompt", sumReq.System)
	}

	// Call 4 replays the compacted history: summary user, ack assistant, new user.
	turnReq := prov.requests[3]
	if len(turnReq.Messages) != 3 {
		t.Fatalf("post-compaction request has %d messages, want 3: %+v", len(turnReq.Messages), turnReq.Messages)
	}
	if turnReq.Messages[0].Role != "user" || !strings.Contains(turnReq.Messages[0].Content[0].Text, "SUMMARY-XYZ") {
		t.Errorf("message 0 = %+v, want user summary containing SUMMARY-XYZ", turnReq.Messages[0])
	}
	if turnReq.Messages[1].Role != "assistant" {
		t.Errorf("message 1 role = %q, want assistant ack", turnReq.Messages[1].Role)
	}
	if turnReq.Messages[2].Content[0].Text != "turn three" {
		t.Errorf("message 2 = %+v, want the new user turn", turnReq.Messages[2])
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

// TestConversation_AutoCompact_SummarizeFailure_ProceedsUncompacted verifies
// a failing summarize call surfaces a system row and the turn still runs on
// the full history.
func TestConversation_AutoCompact_SummarizeFailure_ProceedsUncompacted(t *testing.T) {
	sink := &recordingSink{}
	lowUsage := provider.Usage{InputTokens: 900}
	prov := newRecordingProvider(
		endTurn("reply one", lowUsage),
		endTurn("reply two", lowUsage),
		mock.Script{Err: context.DeadlineExceeded}, // summarize fails
		endTurn("reply three", provider.Usage{}),
	)

	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	conv.SendTurn(context.Background(), proc, "turn one")
	conv.SendTurn(context.Background(), proc, "turn two")
	if s := conv.SendTurn(context.Background(), proc, "turn three"); s != "PASS" {
		t.Fatalf("turn 3 status = %q", s)
	}

	turnReq := prov.requests[3]
	if len(turnReq.Messages) != 5 {
		t.Errorf("uncompacted request has %d messages, want 5 (full history)", len(turnReq.Messages))
	}
	foundFail := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "compaction failed") {
			foundFail = true
		}
	}
	if !foundFail {
		t.Errorf("expected 'compaction failed' system row, got %+v", sink.Calls())
	}
}
