package apirun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/spawner/apirun/provider"
)

const (
	// compactThresholdPct: SendTurn compacts the history before running when
	// the last turn left less than this % of the context window.
	compactThresholdPct = 15
	// compactMinMessages guards against compacting a conversation too short
	// to be worth a summarize round-trip.
	compactMinMessages = 4
)

const compactSystem = `You are a conversation summarizer. Produce a dense, factual summary of the conversation so far so it can be continued with the summary as the only context. Preserve: the user's goals and constraints, decisions made, exact identifiers (file paths, ids, ticket ids, URLs, commands), tool results that later turns rely on, and any unfinished work. Do not add commentary or preamble — output only the summary.`

// nullEventSink discards streaming events — the compaction summarize call has
// no live consumer.
type nullEventSink struct{}

func (nullEventSink) OnTextDelta(string)                 {}
func (nullEventSink) OnThinkingDelta(string)             {}
func (nullEventSink) OnToolUseStart(string, string)      {}
func (nullEventSink) OnToolUseInputDelta(string, string) {}
func (nullEventSink) OnToolUseStop(string, json.RawMessage) {}
func (nullEventSink) OnUsage(provider.Usage)             {}

// ctxCaptureProc tees SetContextLeft into the Conversation so it knows how
// much window the previous turn left without owning usage accounting itself.
type ctxCaptureProc struct {
	ProcState
	c *Conversation
}

func (p ctxCaptureProc) SetContextLeft(pct int) {
	p.c.noteContextLeft(pct)
	p.ProcState.SetContextLeft(pct)
}

func (c *Conversation) noteContextLeft(pct int) {
	c.mu.Lock()
	c.contextLeft = pct
	c.contextKnown = true
	c.mu.Unlock()
}

// maybeCompact replaces the history with a provider-generated summary when
// the previous turn left the context window nearly full. Failure is
// non-fatal: the turn proceeds on the uncompacted history and the error is
// surfaced as a system row.
func (c *Conversation) maybeCompact(ctx context.Context, proc ProcState) {
	c.mu.Lock()
	known, left, msgs := c.contextKnown, c.contextLeft, c.msgs
	c.mu.Unlock()
	if !known || left > compactThresholdPct || len(msgs) < compactMinMessages {
		return
	}

	summary, err := c.summarize(ctx, msgs)
	if err != nil || strings.TrimSpace(summary) == "" {
		if c.cfg.Sink != nil {
			c.cfg.Sink.TrackMessage(fmt.Sprintf("compaction failed (continuing uncompacted): %v", err), "system")
		}
		return
	}

	// user summary + assistant ack keeps the strict user/assistant
	// alternation both providers expect once SendTurn appends the real turn.
	replacement := []provider.Message{
		{Role: "user", Content: []provider.ContentBlock{{
			Type: "text",
			Text: "[The conversation so far was compacted to free context. Summary of everything before this point:]\n\n" + summary,
		}}},
		{Role: "assistant", Content: []provider.ContentBlock{{
			Type: "text",
			Text: "Understood. Continuing from the summary.",
		}}},
	}

	c.mu.Lock()
	compacted := len(c.msgs)
	c.msgs = replacement
	c.contextKnown = false // stale until the next turn reports fresh usage
	c.mu.Unlock()

	if c.cfg.Sink != nil {
		c.cfg.Sink.TrackMessage(fmt.Sprintf("conversation compacted: %d messages summarized at %d%% context left", compacted, left), "system")
	}
	if proc != nil {
		proc.SetContextLeft(100)
	}
}

// summarize runs one tool-less provider call over the history plus a
// summarize instruction and returns the concatenated text blocks.
func (c *Conversation) summarize(ctx context.Context, msgs []provider.Message) (string, error) {
	req := provider.Request{
		System: compactSystem,
		Messages: append(append([]provider.Message{}, msgs...), provider.Message{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "Summarize the conversation above now."}},
		}),
		MaxTokens:       c.cfg.MaxTokens,
		ToolChoice:      "auto",
		Model:           c.cfg.Model,
		ReasoningEffort: c.cfg.ReasoningEffort,
	}
	resp, err := c.cfg.Provider.Run(ctx, req, nullEventSink{})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, b := range resp.Content {
		if b.Type == "text" {
			out.WriteString(b.Text)
		}
	}
	return out.String(), nil
}
