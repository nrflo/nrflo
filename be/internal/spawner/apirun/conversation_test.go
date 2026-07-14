package apirun

import (
	"context"
	"errors"
	"testing"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// recordingProvider wraps mock's scripted provider but captures the Request
// handed to it on every Run() call, so a test can assert what history a later
// turn replayed — mock.mockProvider discards the request.
type recordingProvider struct {
	inner    provider.Provider
	requests []provider.Request
}

func newRecordingProvider(scripts ...mock.Script) *recordingProvider {
	return &recordingProvider{inner: mock.New(scripts...)}
}

func (p *recordingProvider) Name() string                { return p.inner.Name() }
func (p *recordingProvider) MaxContext(model string) int { return p.inner.MaxContext(model) }
func (p *recordingProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	p.requests = append(p.requests, req)
	return p.inner.Run(ctx, req, sink)
}

func newConvTestProc() *fakeProc {
	return &fakeProc{sessionID: "chat-sess-1", projectID: "p1", wfiID: ""}
}

// TestConversation_SendTurn_HistoryPreservedAcrossTurns verifies turn 2's
// Request.Messages replays [user1, assistant1, user2] — the whole point of
// Conversation over single-shot Run.
func TestConversation_SendTurn_HistoryPreservedAcrossTurns(t *testing.T) {
	sink := &recordingSink{}
	prov := newRecordingProvider(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "end_turn",
			Content:    []provider.ContentBlock{{Type: "text", Text: "reply one"}},
		}},
		mock.Script{Final: provider.FinalResponse{
			StopReason: "end_turn",
			Content:    []provider.ContentBlock{{Type: "text", Text: "reply two"}},
		}},
	)

	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	status1 := conv.SendTurn(context.Background(), proc, "hello one")
	if status1 != "PASS" {
		t.Fatalf("turn 1 status = %q, want PASS", status1)
	}
	status2 := conv.SendTurn(context.Background(), proc, "hello two")
	if status2 != "PASS" {
		t.Fatalf("turn 2 status = %q, want PASS", status2)
	}

	if len(prov.requests) != 2 {
		t.Fatalf("provider.Run called %d times, want 2", len(prov.requests))
	}
	turn2Msgs := prov.requests[1].Messages
	if len(turn2Msgs) != 3 {
		t.Fatalf("turn 2 Messages = %d entries, want 3 ([user1, assistant1, user2]); got %+v", len(turn2Msgs), turn2Msgs)
	}
	if turn2Msgs[0].Role != "user" || turn2Msgs[0].Content[0].Text != "hello one" {
		t.Errorf("turn2Msgs[0] = %+v, want user/hello one", turn2Msgs[0])
	}
	if turn2Msgs[1].Role != "assistant" || turn2Msgs[1].Content[0].Text != "reply one" {
		t.Errorf("turn2Msgs[1] = %+v, want assistant/reply one", turn2Msgs[1])
	}
	if turn2Msgs[2].Role != "user" || turn2Msgs[2].Content[0].Text != "hello two" {
		t.Errorf("turn2Msgs[2] = %+v, want user/hello two", turn2Msgs[2])
	}
}

// TestConversation_EndTurn_DoesNotFinalizeSession verifies an end_turn does
// NOT set a session-final signal beyond the per-turn PASS status: proc's
// SetFinalStatus is record-only (fakeProc), and a second SendTurn after an
// end_turn still runs normally rather than being rejected as "already done".
func TestConversation_EndTurn_DoesNotFinalizeSession(t *testing.T) {
	sink := &recordingSink{}
	prov := newRecordingProvider(
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)
	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	if status := conv.SendTurn(context.Background(), proc, "turn 1"); status != "PASS" {
		t.Fatalf("turn 1 status = %q, want PASS", status)
	}
	if got := proc.FinalStatus(); got != "PASS" {
		t.Errorf("proc.FinalStatus after turn 1 = %q, want PASS (record-only, not a session kill)", got)
	}

	// A second turn on the SAME conversation must still be honored — end_turn
	// is a turn boundary, not a session end.
	if status := conv.SendTurn(context.Background(), proc, "turn 2"); status != "PASS" {
		t.Fatalf("turn 2 status = %q, want PASS (end_turn must not block a subsequent SendTurn)", status)
	}
	if len(prov.requests) != 2 {
		t.Fatalf("provider.Run called %d times, want 2", len(prov.requests))
	}
}

