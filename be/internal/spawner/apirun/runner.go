package apirun

import (
	"context"
	"errors"
	"fmt"
	"time"

	"be/internal/spawner/apirun/provider"
)

// defaultMaxIterations is the loop bound when the agent definition does not
// specify api_max_iterations.
const defaultMaxIterations = 50

// defaultMaxTokens is the per-turn output cap. T4 may make this configurable.
const defaultMaxTokens = 4096

// Config carries the runner's per-spawn configuration. All fields are
// populated by the spawner in prepareSpawn.
type Config struct {
	Provider         provider.Provider
	Sink             MessageSink
	AgentSvc         AgentSvc
	ErrorSvc         ErrorRecorder
	System           string
	InitialPrompt    string
	Tools            []provider.ToolSpec
	Handlers         Registry
	Env              ToolEnv
	CacheBreakpoints []provider.CacheBreakpoint
	Model            string
	MaxIterations    int
	MaxTokens        int
	MaxContext       int
	Deadline         time.Time
}

// Runner drives an API-mode agent through one or more turns. Each Runner
// instance is single-shot — after Run returns, finalStatus is set on the
// proc and the run is complete.
type Runner struct {
	cfg Config
}

// NewRunner constructs a Runner from cfg. Defaults are applied for
// MaxIterations and MaxTokens when zero.
func NewRunner(cfg Config) *Runner {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = defaultMaxIterations
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = defaultMaxTokens
	}
	if cfg.MaxContext <= 0 {
		cfg.MaxContext = 200000
	}
	return &Runner{cfg: cfg}
}

// Run executes the loop until a terminal state is reached. On exit it sets
// proc.FinalStatus; the caller is responsible for closing proc's done
// channel and persisting messages/sessions.
func (r *Runner) Run(ctx context.Context, proc ProcState) {
	if r.cfg.Provider == nil {
		r.fail(proc, "provider not configured")
		return
	}
	if r.cfg.Sink == nil {
		// Without a sink we cannot report messages — bail with FAIL state but
		// no message (caller still has the proc state to act on).
		proc.SetFinalStatus("FAIL")
		return
	}

	msgs := []provider.Message{
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "text", Text: r.cfg.InitialPrompt},
			},
		},
	}

	for turn := 0; turn < r.cfg.MaxIterations; turn++ {
		if ctx.Err() != nil {
			proc.SetFinalStatus("CANCELLED")
			return
		}
		if !r.cfg.Deadline.IsZero() && !time.Now().Before(r.cfg.Deadline) {
			r.fail(proc, fmt.Sprintf("deadline exceeded (%s)", r.cfg.Deadline.Format(time.RFC3339)))
			return
		}

		sink := newRunnerSink(r.cfg.Sink)
		req := provider.Request{
			System:           r.cfg.System,
			Messages:         msgs,
			Tools:            r.cfg.Tools,
			MaxTokens:        r.cfg.MaxTokens,
			ToolChoice:       "auto",
			CacheBreakpoints: r.cfg.CacheBreakpoints,
			Model:            r.cfg.Model,
		}
		resp, err := r.cfg.Provider.Run(ctx, req, sink)
		sink.close()
		if err != nil {
			status, msg, class := classifyProviderError(ctx, err)
			r.cfg.Sink.TrackMessage(msg, "system")
			if class == RetryClassRateLimit {
				proc.SetFinalStatus("RATE_LIMITED")
				return
			}
			if r.cfg.ErrorSvc != nil && status == "FAIL" {
				r.cfg.ErrorSvc.RecordError(proc.ProjectID(), "agent", proc.SessionID(), msg)
			}
			proc.SetFinalStatus(status)
			return
		}

		r.updateContext(proc, resp.Usage)

		switch resp.StopReason {
		case "end_turn":
			proc.SetFinalStatus("PASS")
			return
		case "max_tokens", "stop_sequence":
			r.fail(proc, fmt.Sprintf("stop_reason=%s", resp.StopReason))
			return
		case "tool_use":
			toolResults, terminate := r.dispatchTools(ctx, proc, resp.Content)
			if terminate {
				return
			}
			if len(toolResults) == 0 {
				r.fail(proc, "tool_use stop_reason but no tool_use blocks in response")
				return
			}
			msgs = append(msgs,
				provider.Message{Role: "assistant", Content: resp.Content},
				provider.Message{Role: "user", Content: toolResults},
			)
			continue
		default:
			r.fail(proc, fmt.Sprintf("unexpected stop_reason=%q", resp.StopReason))
			return
		}
	}

	r.fail(proc, fmt.Sprintf("max iterations %d reached", r.cfg.MaxIterations))
}

