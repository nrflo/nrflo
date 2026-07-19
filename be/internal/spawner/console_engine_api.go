package spawner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// consoleAPISystem is the system prompt for the api console engine: an
// nrflo-control + web-research assistant with no native file/edit/bash tools
// — its only capabilities are the console tool profile (workflow control,
// findings, project/ticket, web_search/web_fetch).
const consoleAPISystem = `You are nrflo's console assistant, reached over a direct API connection with no local CLI. You help the user drive nrflo workflows, inspect projects/tickets, and research topics via web_search/web_fetch. You have NO file, edit, or shell/bash tools: you cannot read or write files on the user's machine and cannot execute commands. Use the tools available to you to answer the user's requests.`

// newConsoleAPIProvider is a test seam (the same package-level factory idiom as
// lookPath/dialAppServer) so tests can inject a fake provider without a
// network call or real credentials.
var newConsoleAPIProvider = service.BuildAPIProvider

// apiConsoleEngine drives an in-process apirun.Conversation for a human
// console chat session: no CLI process, no PTY. Approvals exist only for the
// gated native fs tools (console_engine_api_approval.go); every other
// console-profile tool dispatches unprompted. Like codexEngine/claudeEngine it
// holds no processInfo, so it is structurally exempt from the autonomous
// nudge/stall/restart-cap policies — that requirement is structural, not a
// flag, the same as the other two engines.
type apiConsoleEngine struct {
	sink Sink
	api  APIEngineDeps

	mu                sync.Mutex
	spec              EngineSpec
	conv              *apirun.Conversation
	cancel            context.CancelFunc
	runCtx            context.Context
	turnActive        bool
	turnCancel        context.CancelFunc
	stopped           bool
	lastTurnStatus    string
	lastCallbackLevel int
	turnWG            sync.WaitGroup

	events       chan EngineEvent
	stopping     chan struct{}
	stopOnce     sync.Once
	stoppingOnce sync.Once

	// rateLimitBackoff drives the bounded in-turn retry after a RATE_LIMITED
	// turn (one entry per retry). Injectable so tests use tiny delays.
	rateLimitBackoff []time.Duration

	// approvals gates the mutating native fs tools (edit_file/bash) behind a
	// human decision; approvalTimeout is injectable for tests.
	approvals       *apiEngineApprovals
	approvalTimeout time.Duration
}

func newAPIConsoleEngine(deps EngineDeps) *apiConsoleEngine {
	return &apiConsoleEngine{
		sink:             deps.Sink,
		api:              deps.API,
		events:           make(chan EngineEvent, 256),
		stopping:         make(chan struct{}),
		rateLimitBackoff: []time.Duration{30 * time.Second, 60 * time.Second},
		approvals:        newAPIEngineApprovals(),
		approvalTimeout:  consoleApprovalTimeout,
	}
}

func (e *apiConsoleEngine) Name() string { return "api" }

// Start gates api_mode_enabled (mirrors spawner_prepare.go's autonomous
// api_mode_disabled gate — a runtime toggle read fresh here, not at server
// boot), resolves the provider, and builds the Conversation. No process, no
// PTY, no profile dir to write — turns run as goroutines on runCtx.
func (e *apiConsoleEngine) Start(ctx context.Context, spec EngineSpec) error {
	settingsSvc := service.NewGlobalSettingsService(e.api.Pool, e.api.Clock)
	enabled, _ := settingsSvc.Get("api_mode_enabled")
	if enabled != "true" {
		return service.ErrAPIModeDisabled
	}

	prov, err := newConsoleAPIProvider(ctx, e.api.Pool, e.api.Clock, spec.APIProvider, spec.ProjectID)
	if err != nil {
		return fmt.Errorf("api console engine: %w", err)
	}
	captureThinking, _ := settingsSvc.GetCaptureThinkingEnabled(spec.ProjectID)

	runCtx, cancel := context.WithCancel(ctx)

	e.mu.Lock()
	e.spec = spec
	e.runCtx = runCtx
	e.cancel = cancel
	e.mu.Unlock()

	// Native fs tools (read_file/edit_file/bash) join the console profile
	// when the api_native_tools_enabled global setting is on and the chat has
	// a workdir; the mutating ones are approval-gated
	// (console_engine_api_approval.go). The system prompt fallback swaps to
	// the variant that stops claiming "no local tools".
	tools, handlers, env := e.api.Tools, e.api.Handlers, e.api.ToolEnv
	fallback := consoleAPISystem
	if spec.WorkDir != "" && apiNativeToolsEnabled(e.api.Pool, e.api.Clock) {
		tools, handlers = e.withFSTools(tools, handlers)
		env.WorkDir = spec.WorkDir
		fallback = consoleAPIFSSystem
	}
	sysVars := map[string]string{"PROJECT_ID": spec.ProjectID, "MODEL": spec.Model, "NODE_ID": spec.SessionID}
	system := renderAPISystemPrompt(runCtx, e.api.Pool, "api-console-system-prompt", sysVars, fallback)

	e.conv = apirun.NewConversation(apirun.Config{
		Provider: prov,
		Sink:     &apiEngineSink{sessionID: spec.SessionID, sink: e.sink, pool: e.api.Pool, clock: e.api.Clock},
		AgentSvc: e.sink,
		System:   system,
		Tools:    tools,
		Handlers: handlers,
		Env:      env,
		// Prompt caching mirrors the autonomous api backend (backend.go): one
		// marker on the system block (also caches tool definitions) plus a
		// sliding marker on the conversation tail each turn.
		CacheBreakpoints: []provider.CacheBreakpoint{
			{Target: provider.CacheTargetSystem},
			{Target: provider.CacheTargetMessage},
		},
		Model:           spec.Model,
		MaxContext:      spec.MaxContext,
		ReasoningEffort: spec.ReasoningEffort,
		CaptureThinking: captureThinking,
		Stream:          &apiEngineStream{e: e},
	})

	return nil
}