// TestConversation_ToolUse_RoundTripReplaysInNextTurn verifies a tool_use
// round trip lands assistant+tool_result blocks in history and the NEXT turn
// replays them.
func TestConversation_ToolUse_RoundTripReplaysInNextTurn(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "findings_add", output: "ok"}
	prov := newRecordingProvider(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{toolUseBlock("tu_1", "findings_add", `{"key":"k"}`)},
		}},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)
	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"findings_add": handler},
		MaxIterations: 5,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	if status := conv.SendTurn(context.Background(), proc, "please add finding"); status != "PASS" {
		t.Fatalf("turn 1 status = %q, want PASS", status)
	}
	if status := conv.SendTurn(context.Background(), proc, "next turn"); status != "PASS" {
		t.Fatalf("turn 2 status = %q, want PASS", status)
	}

	// Turn 1 alone makes 2 provider.Run calls internally (tool_use, then
	// end_turn) before returning — so turn 2's SendTurn issues the 3rd overall
	// call (prov.requests[2]), which must replay turn 1's full history: user1,
	// assistant(tool_use), user(tool_result), assistant(turn 1's end_turn),
	// plus turn 2's own user message — 5 entries.
	turn2Req := prov.requests[2]
	if len(turn2Req.Messages) != 5 {
		t.Fatalf("turn 2 replay Messages = %d, want 5; got %+v", len(turn2Req.Messages), turn2Req.Messages)
	}
	assistantMsg := turn2Req.Messages[1]
	if assistantMsg.Role != "assistant" || assistantMsg.Content[0].Type != "tool_use" {
		t.Errorf("Messages[1] = %+v, want assistant/tool_use", assistantMsg)
	}
	toolResultMsg := turn2Req.Messages[2]
	if toolResultMsg.Role != "user" || toolResultMsg.Content[0].Type != "tool_result" {
		t.Errorf("Messages[2] = %+v, want user/tool_result", toolResultMsg)
	}
	if toolResultMsg.Content[0].ToolUseID != "tu_1" {
		t.Errorf("tool_result.ToolUseID = %q, want tu_1", toolResultMsg.Content[0].ToolUseID)
	}
}

// TestConversation_PerTurnIterationCap_ResetsOnNextTurn verifies MaxIterations
// applies per SendTurn, not per session: a tool_use-forever script fails THAT
// turn (max iterations reached), and a subsequent SendTurn with an end_turn
// script succeeds — proving the loop counter reset for the new turn.
func TestConversation_PerTurnIterationCap_ResetsOnNextTurn(t *testing.T) {
	sink := &recordingSink{}
	handler := &recordingHandler{name: "loop_tool", output: "again"}

	toolUseForever := func() mock.Script {
		return mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{toolUseBlock("tu_x", "loop_tool", `{}`)},
		}}
	}
	prov := newRecordingProvider(
		toolUseForever(), toolUseForever(), // MaxIterations=2 exhausts here
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}}, // next turn
	)
	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		Handlers:      Registry{"loop_tool": handler},
		MaxIterations: 2,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	status1 := conv.SendTurn(context.Background(), proc, "loop please")
	if status1 != "FAIL" {
		t.Fatalf("turn 1 status = %q, want FAIL (max iterations reached)", status1)
	}

	status2 := conv.SendTurn(context.Background(), proc, "now stop")
	if status2 != "PASS" {
		t.Fatalf("turn 2 status = %q, want PASS (cap is per-turn, not per-session)", status2)
	}
	if len(prov.requests) != 3 {
		t.Fatalf("provider.Run called %d times, want 3", len(prov.requests))
	}
}

// TestConversation_ProviderError_MidTurn_ReturnsStatusWithoutDroppingHistory
// verifies a provider error mid-turn returns FAIL/RATE_LIMITED without
// dropping the history accumulated by prior turns.
func TestConversation_ProviderError_MidTurn_ReturnsStatusWithoutDroppingHistory(t *testing.T) {
	sink := &recordingSink{}
	prov := newRecordingProvider(
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
		mock.Script{Err: errors.New("boom")},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)
	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	if status := conv.SendTurn(context.Background(), proc, "turn 1"); status != "PASS" {
		t.Fatalf("turn 1 status = %q, want PASS", status)
	}
	status2 := conv.SendTurn(context.Background(), proc, "turn 2 (errors)")
	if status2 != "FAIL" {
		t.Fatalf("turn 2 status = %q, want FAIL", status2)
	}

	// History from turn 1 must survive the failed turn 2 — turn 3 (a fresh
	// SendTurn) still replays it.
	status3 := conv.SendTurn(context.Background(), proc, "turn 3")
	if status3 != "PASS" {
		t.Fatalf("turn 3 status = %q, want PASS", status3)
	}
	turn3Req := prov.requests[2]
	// turn1 user + turn1 assistant + turn2 user (turn2's failed msgs slice was
	// discarded by runTurns on error, so turn2's user text is NOT replayed —
	// only what SendTurn persisted from turn1, plus turn3's own user message).
	foundTurn1User := false
	for _, m := range turn3Req.Messages {
		if m.Role == "user" && len(m.Content) > 0 && m.Content[0].Text == "turn 1" {
			foundTurn1User = true
		}
	}
	if !foundTurn1User {
		t.Errorf("turn 3 Messages = %+v, want turn 1's user message preserved", turn3Req.Messages)
	}
}

// TestConversation_RateLimitedError_MidTurn verifies a rate-limit classified
// provider error (429, same makeOpenAIErr helper errors_test.go uses) returns
// RATE_LIMITED.
func TestConversation_RateLimitedError_MidTurn(t *testing.T) {
	sink := &recordingSink{}
	prov := newRecordingProvider(mock.Script{Err: makeOpenAIErr(429)})
	conv := NewConversation(Config{
		Provider:      prov,
		Sink:          sink,
		MaxIterations: 3,
		MaxContext:    1000,
	})
	proc := newConvTestProc()

	status := conv.SendTurn(context.Background(), proc, "hit rate limit")
	if status != "RATE_LIMITED" {
		t.Fatalf("status = %q, want RATE_LIMITED", status)
	}
}
