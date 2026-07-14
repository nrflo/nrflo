package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// codexEngine drives `codex app-server` for a human console session, reusing
// the same appServerClient transport (codex_appserver_client.go) and
// dispatchAppServerEvent mapper (codex_appserver_events.go) as the autonomous
// codexAppServerBackend. Unlike that backend it holds no *processInfo: there
// is no stall heartbeat, nudge loop, or restart cap to opt out of — those
// policies live entirely on processInfo/monitorAll and are simply
// unreachable from an object that never has one. This IS the "console
// engines exempt from autonomous policies" requirement; it is structural,
// not a flag.
//
// appServerArgs()'s --disable delegation flags (multi_agent/multi_agent_v2/
// enable_fanout) stay on: an app-server-spawned child is invisible to nrflo
// whether the parent is a managed session or a human console session, so the
// cc96eed6 rationale still applies here even though the other managed-session
// hardening (bypass flags, --disallowedTools, safety hook) does not — this
// engine defaults to approvalPolicy=on-request/sandbox=workspace-write
// instead of the autonomous never/danger-full-access.
type codexEngine struct {
	sink Sink

	mu         sync.Mutex
	spec       EngineSpec
	cancel     context.CancelFunc
	client     *appServerClient
	profileDir string
	threadID   string
	turnActive bool

	events       chan EngineEvent
	loopDone     chan struct{}
	stopping     chan struct{}
	loopOnce     sync.Once
	stopOnce     sync.Once
	stoppingOnce sync.Once

	approvals *pendingApprovals
}

func newCodexEngine(sink Sink) *codexEngine {
	return &codexEngine{
		sink:      sink,
		events:    make(chan EngineEvent, 256),
		loopDone:  make(chan struct{}),
		stopping:  make(chan struct{}),
		approvals: newPendingApprovals(),
	}
}

func (e *codexEngine) Name() string { return "codex" }

// Start writes the per-session CODEX_HOME profile (same writer the console
// driver uses, WriteConsoleCodexProfile), dials the app-server, and performs
// the initialize/initialized/thread/start handshake before launching the
// event loop. Spawn/handshake failures return an error synchronously — there
// is no goroutine-then-DB-result path here, since a console session has no
// DB-driven completion.
func (e *codexEngine) Start(ctx context.Context, spec EngineSpec) error {
	if spec.Sandbox == "" {
		spec.Sandbox = "workspace-write"
	}
	if spec.ApprovalPolicy == "" {
		spec.ApprovalPolicy = "on-request"
	}

	profileDir, err := os.MkdirTemp("", "nrflo-console-engine-"+spec.SessionID+"-*")
	if err != nil {
		return fmt.Errorf("codex console engine: mkdir profile: %w", err)
	}
	if err := WriteConsoleCodexProfile(profileDir, spec.WorkDir, spec.MCPServerPath, spec.MCPEnv); err != nil {
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("codex console engine: write profile: %w", err)
	}

	env := removeEnvKey(spec.Env, "CODEX_HOME=")
	env = append(env, "CODEX_HOME="+profileDir)
	if !envHasTERM(env) {
		env = append(env, "TERM=xterm-256color")
	}

	runCtx, cancel := context.WithCancel(ctx)
	client, err := dialAppServer(runCtx, env, spec.WorkDir)
	if err != nil {
		cancel()
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("codex console engine: %w", err)
	}

	if _, err := client.call(runCtx, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": appServerClientName, "version": "1"},
	}); err != nil {
		cancel()
		client.close()
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("codex console engine: initialize: %w", err)
	}
	_ = client.notify("initialized", nil)

	resp, err := client.call(runCtx, "thread/start", threadStartParams(spec.Model, spec.WorkDir, spec.Sandbox, spec.ApprovalPolicy))
	if err != nil {
		cancel()
		client.close()
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("codex console engine: thread/start: %w", err)
	}
	var threadResp struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(resp, &threadResp)
	if threadResp.Thread.ID == "" {
		cancel()
		client.close()
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("codex console engine: thread/start: empty thread id")
	}

	e.mu.Lock()
	e.spec = spec
	e.cancel = cancel
	e.client = client
	e.profileDir = profileDir
	e.threadID = threadResp.Thread.ID
	e.mu.Unlock()

	go e.runLoop(runCtx)
	return nil
}

