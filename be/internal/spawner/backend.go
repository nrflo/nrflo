package spawner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// ExecutionBackend abstracts how an agent process is started, signaled, and tracked.
// CLI agents use cliBackend (current behavior). API agents (T2-T5) plug in their own
// backend without touching monitorAll.
type ExecutionBackend interface {
	Name() string
	SupportsResume() bool
	SupportsTakeControl() bool
	// RequiresPrompt reports whether the backend requires a rendered prompt template.
	RequiresPrompt() bool
	// TracksContext reports whether monitorAll should trigger low-context save for this backend.
	TracksContext() bool
	// ParsesStructuredOutput reports whether processOutput should parse stdout as JSON.
	// When false, each stdout line is tracked as a "text" message directly.
	ParsesStructuredOutput() bool
	Start(ctx context.Context, proc *processInfo, prep *prepResult) error
	Kill(ctx context.Context, proc *processInfo, sig syscall.Signal) error
}

// prepResult holds CLI-agnostic prep state produced by prepareSpawn and consumed
// by ExecutionBackend.Start. It carries the resolved CLI adapter (CLI mode only;
// nil for api mode), the prompt file path, and the assembled SpawnOptions.
//
// For api mode, adapter is nil and the api* fields are populated.
// For script mode, scriptCode/scriptID are populated and CLI/API fields are empty.
type prepResult struct {
	adapter    CLIAdapter
	cliName    string
	opts       SpawnOptions
	promptFile string
	suffixFile string // temp file for --append-system-prompt-file (Claude only); empty when unused
	prompt     string
	phase      string

	// Script-mode fields.
	scriptCode string
	scriptID   string
	pythonPath string // resolved venv python; empty → fall back to "python3" on PATH

	// API-mode fields. executionMode is "cli" (default), "api", or "script".
	executionMode    string
	apiSystem        string
	apiInitialPrompt string
	apiTools         []provider.ToolSpec
	apiHandlers      apirun.Registry
	apiToolEnv       apirun.ToolEnv
	apiMaxIterations int
	apiMaxTokens     int
	apiDeadline      time.Time
	apiModelID       string // mapped model name, e.g. "claude-haiku-4-5-20251001"
	apiMaxContext    int
}

// apiBackend executes an agent in-process via the apirun.Runner. There is no
// child process — Start launches a goroutine driving the runner, and Kill
// cancels the goroutine's context.
type apiBackend struct {
	s        *Spawner
	provider provider.Provider
	agentSvc apirun.AgentSvc

	mu     sync.Mutex
	cancel context.CancelFunc
}

func newAPIBackend(s *Spawner) *apiBackend {
	return &apiBackend{
		s:        s,
		provider: s.config.Provider,
		agentSvc: s.config.AgentSvc,
	}
}

func (b *apiBackend) Name() string                  { return "api" }
func (b *apiBackend) SupportsResume() bool          { return false }
func (b *apiBackend) SupportsTakeControl() bool     { return false }
func (b *apiBackend) RequiresPrompt() bool          { return true }
func (b *apiBackend) TracksContext() bool           { return true }
func (b *apiBackend) ParsesStructuredOutput() bool  { return false }

