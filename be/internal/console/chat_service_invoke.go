package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"be/internal/repo"
	"be/internal/ws"
)

// invokeResultCap bounds the tool result text persisted to the transcript,
// folded into seedContext, and returned to the caller — applied once and
// reused in all three places so they never disagree.
const invokeResultCap = 16 * 1024

// InvokeResult is ChatService.InvokeTool's return shape.
type InvokeResult struct {
	OK         bool
	Result     string
	DurationMs int64
	Informed   bool
}

// InvokeTool runs one deterministic, server-side tool call against sid's own
// profile catalogue — the same registry/dispatch path a chat's engine reaches
// over MCP, but driven directly instead of through a model turn. guardIdle
// rejects while a turn is in flight (spawner.ErrTurnActive, mapped to 409 by
// the handler) WITHOUT starting one: an invoke never begins a model turn.
//
// A real Dispatch error (ErrToolNotFound aside, which bubbles up for the
// handler's 404) or an isError result never becomes a 5xx — both are
// recorded as ok:false with the message/error text as the result, exactly
// like handleCallConsoleTool's tool_error/error outcomes.
//
// Two agent_messages rows (user_input, tool) are persisted in one InsertBatch
// and a single messages.updated event is broadcast on the session's WS
// channel. When informModel, a compact digest is appended (not replacing) to
// the session's pending seedContext, folded into the caller's NEXT
// SendUserTurn — never this call's.
func (s *ChatService) InvokeTool(ctx context.Context, sid, tool string, args json.RawMessage, informModel bool) (InvokeResult, error) {
	sess, ok := s.get(sid)
	if !ok {
		return InvokeResult{}, ErrChatSessionNotFound
	}
	if err := sess.guardIdle(); err != nil {
		return InvokeResult{}, err
	}

	profile, err := ProfileByName(sess.profile)
	if err != nil {
		return InvokeResult{}, err
	}
	reg, err := BuildRegistry(s.deps.Tools, profile.Catalogue)
	if err != nil {
		return InvokeResult{}, err
	}
	env := NewToolEnv(s.deps.Tools, sid, sess.projectID)

	start := s.deps.Clock.Now()
	output, isErr, callErr := Dispatch(ctx, reg, env, tool, args)
	dur := s.deps.Clock.Now().Sub(start)

	if errors.Is(callErr, ErrToolNotFound) {
		return InvokeResult{}, callErr
	}

	ok2 := callErr == nil && !isErr
	result := output
	if callErr != nil {
		result = callErr.Error()
	}
	result = truncateResult(result)

	compact := compactArgs(args)
	userContent := "/invoke " + tool + " " + compact
	toolContent := tool + " → " + result
	s.recordInvokeTranscript(sid, sess.projectID, userContent, toolContent)
	s.touchRefinery(sid)

	informed := false
	if informModel {
		digest := "[console invoke] " + tool + " " + compact + " → " + result
		sess.appendSeedContext(digest)
		informed = true
	}

	return InvokeResult{OK: ok2, Result: result, DurationMs: dur.Milliseconds(), Informed: informed}, nil
}

// recordInvokeTranscript persists the user_input+tool rows for one invoke and
// broadcasts messages.updated on the session's WS channel — mirrors
// chatSink's persistence/broadcast split for engine-originated messages.
func (s *ChatService) recordInvokeTranscript(sid, projectID, userContent, toolContent string) {
	repo.NewAgentMessageRepo(s.deps.Pool, s.deps.Clock).InsertBatch(sid, []repo.MessageEntry{ //nolint:errcheck
		{Content: userContent, Category: "user_input"},
		{Content: toolContent, Category: "tool"},
	})
	pushSessionEvent(s.deps.WSHub, sid, projectID, ws.EventMessagesUpdated, map[string]interface{}{"session_id": sid})
}

// compactArgs collapses raw JSON args to a single compact line for the
// transcript/digest; an empty/blank body becomes "{}".
func compactArgs(args json.RawMessage) string {
	if len(bytes.TrimSpace(args)) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, args); err != nil {
		return string(args)
	}
	return buf.String()
}

// truncateResult cuts result to invokeResultCap bytes at a UTF-8 rune
// boundary, appending a marker when truncated.
func truncateResult(result string) string {
	if len(result) <= invokeResultCap {
		return result
	}
	cut := result[:invokeResultCap]
	for len(cut) > 0 && !utf8.RuneStart(cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…[truncated]"
}