// SendUserTurn issues turn/start and persists the user text through the Sink
// (category "user_input", matching Spawner.RecordUserInput's category).
func (e *codexEngine) SendUserTurn(ctx context.Context, text string) error {
	e.mu.Lock()
	if e.turnActive {
		e.mu.Unlock()
		return ErrTurnActive
	}
	client, threadID, spec := e.client, e.threadID, e.spec
	if client == nil {
		e.mu.Unlock()
		return fmt.Errorf("console engine: not started")
	}
	e.turnActive = true
	e.mu.Unlock()

	// Persist the user row BEFORE issuing turn/start: the agent rows this turn
	// produces are written from runLoop's goroutine, which would otherwise race
	// ahead of this one and land an assistant message before the user message
	// it answers.
	emitMessage(spec.SessionID, text, "user_input", e.sink)

	if _, err := client.call(ctx, "turn/start", turnStartParams(threadID, text, spec.ReasoningEffort, spec.Model)); err != nil {
		e.mu.Lock()
		e.turnActive = false
		e.mu.Unlock()
		return fmt.Errorf("console engine: turn/start: %w", err)
	}
	return nil
}

// Events returns the normalized event channel, closed when the run loop exits.
func (e *codexEngine) Events() <-chan EngineEvent { return e.events }

// Stop cancels the run context, closes the app-server client, waits for the
// run loop to fully exit (so it never sends on a closed channel), removes
// the profile dir, and closes Events exactly once. Safe to call when Start
// never launched the loop (e.g. it failed) or more than once.
//
// `stopping` is closed BEFORE waiting on loopDone: a caller that stops while
// events are still queued (the natural `for ev := range Events() { … Stop() }`
// shape stops draining the moment it calls Stop) would otherwise deadlock —
// the run loop would be parked in a blocking send on a full events buffer,
// never re-entering its select to observe ctx.Done. emit selects on `stopping`
// so the loop can always unwind.
func (e *codexEngine) Stop() {
	e.stoppingOnce.Do(func() { close(e.stopping) })
	e.mu.Lock()
	cancel, client, profileDir := e.cancel, e.client, e.profileDir
	e.mu.Unlock()
	if cancel != nil {
		cancel()
		<-e.loopDone
	}
	if client != nil {
		client.close()
	}
	if profileDir != "" {
		_ = os.RemoveAll(profileDir)
	}
	e.stopOnce.Do(func() { close(e.events) })
}

// emit delivers one EngineEvent to the buffered Events channel, matching the
// EventEmitter signature so it can be passed directly to
// dispatchAppServerEvent. Only called from within runLoop's own goroutine,
// strictly before it closes loopDone/Stop closes the channel, so the send is
// safe. It abandons the send once Stop has begun, so a non-draining consumer
// can never wedge the run loop (see Stop).
func (e *codexEngine) emit(ev EngineEvent) {
	select {
	case e.events <- ev:
	case <-e.stopping:
	}
}

// runLoop consumes notifications and server requests until ctx is cancelled
// or the app-server connection closes. No idle timer, no nudge, no restart
// cap, no rate-limit dance — see the type doc comment.
func (e *codexEngine) runLoop(ctx context.Context) {
	defer e.loopOnce.Do(func() { close(e.loopDone) })
	// Once the loop is gone no turn/completed can ever arrive, so a turn left
	// in flight (connection dropped mid-turn) would otherwise pin turnActive
	// forever and reject every later SendUserTurn with ErrTurnActive.
	defer func() {
		e.mu.Lock()
		e.turnActive = false
		e.mu.Unlock()
	}()

	e.mu.Lock()
	client := e.client
	e.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.closed:
			e.emit(EngineEvent{Type: EventError, SessionID: e.spec.SessionID, Text: "app-server connection closed", IsError: true})
			return
		case req := <-client.reqCh:
			e.onServerRequest(req)
		case n := <-client.notifyCh:
			if n.Method == "serverRequest/resolved" {
				e.onServerRequestResolved(n.Params)
				continue
			}
			sig := dispatchAppServerEvent(e.spec.SessionID, n, e.sink, e.spec.MaxContext, e.emit)
			e.mu.Lock()
			if sig.turnStarted {
				e.turnActive = true
			}
			if sig.turnCompleted {
				e.turnActive = false
			}
			e.mu.Unlock()
		}
	}
}
