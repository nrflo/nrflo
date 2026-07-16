package apirun

import (
	"context"
	"fmt"
	"strings"

	"be/internal/spawner/apirun/provider"
)

// maybeCompactInLoop replaces msgs with a compacted history when the previous
// turn of the tool-use loop left the context window at or below
// cfg.CompactPct. Unlike Conversation.maybeCompact (which runs between user
// turns and can keep a user/assistant pair), the loop's next request must end
// on a user message, so the replacement is a single user message: the original
// task prompt (when the runner has one — autonomous Run) plus the summary and
// a continue instruction. Failure is non-fatal: the loop proceeds on the
// uncompacted history. Returns the (possibly replaced) history and whether a
// compaction happened.
func (r *Runner) maybeCompactInLoop(ctx context.Context, proc ProcState, msgs []provider.Message, pctLeft int) ([]provider.Message, bool) {
	if pctLeft > r.cfg.CompactPct || len(msgs) < compactMinMessages {
		return msgs, false
	}

	summary, err := summarizeHistory(ctx, r.cfg, msgs)
	if err != nil || strings.TrimSpace(summary) == "" {
		r.cfg.Sink.TrackMessage(fmt.Sprintf("compaction failed (continuing uncompacted): %v", err), "system")
		return msgs, false
	}

	blocks := make([]provider.ContentBlock, 0, 2)
	if r.cfg.InitialPrompt != "" {
		blocks = append(blocks, provider.ContentBlock{Type: "text", Text: r.cfg.InitialPrompt})
	}
	blocks = append(blocks, provider.ContentBlock{
		Type: "text",
		Text: "[The conversation so far was compacted to free context. Summary of everything before this point:]\n\n" +
			summary +
			"\n\nContinue the work from this summary — do not restart from scratch and do not re-run tool calls whose results are captured above.",
	})

	r.cfg.Sink.TrackMessage(fmt.Sprintf("conversation compacted: %d messages summarized at %d%% context left", len(msgs), pctLeft), "system")
	// Optimistic reset so monitorAll's low-context kill doesn't fire on the
	// pre-compaction value; the next turn's usage reports the real figure.
	proc.SetContextLeft(100)
	if r.cfg.AgentSvc != nil {
		r.cfg.AgentSvc.UpdateContextLeft(proc.SessionID(), 100)
	}

	return []provider.Message{{Role: "user", Content: blocks}}, true
}
