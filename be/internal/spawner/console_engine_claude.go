package spawner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"be/internal/model"
	ptyPkg "be/internal/pty"
)

// claudeSessionStartTimeout/claudeBootstrapFloor bound SendUserTurn's wait
// for the TUI to be ready before writing the turn body: SessionStart (a hook
// notification relayed by ConsoleHub.ConsoleSessionReady) proves the TUI has
// begun bootstrapping; the bootstrap floor gives its input loop a moment to
// finish painting before a submit CR is honored, and so applies only to the
// FIRST turn. claudeSubmitDelay separates the body write from the submit CR
// (same 150ms deliverPrompt uses): coalesced into one PTY read, the TUI can
// swallow the CR and the turn is typed but never submitted. All are
// injectable engine fields (defaulted in newClaudeEngine) so tests use 0
// instead of sleeping.
const (
	claudeSessionStartTimeout = 20 * time.Second
	claudeBootstrapFloor      = 1500 * time.Millisecond
	claudeSubmitDelay         = 150 * time.Millisecond
)

// claudeEngine drives a human-attended console session over the existing PTY
// + Claude-hooks path — no headless -p/stream-json, so console sessions stay
// on subscription pricing. Unlike codexEngine (app-server JSON-RPC) it holds
// no client connection: the PTY carries bytes the engine drops (claude's
// heartbeat and turn boundaries come from hooks, not PTY output), and the
// hooks reach the engine only indirectly, via ConsoleHub notifying the
// registered consoleTarget methods. Like codexEngine it holds no
// *processInfo, so it is structurally exempt from the autonomous
// nudge/stall/restart-cap policies.
type claudeEngine struct {
	sink      Sink
	hub       *ConsoleHub
	nrfloPath string
	pty       ptyManagerIface

	mu               sync.Mutex
	spec             EngineSpec
	cancel           context.CancelFunc
	ptySession       ptySessionIface
	tempDir          string
	turnActive       bool
	turnTextSeen     bool
	bootstrapped     bool
	transcriptOffset int64

	// flushMu serializes flushTranscript (ticker + hook goroutines race).
	flushMu sync.Mutex

	// pendingEcho (mu-guarded) is the last SendUserTurn text awaiting its
	// UserPromptSubmit hook echo — see NotifyUserPrompt.
	pendingEcho string

	// viewer (viewerMu-guarded) is the attached raw-terminal sink, nil when
	// no terminal is attached — see console_engine_claude_viewer.go.
	viewerMu sync.Mutex
	viewer   *consoleViewer

	readyCh   chan struct{}
	readyOnce sync.Once

	events       chan EngineEvent
	ferryDone    chan struct{}
	tailDone     chan struct{}
	stopping     chan struct{}
	ferryOnce    sync.Once
	tailOnce     sync.Once
	stopOnce     sync.Once
	stoppingOnce sync.Once

	// stopped + emitWG (mu-guarded) give Stop exclusive ownership of
	// close(events): emit Adds to emitWG only while !stopped, under mu.
	stopped bool
	emitWG  sync.WaitGroup

	approvals *claudeApprovals

	// Injectable timeouts (tests override the zero-value defaults below).
	approvalTimeout     time.Duration
	sessionStartTimeout time.Duration
	bootstrapFloor      time.Duration
	submitDelay         time.Duration
	tailInterval        time.Duration
}

func newClaudeEngine(deps EngineDeps) *claudeEngine {
	return &claudeEngine{
		sink:                deps.Sink,
		hub:                 deps.Hub,
		nrfloPath:           deps.NrfloPath,
		pty:                 wrapPtyManager(deps.PTY),
		events:              make(chan EngineEvent, 256),
		ferryDone:           make(chan struct{}),
		tailDone:            make(chan struct{}),
		stopping:            make(chan struct{}),
		readyCh:             make(chan struct{}),
		approvals:           newClaudeApprovals(),
		approvalTimeout:     consoleApprovalTimeout,
		sessionStartTimeout: claudeSessionStartTimeout,
		bootstrapFloor:      claudeBootstrapFloor,
		submitDelay:         claudeSubmitDelay,
		tailInterval:        claudeTranscriptTailInterval,
	}
}

func (e *claudeEngine) Name() string { return "claude" }

