package apirun

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"be/internal/spawner/apirun/provider"
)

// maxParallelToolDispatch bounds how many tool_use blocks of one assistant
// turn run concurrently. Providers rarely emit more than a handful of
// parallel calls; the cap keeps a pathological turn from fanning out
// unbounded python subprocesses / consultant spawns.
const maxParallelToolDispatch = 4

// runTurns drives the shared tool-use loop starting from msgs until a
// terminal status is reached, returning the accumulated message history
// alongside the terminal status string. It calls proc.SetFinalStatus/r.fail()
// at exactly the same points Run's inline loop used to. Run (single-shot
// autonomous agents) discards the returned history; Conversation.SendTurn
// keeps it so the next turn replays the full transcript. On end_turn the
// assistant's content is appended to msgs before returning "PASS" so a
// Conversation can replay it in the next turn's request.
func (r *Runner) runTurns(ctx context.Context, proc ProcState, msgs []provider.Message) ([]provider.Message, string) {
	pctLeft, pctKnown := 0, false
	for turn := 0; turn < r.cfg.MaxIterations; turn++ {
		if ctx.Err() != nil {
			proc.SetFinalStatus("CANCELLED")
			return msgs, "CANCELLED"
		}
		if !r.cfg.Deadline.IsZero() && !time.Now().Before(r.cfg.Deadline) {
			r.fail(proc, fmt.Sprintf("deadline exceeded (%s)", r.cfg.Deadline.Format(time.RFC3339)))
			return msgs, "FAIL"
		}
		fallbackCompact := func() {
			if !pctKnown {
				return
			}
			var compacted bool
			if msgs, compacted = r.maybeCompactInLoop(ctx, proc, msgs, pctLeft); compacted {
				pctKnown = false // stale until the next turn reports fresh usage
			}
		}
		if r.cfg.Watcher != nil {
			if plan, ok := r.cfg.Watcher.PlanGC(WatcherState{MessageCount: len(msgs), PctLeft: pctLeft, PctKnown: pctKnown}); ok {
				msgs = applyCompactionPlan(ctx, r.cfg, msgs, plan)
				pctKnown = false // stale until the next turn reports fresh usage
			} else {
				fallbackCompact()
			}
		} else {
			fallbackCompact()
		}

		sink := newRunnerSink(r.cfg.Sink, r.cfg.CaptureThinking, r.cfg.Stream)
		req := provider.Request{
			System:           r.cfg.System,
			Messages:         msgs,
			Tools:            r.cfg.Tools,
			MaxTokens:        r.cfg.MaxTokens,
			ToolChoice:       "auto",
			CacheBreakpoints: r.cfg.CacheBreakpoints,
			Model:            r.cfg.Model,
			ReasoningEffort:  r.cfg.ReasoningEffort,
		}
		resp, err := r.cfg.Provider.Run(ctx, req, sink)
		sink.close()
		if err != nil {
			status, msg, class := classifyProviderError(ctx, err)
			r.cfg.Sink.TrackMessage(msg, "system")
			if class == RetryClassRateLimit {
				proc.SetFinalStatus("RATE_LIMITED")
				return msgs, "RATE_LIMITED"
			}
			if r.cfg.ErrorSvc != nil && status == "FAIL" {
				r.cfg.ErrorSvc.RecordError(proc.ProjectID(), "agent", proc.SessionID(), msg)
			}
			proc.SetFinalStatus(status)
			return msgs, status
		}

		if pct, ok := r.updateContext(ctx, proc, resp.Usage); ok {
			pctLeft, pctKnown = pct, true
		}

		switch resp.StopReason {
		case "end_turn":
			// Do NOT filter resp.Content — thinking blocks must ride along for
			// required API replay, same as the tool_use branch below.
			msgs = append(msgs, provider.Message{Role: "assistant", Content: resp.Content})
			if r.cfg.Observer != nil {
				r.cfg.Observer.OnMessage("assistant", resp.Content)
			}
			proc.SetFinalStatus("PASS")
			return msgs, "PASS"
		case "max_tokens", "stop_sequence":
			r.fail(proc, fmt.Sprintf("stop_reason=%s", resp.StopReason))
			return msgs, "FAIL"
		case "refusal":
			r.fail(proc, "provider refusal")
			return msgs, "FAIL"
		case "tool_use":
			toolResults, terminalStatus := r.dispatchTools(ctx, proc, resp.Content)
			if terminalStatus != "" {
				return msgs, terminalStatus
			}
			if len(toolResults) == 0 {
				r.fail(proc, "tool_use stop_reason but no tool_use blocks in response")
				return msgs, "FAIL"
			}
			// Do NOT filter resp.Content — thinking blocks must ride along for required API replay.
			msgs = append(msgs,
				provider.Message{Role: "assistant", Content: resp.Content},
				provider.Message{Role: "user", Content: toolResults},
			)
			if r.cfg.Observer != nil {
				r.cfg.Observer.OnMessage("assistant", resp.Content)
				r.cfg.Observer.OnMessage("user", toolResults)
			}
			continue
		default:
			r.fail(proc, fmt.Sprintf("unexpected stop_reason=%q", resp.StopReason))
			return msgs, "FAIL"
		}
	}

	r.fail(proc, fmt.Sprintf("max iterations %d reached", r.cfg.MaxIterations))
	return msgs, "FAIL"
}

