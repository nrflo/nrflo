package apirun

import (
	"context"
	"time"

	"be/internal/logger"
	"be/internal/spawner/apirun/provider"
)

// defaultMaxIterations is the loop bound when the agent definition does not
// specify api_max_iterations.
const defaultMaxIterations = 50

// defaultMaxTokens is the per-turn output cap when the spawner doesn't supply
// one. Per-agent overrides come from agent_definitions.api_max_tokens.
const defaultMaxTokens = 16384

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
	ReasoningEffort  string
	CaptureThinking  bool
	// CompactPct is the in-loop compaction threshold: when a turn reports
	// context-left at or below this %, runTurns summarizes the history before
	// the next request (runner_compact.go). 0 applies compactThresholdPct.
	// The spawner passes restartThreshold+5 for autonomous agents so an
	// in-process compaction preempts the kill+saver+relaunch dance.
	CompactPct int
	// Stream receives raw text/thinking deltas as they arrive, before the
	// runner sink's chunked buffering. Nil for autonomous agents (Run); a
	// console chat engine (Conversation) passes a live consumer.
	Stream StreamHook
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
	if cfg.CompactPct <= 0 {
		cfg.CompactPct = compactThresholdPct
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

	r.runTurns(ctx, proc, msgs)
}

// updateContext computes the percentage of context window remaining from the
// turn's Usage and writes it to proc + AgentSvc so monitorAll observes the
// same low-context threshold path used by CLI agents. It also emits a
// structured per-turn usage line (the only place cache_read/cache_creation are
// surfaced — they are otherwise summed away into the context-left %). Returns
// (pct, true) when a percentage was computed — runTurns feeds it into the
// in-loop compaction check.
func (r *Runner) updateContext(ctx context.Context, proc ProcState, u provider.Usage) (int, bool) {
	total := u.InputTokens + u.CacheReadTokens + u.CacheCreationTokens
	if total <= 0 {
		return 0, false
	}
	// cache_read = reused prefix, cache_creation = fresh write, input = uncached.
	// cache_hit_pct is the share of billed input served from cache this turn.
	cacheStatus := "miss"
	if u.CacheReadTokens > 0 {
		cacheStatus = "hit"
	}
	logger.Info(ctx, "apirun turn usage",
		"session", proc.SessionID(),
		"model", r.cfg.Model,
		"input", u.InputTokens,
		"cache_read", u.CacheReadTokens,
		"cache_creation", u.CacheCreationTokens,
		"output", u.OutputTokens,
		"cache", cacheStatus,
		"cache_hit_pct", 100*u.CacheReadTokens/total,
	)
	if r.cfg.MaxContext <= 0 {
		return 0, false
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
	return pct, true
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
