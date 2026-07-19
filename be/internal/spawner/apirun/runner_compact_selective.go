package apirun

import (
	"context"
	"fmt"
	"strings"

	"be/internal/spawner/apirun/provider"
)

// applyCompactionPlan replaces msgs[plan.KeepPrefixMsgs:len(msgs)-plan.KeepSuffixMsgs]
// with a single digest message, keeping the pinned prefix and the recent
// window byte-identical — unlike maybeCompactInLoop/maybeCompact (which
// replace the whole history), this is the selective applier a ContextWatcher
// plan drives. Failure (or an empty/degenerate range) is non-fatal: the loop
// proceeds on the uncompacted history.
func applyCompactionPlan(ctx context.Context, cfg Config, msgs []provider.Message, plan CompactionPlan) []provider.Message {
	start := plan.KeepPrefixMsgs
	if start < 0 {
		start = 0
	}
	end := len(msgs) - plan.KeepSuffixMsgs
	if end > len(msgs) {
		end = len(msgs)
	}
	if start >= end {
		return msgs
	}

	// The digest message must differ in role from both neighbors to keep
	// strict user/assistant alternation. Since the untouched messages still
	// alternate, that holds only when the evicted range has odd length;
	// grow it on the suffix side (never touching the pinned prefix) until it
	// does.
	for start < end && end < len(msgs) && (end-start)%2 == 0 {
		end++
	}
	if start >= end {
		return msgs
	}

	summary, err := summarizeHistory(ctx, cfg, msgs[start:end])
	if err != nil || strings.TrimSpace(summary) == "" {
		if cfg.Sink != nil {
			cfg.Sink.TrackMessage(fmt.Sprintf("selective compaction failed (continuing uncompacted): %v", err), "system")
		}
		return msgs
	}

	digestText := "[Selective context GC: the following messages were evicted to free context. Summary:]\n\n" + summary
	if plan.ReferenceDigest != "" {
		digestText += "\n\n" + plan.ReferenceDigest
	}

	digestRole := "user"
	switch {
	case start > 0 && msgs[start-1].Role == "user":
		digestRole = "assistant"
	case start == 0 && end < len(msgs) && msgs[end].Role == "user":
		digestRole = "assistant"
	}

	out := make([]provider.Message, 0, start+1+(len(msgs)-end))
	out = append(out, msgs[:start]...)
	out = append(out, provider.Message{
		Role:    digestRole,
		Content: []provider.ContentBlock{{Type: "text", Text: digestText}},
	})
	out = append(out, msgs[end:]...)

	if cfg.Sink != nil {
		cfg.Sink.TrackMessage(fmt.Sprintf(
			"conversation compacted (selective, policy=%s): %d messages evicted (~%d tokens), %d kept verbatim",
			plan.PolicyName, end-start, plan.TokensEvicted, len(out)-1), "system")
	}

	return out
}

// maybeWatcherGC consults Config.Watcher before the first request of a new
// turn — the idle-gap policy's cache-free rewrite point, since nothing has
// been sent to the provider since the last turn ended and a rewrite here
// costs nothing in cache terms. Nil-safe: no-op when Watcher is unset or
// declines to act.
func (c *Conversation) maybeWatcherGC(ctx context.Context) {
	if c.cfg.Watcher == nil {
		return
	}
	c.mu.Lock()
	msgs := c.msgs
	pctLeft, pctKnown := c.contextLeft, c.contextKnown
	c.mu.Unlock()

	plan, ok := c.cfg.Watcher.PlanGC(WatcherState{MessageCount: len(msgs), PctLeft: pctLeft, PctKnown: pctKnown})
	if !ok {
		return
	}
	newMsgs := applyCompactionPlan(ctx, c.cfg, msgs, plan)

	c.mu.Lock()
	c.msgs = newMsgs
	c.contextKnown = false // stale until the next turn reports fresh usage
	c.mu.Unlock()
}
