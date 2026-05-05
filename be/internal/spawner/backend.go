package spawner

import (
	"bufio"
	"context"
	"fmt"
	"os"
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

// newCLIBackend wraps the existing CLI exec.Cmd flow as an ExecutionBackend.
func newCLIBackend(adapter CLIAdapter, s *Spawner) *cliBackend {
	return &cliBackend{adapter: adapter, s: s}
}

type cliBackend struct {
	adapter CLIAdapter
	s       *Spawner
}

func (b *cliBackend) Name() string                  { return "cli" }
func (b *cliBackend) SupportsResume() bool          { return b.adapter.SupportsResume() }
func (b *cliBackend) SupportsTakeControl() bool     { return b.adapter.SupportsResume() }
func (b *cliBackend) RequiresPrompt() bool          { return true }
func (b *cliBackend) TracksContext() bool           { return true }
func (b *cliBackend) ParsesStructuredOutput() bool  { return true }

// Start builds the exec.Cmd, wires stdin/stdout/stderr pipes, starts the process,
// and launches the output and wait goroutines. Sets proc.cmd and proc.spawnCommand.
func (b *cliBackend) Start(ctx context.Context, proc *processInfo, prep *prepResult) error {
	cmd := b.adapter.BuildCommand(prep.opts)

	removeSuffixFile := func() {
		if prep.suffixFile != "" {
			os.Remove(prep.suffixFile)
		}
	}

	var stdinFile *os.File
	if b.adapter.UsesStdinPrompt() {
		f, err := os.Open(prep.promptFile)
		if err != nil {
			os.Remove(prep.promptFile)
			removeSuffixFile()
			return fmt.Errorf("failed to open prompt file for stdin: %w", err)
		}
		cmd.Stdin = f
		stdinFile = f
	}

	// Capture spawn command for debugging/replay — prepend nrflo env vars
	// so the recorded command is fully reproducible.
	var envParts []string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "NRF_") || strings.HasPrefix(e, "NRFLO_") {
			envParts = append(envParts, e)
		}
	}
	spawnCommand := strings.Join(cmd.Args, " ")
	if b.adapter.UsesStdinPrompt() {
		spawnCommand += " < " + prep.promptFile
	}
	if len(envParts) > 0 {
		spawnCommand = strings.Join(envParts, " ") + " " + spawnCommand
	}
	proc.spawnCommand = spawnCommand

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if stdinFile != nil {
			stdinFile.Close()
		}
		os.Remove(prep.promptFile)
		removeSuffixFile()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if stdinFile != nil {
			stdinFile.Close()
		}
		os.Remove(prep.promptFile)
		removeSuffixFile()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if stdinFile != nil {
			stdinFile.Close()
		}
		os.Remove(prep.promptFile)
		removeSuffixFile()
		return fmt.Errorf("failed to start agent: %w", err)
	}
	if stdinFile != nil {
		stdinFile.Close()
	}

	proc.cmd = cmd

	// Launch output monitoring goroutines.
	go b.s.monitorOutput(proc, stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			b.s.warnAgent(proc, "[stderr] "+line)
			b.s.TrackMessage(proc, "[stderr] "+line, "text")
		}
	}()

	// Wait goroutine — closes doneCh when process exits.
	// Capture doneCh locally: proc.doneCh may be replaced during low-context save.
	origDoneCh := proc.doneCh
	promptPath := prep.promptFile
	suffixPath := prep.suffixFile
	go func() {
		proc.waitErr = cmd.Wait()
		close(origDoneCh)
		os.Remove(promptPath)
		if suffixPath != "" {
			os.Remove(suffixPath)
		}
	}()

	return nil
}

// Kill sends a signal to the running process. SIGKILL routes to Process.Kill();
// other signals route to Process.Signal. Nil-Process is silently ignored to
// preserve the previous safe-kill semantics at all call sites.
func (b *cliBackend) Kill(ctx context.Context, proc *processInfo, sig syscall.Signal) error {
	if proc.cmd == nil || proc.cmd.Process == nil {
		return nil
	}
	if sig == syscall.SIGKILL {
		return proc.cmd.Process.Kill()
	}
	return proc.cmd.Process.Signal(sig)
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