// SendUserTurn persists the user_input row BEFORE starting the turn goroutine
// — same ordering rationale as codexEngine (console_engine_codex.go:156-160)
// — emits turn_started, runs the shared tool-use loop on e.runCtx, then emits
// turn_completed (PASS) or error (anything else).
func (e *apiConsoleEngine) SendUserTurn(ctx context.Context, text string) error {
	e.mu.Lock()
	// A turn must never start once Stop has begun: Stop closes e.events after
	// turnWG.Wait(), so a turnWG.Add racing that Wait is both WaitGroup misuse
	// and a send on a closed channel from the emit below.
	if e.stopped {
		e.mu.Unlock()
		return ErrEngineStopped
	}
	if e.turnActive {
		e.mu.Unlock()
		return ErrTurnActive
	}
	conv, runCtx, spec := e.conv, e.runCtx, e.spec
	if conv == nil || runCtx == nil {
		e.mu.Unlock()
		return fmt.Errorf("api console engine: not started")
	}
	e.turnActive = true
	turnCtx, turnCancel := context.WithCancel(runCtx)
	e.turnCancel = turnCancel
	e.turnWG.Add(1)
	e.mu.Unlock()

	emitMessage(spec.SessionID, text, "user_input", e.sink)
	e.emit(EngineEvent{Type: EventTurnStarted, SessionID: spec.SessionID})

	go func() {
		defer e.turnWG.Done()
		proc := &apiEngineProcState{e: e}
		status := conv.SendTurn(turnCtx, proc, text)

		// Bounded in-turn retry on rate limits: unlike autonomous api agents
		// (which relaunch via the spawner's backoff dance), a console chat
		// has no relauncher — without this a 429 just ends the turn with an
		// error. The user text is already in the history, so retries resume
		// rather than re-send.
		for i := 0; status == "RATE_LIMITED" && i < len(e.rateLimitBackoff); i++ {
			delay := e.rateLimitBackoff[i]
			emitMessage(spec.SessionID, fmt.Sprintf("provider rate-limited — retrying in %s (attempt %d/%d)", delay, i+1, len(e.rateLimitBackoff)), "system", e.sink)
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				status = conv.ResumeTurn(turnCtx, proc)
			case <-turnCtx.Done():
				timer.Stop()
				status = "CANCELLED"
			case <-e.stopping:
				timer.Stop()
				status = "CANCELLED"
			}
		}

		e.mu.Lock()
		e.turnActive = false
		e.turnCancel = nil
		e.mu.Unlock()

		if status == "PASS" {
			e.emit(EngineEvent{Type: EventTurnCompleted, SessionID: spec.SessionID})
			return
		}
		if status == "CANCELLED" {
			e.emit(EngineEvent{Type: EventTurnCompleted, SessionID: spec.SessionID})
			return
		}
		e.emit(EngineEvent{Type: EventError, SessionID: spec.SessionID, Text: fmt.Sprintf("turn ended: %s", status), IsError: true})
	}()

	return nil
}

// InterruptTurn cancels only the current Conversation.SendTurn call; the
// session-level context and accumulated conversation remain live.
func (e *apiConsoleEngine) InterruptTurn(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.turnActive || e.turnCancel == nil {
		return ErrNoActiveTurn
	}
	e.turnCancel()
	return nil
}

// Events returns the normalized event channel, closed when Stop completes.
func (e *apiConsoleEngine) Events() <-chan EngineEvent { return e.events }

// Stop cancels runCtx, waits for any in-flight turn goroutine to finish, and
// closes Events exactly once. `stopping` is closed BEFORE waiting so emit can
// always unwind (codexEngine's Stop, console_engine_codex.go:185-201): a
// caller that stops mid-drain must never deadlock a turn blocked on a full
// events buffer.
func (e *apiConsoleEngine) Stop() {
	e.stoppingOnce.Do(func() { close(e.stopping) })
	e.mu.Lock()
	e.stopped = true
	cancel := e.cancel
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	e.turnWG.Wait()
	e.stopOnce.Do(func() { close(e.events) })
}

// emit delivers one EngineEvent to the buffered Events channel, abandoning
// the send once Stop has begun so a non-draining consumer can never wedge the
// turn goroutine.
func (e *apiConsoleEngine) emit(ev EngineEvent) {
	select {
	case e.events <- ev:
	case <-e.stopping:
	}
}