// Start writes the per-session --mcp-config file, builds the console argv (no
// --dangerously-skip-permissions, --disallowedTools, --strict-mcp-config, or
// safety-hook merge — this is a human-driven console conversation), registers
// + creates the PTY session under
// spec.SessionID (also a valid `claude --session-id` value), registers with
// the Hub, and starts the PTY ferry + transcript tailer goroutines.
func (e *claudeEngine) Start(ctx context.Context, spec EngineSpec) error {
	if e.pty == nil {
		return fmt.Errorf("claude console engine: no PTY manager configured")
	}

	tempDir, err := os.MkdirTemp("", "nrflo-console-claude-engine-"+spec.SessionID+"-*")
	if err != nil {
		return fmt.Errorf("claude console engine: mkdir temp: %w", err)
	}

	mcpPath, err := WriteConsoleClaudeMCPConfig(tempDir, e.nrfloPath, []string{"agent", "mcp-external"}, spec.MCPEnv)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return fmt.Errorf("claude console engine: write mcp config: %w", err)
	}

	args := []string{"--session-id", spec.effectiveCLISessionID()}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.ReasoningEffort != "" {
		args = append(args, "--effort", spec.ReasoningEffort)
	}
	if spec.FallbackModels != "" {
		args = append(args, "--fallback-model", spec.FallbackModels)
	}
	if spec.NativeToolsCSV == model.NativeToolsNone {
		// Sentinel: disable every native tool (MCP-only chat). Mirrors
		// cli_adapter_claude.go's autonomous-spawn precedent — an empty
		// --tools value means "no built-in tools".
		args = append(args, "--tools", "")
	} else if spec.NativeToolsCSV != "" {
		args = append(args, "--tools", spec.NativeToolsCSV)
	}
	if spec.SystemPrompt != "" {
		sysPromptPath := filepath.Join(tempDir, "system-prompt.md")
		if err := os.WriteFile(sysPromptPath, []byte(spec.SystemPrompt), 0o600); err != nil {
			_ = os.RemoveAll(tempDir)
			return fmt.Errorf("claude console engine: write system prompt file: %w", err)
		}
		args = append(args, "--system-prompt-file", sysPromptPath)
	}
	args = append(args, "--settings", BuildConsoleSettingsJSON(e.nrfloPath), "--mcp-config", mcpPath)

	e.pty.RegisterLaunch(spec.SessionID, ptyPkg.Launch{
		Command: "claude",
		Args:    args,
		Env:     spec.Env,
		Dir:     spec.WorkDir,
	})
	sess, err := e.pty.Create(spec.SessionID, spec.WorkDir, spec.Env)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return fmt.Errorf("claude console engine: create PTY session: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)

	e.mu.Lock()
	e.spec = spec
	e.cancel = cancel
	e.ptySession = sess
	e.tempDir = tempDir
	e.mu.Unlock()

	if e.hub != nil {
		e.hub.Register(spec.SessionID, e)
	}

	go e.ferry(sess)
	go e.tailLoop(runCtx)

	return nil
}

// ferry reads and drops PTY output until the session closes — claude's
// heartbeat and turn boundaries come from hooks, not PTY bytes. A read error
// while Stop has NOT been requested means the CLI process died on its own:
// no Stop hook will ever arrive, so a turn in flight would stay pinned
// forever. Emit an EventError so the consumer can end the turn and surface
// the death. (It does not close Events: tailLoop is still emitting on its own
// goroutine; Stop owns that close.)
func (e *claudeEngine) ferry(sess ptySessionIface) {
	defer e.ferryOnce.Do(func() { close(e.ferryDone) })
	buf := make([]byte, 4096)
	for {
		n, err := sess.Read(buf)
		if n > 0 {
			e.forwardToViewer(buf[:n])
		}
		if err != nil {
			select {
			case <-e.stopping:
			default:
				e.emit(EngineEvent{
					Type:      EventError,
					SessionID: e.sessionID(),
					Text:      "claude console session ended unexpectedly",
					IsError:   true,
				})
			}
			return
		}
	}
}

// sessionID returns the spec's session id under the lock.
func (e *claudeEngine) sessionID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spec.SessionID
}

// Events returns the normalized event channel, closed when Stop completes.
func (e *claudeEngine) Events() <-chan EngineEvent { return e.events }

// Stop tears down the engine: unregisters from the Hub, cancels the tailer's
// context, closes the PTY session, waits for the ferry + tailer goroutines to
// exit, drains in-flight emits, removes the temp dir, and closes Events
// exactly once. `stopping` closes BEFORE any of that so a blocked approval or
// a non-draining Events consumer cannot wedge this teardown; `stopped` is set
// under the same lock emit checks, giving Stop exclusive ownership of the
// close (same discipline as codexEngine.Stop / apiConsoleEngine.Stop). cancel
// is non-nil only once Start has committed to launching the ferry + tailer
// goroutines, so it also gates the done-channel waits.
func (e *claudeEngine) Stop() {
	e.stoppingOnce.Do(func() { close(e.stopping) })

	e.mu.Lock()
	sessionID := e.spec.SessionID
	cancel, sess, tempDir := e.cancel, e.ptySession, e.tempDir
	e.stopped = true
	e.mu.Unlock()

	if e.hub != nil && sessionID != "" {
		e.hub.Unregister(sessionID)
	}
	if cancel != nil {
		cancel()
		if sess != nil {
			_ = sess.Close()
			<-e.ferryDone
		}
		<-e.tailDone
	}
	if tempDir != "" {
		_ = os.RemoveAll(tempDir)
	}
	// stopped==true blocks new emitWG.Add calls, so this drains in-flight emits.
	e.emitWG.Wait()
	e.stopOnce.Do(func() { close(e.events) })
}

// emit delivers one EngineEvent to Events, returning immediately once Stop
// has set stopped (no send can race the close), else abandoning the send
// once stopping closes (a non-draining consumer can't wedge the caller).
func (e *claudeEngine) emit(ev EngineEvent) {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}
	e.emitWG.Add(1)
	e.mu.Unlock()
	defer e.emitWG.Done()

	select {
	case e.events <- ev:
	case <-e.stopping:
	}
}
