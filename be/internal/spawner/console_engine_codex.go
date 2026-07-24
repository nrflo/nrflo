package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"be/internal/model"
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
// appServerArgs()'s `-c agents.enabled=false` delegation block (codex_delegation.go)
// stays on: an app-server-spawned child is invisible to nrflo whether the
// parent is a managed session or a human console session, so the cc96eed6
// rationale still applies here even though the other managed-session
// hardening (bypass flags, --disallowedTools, safety hook) does not — this
// engine defaults to approvalPolicy=on-request/sandbox=workspace-write
// instead of the autonomous never/danger-full-access.
type codexEngine struct {
	sink Sink

	mu            sync.Mutex
	spec          EngineSpec
	cancel        context.CancelFunc
	client        *appServerClient
	profileDir    string
	threadID      string
	turnID        string
	turnActive    bool
	systemPrompt  string
	seededContext string
	firstTurnSent bool

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

// Start writes the per-session CODEX_HOME profile with
// WriteConsoleCodexProfile, dials the app-server, and performs
// the initialize/initialized/thread/start handshake before launching the
// event loop. Spawn/handshake failures return an error synchronously — there
// is no goroutine-then-DB-result path here, since a console session has no
// DB-driven completion.
func (e *codexEngine) Start(ctx context.Context, spec EngineSpec) error {
	if spec.Sandbox == "" {
		spec.Sandbox = model.SandboxWorkspaceWrite
	}
	if spec.ApprovalPolicy == "" {
		if spec.Yolo {
			spec.ApprovalPolicy = "never"
		} else {
			spec.ApprovalPolicy = "on-request"
		}
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
	e.systemPrompt = spec.SystemPrompt
	e.seededContext = spec.SeededContext
	e.mu.Unlock()

	go e.runLoop(runCtx)
	return nil
}

// SendUserTurn issues turn/start and persists the user text through the Sink
// (category "user_input", matching Spawner.RecordUserInput's category).
// turn.Skill, when set, replaces the provider-visible base text with the
// skill's expanded body (expandSkillTurn) before the first-turn system-prompt
// prefix is applied — codex's side of the Rule 6 seam; the persisted row
// still gets turn.Text unchanged (claude passes it through raw instead,
// console_engine_claude_turn.go).
func (e *codexEngine) SendUserTurn(ctx context.Context, turn UserTurn) error {
	text := turn.Text
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
	e.turnID = ""
	base := text
	if turn.Skill != nil {
		base = expandSkillTurn(turn.Skill)
	}
	turnText := codexFirstTurnText(base, e.systemPrompt, e.seededContext, e.firstTurnSent)
	e.mu.Unlock()

	// Persist the user row (original text, no system-prompt prefix) BEFORE
	// issuing turn/start: the agent rows this turn produces are written from
	// runLoop's goroutine, which would otherwise race ahead of this one and
	// land an assistant message before the user message it answers.
	emitMessage(spec.SessionID, text, "user_input", e.sink)

	resp, err := client.call(ctx, "turn/start", turnStartParams(threadID, turnText, spec.ReasoningEffort, spec.Model))
	if err != nil {
		e.mu.Lock()
		e.turnActive = false
		e.mu.Unlock()
		return fmt.Errorf("console engine: turn/start: %w", err)
	}
	var started struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(resp, &started) != nil || started.Turn.ID == "" {
		e.mu.Lock()
		e.turnActive = false
		e.mu.Unlock()
		return fmt.Errorf("console engine: turn/start: empty turn id")
	}
	e.mu.Lock()
	e.turnID = started.Turn.ID
	// Mark the first turn consumed only after turn/start succeeds: a failed
	// first turn must still prepend the system prompt on retry, since this is
	// codex's only system-prompt delivery channel.
	e.firstTurnSent = true
	e.mu.Unlock()
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