// dispatchTools iterates tool_use blocks in resp.Content sequentially. It
// returns the assembled tool_result blocks plus a terminate signal when a
// handler emits a TerminalSignal (FAIL/CONTINUE/CALLBACK). Sequential
// dispatch only in v1 — TODO(parallel): the for-range loop below is the
// natural slot for parallel dispatch.
func (r *Runner) dispatchTools(ctx context.Context, proc ProcState, content []provider.ContentBlock) ([]provider.ContentBlock, bool) {
	results := []provider.ContentBlock{}
	for _, block := range content {
		if block.Type != "tool_use" {
			continue
		}
		handler, ok := r.cfg.Handlers[block.ToolName]
		if !ok {
			msg := fmt.Sprintf("unknown tool: %s", block.ToolName)
			r.cfg.Sink.TrackMessage(msg, "tool_error")
			results = append(results, provider.ContentBlock{
				Type:      "tool_result",
				ToolUseID: block.ToolUseID,
				Output:    msg,
				IsError:   true,
			})
			continue
		}

		out, isErr, terr := handler.Invoke(ctx, r.cfg.Env, block.Input)

		var ts TerminalSignal
		if errors.As(terr, &ts) {
			proc.SetFinalStatus(ts.Status)
			if ts.Status == "CALLBACK" {
				proc.SetCallbackLevel(ts.Level)
			}
			return nil, true
		}
		if terr != nil {
			out = terr.Error()
			isErr = true
		}
		r.cfg.Sink.TrackMessage(formatToolResult(block.ToolName, out, isErr), "tool_result")
		results = append(results, provider.ContentBlock{
			Type:      "tool_result",
			ToolUseID: block.ToolUseID,
			Output:    out,
			IsError:   isErr,
		})
	}
	return results, false
}

func formatToolResult(name, out string, isErr bool) string {
	if isErr {
		return fmt.Sprintf("[tool_result:error] name=%s output=%s", name, out)
	}
	return fmt.Sprintf("[tool_result] name=%s output=%s", name, out)
}

// updateContext computes the percentage of context window remaining from the
// turn's Usage and writes it to proc + AgentSvc so monitorAll observes the
// same low-context threshold path used by CLI agents.
func (r *Runner) updateContext(proc ProcState, u provider.Usage) {
	total := u.InputTokens + u.CacheReadTokens + u.CacheCreationTokens
	if total <= 0 || r.cfg.MaxContext <= 0 {
		return
	}
	pct := 100 - (100*total)/r.cfg.MaxContext
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	proc.SetContextLeft(pct)
	if r.cfg.AgentSvc != nil {
		r.cfg.AgentSvc.UpdateContextLeft(proc.SessionID(), pct)
	}
}

// fail emits a system message and marks the proc as FAIL. Also records the
// error via ErrorSvc when configured.
func (r *Runner) fail(proc ProcState, msg string) {
	if r.cfg.Sink != nil {
		r.cfg.Sink.TrackMessage(msg, "system")
	}
	if r.cfg.ErrorSvc != nil {
		r.cfg.ErrorSvc.RecordError(proc.ProjectID(), "agent", proc.SessionID(), msg)
	}
	proc.SetFinalStatus("FAIL")
}