// toolOutcome is one tool_use block's dispatch result: either a terminal
// signal (result unused) or an assembled tool_result block.
type toolOutcome struct {
	terminal *TerminalSignal
	result   provider.ContentBlock
}

// dispatchTools dispatches the tool_use blocks in resp.Content — concurrently
// (capped at maxParallelToolDispatch) when the model requested more than one —
// and assembles tool_result blocks in the original block order, which is also
// the order results are replayed to the provider. It returns the results plus
// the terminal status a handler signaled via TerminalSignal (empty when no
// handler terminated the loop); with parallel dispatch every block still runs
// to completion, and the first terminal signal in block order wins.
func (r *Runner) dispatchTools(ctx context.Context, proc ProcState, content []provider.ContentBlock) ([]provider.ContentBlock, string) {
	calls := make([]provider.ContentBlock, 0, len(content))
	for _, block := range content {
		if block.Type == "tool_use" {
			calls = append(calls, block)
		}
	}

	outcomes := make([]toolOutcome, len(calls))
	if len(calls) <= 1 {
		for i, block := range calls {
			outcomes[i] = r.invokeTool(ctx, block)
		}
	} else {
		sem := make(chan struct{}, maxParallelToolDispatch)
		var wg sync.WaitGroup
		for i, block := range calls {
			wg.Add(1)
			go func(i int, block provider.ContentBlock) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				outcomes[i] = r.invokeTool(ctx, block)
			}(i, block)
		}
		wg.Wait()
	}

	results := make([]provider.ContentBlock, 0, len(calls))
	for _, oc := range outcomes {
		if oc.terminal != nil {
			proc.SetFinalStatus(oc.terminal.Status)
			if oc.terminal.Status == "CALLBACK" {
				proc.SetCallbackLevel(oc.terminal.Level)
			}
			return nil, oc.terminal.Status
		}
		results = append(results, oc.result)
	}
	return results, ""
}

// invokeTool runs one tool_use block through its handler and returns the
// outcome. Sink calls (TrackMessage/CloseToolSpan) happen here, from the
// dispatching goroutine — MessageSink implementations are concurrency-safe.
func (r *Runner) invokeTool(ctx context.Context, block provider.ContentBlock) toolOutcome {
	handler, ok := r.cfg.Handlers[block.ToolName]
	if !ok {
		msg := fmt.Sprintf("unknown tool: %s", block.ToolName)
		r.cfg.Sink.TrackMessage(msg, "error")
		return toolOutcome{result: provider.ContentBlock{
			Type:      "tool_result",
			ToolUseID: block.ToolUseID,
			Output:    msg,
			IsError:   true,
		}}
	}

	var (
		out   string
		media []provider.MediaBlock
		isErr bool
		terr  error
	)
	if mh, ok := handler.(MediaToolHandler); ok {
		out, media, isErr, terr = mh.InvokeMedia(ctx, r.cfg.Env, block.Input)
	} else {
		out, isErr, terr = handler.Invoke(ctx, r.cfg.Env, block.Input)
	}
	r.cfg.Sink.CloseToolSpan(block.ToolUseID)

	var ts TerminalSignal
	if errors.As(terr, &ts) {
		return toolOutcome{terminal: &ts}
	}
	if terr != nil {
		out = terr.Error()
		isErr = true
		media = nil
	}
	if !isErr {
		out = MaybeOffloadToolResult(ctx, r.cfg.Env, block.ToolName, out)
	}
	category := "tool"
	if isErr {
		category = "error"
	}
	r.cfg.Sink.TrackMessage(formatToolResult(block.ToolName, out, isErr), category)
	return toolOutcome{result: provider.ContentBlock{
		Type:        "tool_result",
		ToolUseID:   block.ToolUseID,
		Output:      out,
		IsError:     isErr,
		OutputMedia: media,
	}}
}

func formatToolResult(name, out string, isErr bool) string {
	const maxOut = 2048
	if isErr {
		return fmt.Sprintf("%s: %s", name, out)
	}
	if len(out) > maxOut {
		out = out[:maxOut]
	}
	return fmt.Sprintf("[%s] → %s", name, out)
}