// Start launches the runner goroutine. The goroutine flushes messages and
// registers the session stop before closing proc.doneCh, so monitorAll can
// skip handleCompletion (proc.cmd is nil for api agents).
func (b *apiBackend) Start(ctx context.Context, proc *processInfo, prep *prepResult) error {
	if b.provider == nil {
		return fmt.Errorf("api backend: spawner.Config.Provider is nil")
	}
	if b.agentSvc == nil {
		return fmt.Errorf("api backend: spawner.Config.AgentSvc is nil")
	}

	// Build a synthetic spawn command for forensics/UI display.
	proc.spawnCommand = fmt.Sprintf("api:%s model=%s max_iter=%d max_tokens=%d",
		b.provider.Name(), prep.apiModelID, prep.apiMaxIterations, prep.apiMaxTokens)

	runCtx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()

	sink := &procMessageSink{s: b.s, proc: proc}
	procState := &procStateAdapter{proc: proc}

	runner := apirun.NewRunner(apirun.Config{
		Provider:      b.provider,
		Sink:          sink,
		AgentSvc:      b.agentSvc,
		ErrorSvc:      apirunErrorAdapter(b.s.config.ErrorSvc),
		System:        prep.apiSystem,
		InitialPrompt: prep.apiInitialPrompt,
		Tools:         prep.apiTools,
		Handlers:      prep.apiHandlers,
		Env:           prep.apiToolEnv,
		Model:         prep.apiModelID,
		MaxIterations: prep.apiMaxIterations,
		MaxTokens:     prep.apiMaxTokens,
		MaxContext:    prep.apiMaxContext,
		Deadline:      prep.apiDeadline,
	})

	doneCh := proc.doneCh
	go func() {
		defer close(doneCh)
		defer cancel()
		runner.Run(runCtx, procState)

		// Persist messages and register the session stop ourselves — monitorAll
		// will see finalStatus already set and skip handleCompletion (which
		// would dereference proc.cmd).
		b.s.saveMessages(proc)
		result, reason := mapFinalStatus(proc.finalStatus)
		b.s.registerAgentStopWithReason(proc.projectID, proc.ticketID, proc.workflowName,
			proc.sessionID, proc.agentID, result, reason, proc.modelID)

		logCtx := logger.WithTrx(context.Background(), proc.trx)
		logger.Info(logCtx, "api agent finished", "model", proc.modelID, "status", proc.finalStatus, "session_id", proc.sessionID)
	}()

	return nil
}

// Kill cancels the runner context. Signal is ignored (no process to signal).
func (b *apiBackend) Kill(ctx context.Context, proc *processInfo, sig syscall.Signal) error {
	b.mu.Lock()
	cancel := b.cancel
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// procMessageSink adapts *processInfo to apirun.MessageSink by capturing the
// proc reference and delegating to Spawner.TrackMessage.
type procMessageSink struct {
	s    *Spawner
	proc *processInfo
}

func (p *procMessageSink) TrackMessage(content, category string) {
	p.s.TrackMessage(p.proc, content, category)
}

// procStateAdapter wraps *processInfo with the apirun.ProcState surface.
type procStateAdapter struct {
	proc *processInfo
}

func (p *procStateAdapter) SessionID() string          { return p.proc.sessionID }
func (p *procStateAdapter) ProjectID() string          { return p.proc.projectID }
func (p *procStateAdapter) WorkflowInstanceID() string { return p.proc.workflowInstanceID }
func (p *procStateAdapter) SetFinalStatus(s string)    { p.proc.finalStatus = s }
func (p *procStateAdapter) SetContextLeft(pct int)     { p.proc.contextLeft = pct }
func (p *procStateAdapter) SetCallbackLevel(level int) { p.proc.callbackLevel = level }

// apirunErrorAdapter converts a spawner.ErrorRecorder into apirun.ErrorRecorder.
// Returns nil when the input is nil so the runner skips error recording.
func apirunErrorAdapter(rec ErrorRecorder) apirun.ErrorRecorder {
	if rec == nil {
		return nil
	}
	return errorRecAdapter{r: rec}
}

type errorRecAdapter struct {
	r ErrorRecorder
}

func (e errorRecAdapter) RecordError(projectID, errorType, instanceID, message string) error {
	return e.r.RecordError(projectID, errorType, instanceID, message)
}

// mapFinalStatus translates the runner's terminal state into the
// (result, reason) pair used by registerAgentStopWithReason.
func mapFinalStatus(status string) (result, reason string) {
	switch status {
	case "PASS":
		return "pass", "implicit"
	case "CANCELLED":
		return "fail", "cancelled"
	case "FAIL":
		return "fail", "api_error"
	case "CONTINUE":
		return "continue", "api_continue"
	case "CALLBACK":
		return "callback", "callback"
	case "":
		return "fail", "no_status"
	default:
		return "fail", strings.ToLower(status)
	}
}
