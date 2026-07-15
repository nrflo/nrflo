package console

import (
	"context"
	"sync"

	"be/internal/spawner"
)

// fakeConsoleEngine implements spawner.ConsoleEngine entirely in-memory: no
// codex/claude binary, no PTY, no sleeps. It also captures the spawner.Sink
// handed to it via EngineDeps so tests can simulate the Sink-level side
// effects (RecordHookMessage/BroadcastMessagesUpdated) a real engine performs
// internally when it parses an assistant-text hook — those never go through
// Events(), exactly as chat_events.go documents.
type fakeConsoleEngine struct {
	sink spawner.Sink
	// api is the API tool-profile deps ChatService.Create injects
	// unconditionally (EngineDeps.API) — captured here so a test can assert
	// what ChatService built without starting a real api engine.
	api spawner.APIEngineDeps

	events chan spawner.EngineEvent

	mu         sync.Mutex
	startSpec  spawner.EngineSpec
	started    bool
	stopped    bool
	turnActive bool
	turns      []string
	sendErr    error // consumed once by the next SendUserTurn call
	approvals  []fakeApprovalCall
	approveErr error // consumed once by the next ReplyApproval call
}

type fakeApprovalCall struct {
	id       string
	decision spawner.ApprovalDecision
}

func newFakeConsoleEngine(sink spawner.Sink, api spawner.APIEngineDeps) *fakeConsoleEngine {
	return &fakeConsoleEngine{
		sink:   sink,
		api:    api,
		events: make(chan spawner.EngineEvent, 32),
	}
}

// fakeEngineFactory adapts newFakeConsoleEngine to the
// func(name string, deps spawner.EngineDeps) (spawner.ConsoleEngine, error)
// shape ChatService.SetEngineFactory expects, recording every engine it
// constructs (in creation order) so a test can grab the instance behind a
// session it just created via ChatService.Create.
type fakeEngineFactory struct {
	mu      sync.Mutex
	engines []*fakeConsoleEngine
}

func (f *fakeEngineFactory) factory(_ string, deps spawner.EngineDeps) (spawner.ConsoleEngine, error) {
	eng := newFakeConsoleEngine(deps.Sink, deps.API)
	f.mu.Lock()
	f.engines = append(f.engines, eng)
	f.mu.Unlock()
	return eng, nil
}

// last returns the most recently constructed engine (fails the test if none).
func (f *fakeEngineFactory) last() *fakeConsoleEngine {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.engines) == 0 {
		return nil
	}
	return f.engines[len(f.engines)-1]
}

func (f *fakeConsoleEngine) Name() string { return "fake" }

func (f *fakeConsoleEngine) Start(_ context.Context, spec spawner.EngineSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = true
	f.startSpec = spec
	return nil
}

// SendUserTurn records the turn text. sendErr, if set via setSendErr, is
// returned once (then cleared) instead of recording — lets a test simulate a
// transport failure on exactly one call.
func (f *fakeConsoleEngine) SendUserTurn(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		err := f.sendErr
		f.sendErr = nil
		return err
	}
	f.turns = append(f.turns, text)
	f.turnActive = true
	return nil
}

func (f *fakeConsoleEngine) Events() <-chan spawner.EngineEvent { return f.events }

// ReplyApproval mirrors the real engines' contract (console_engine_claude_approval.go,
// console_engine_codex_approval.go): after successfully forwarding the
// decision, the engine itself emits EventApprovalResolved — pumpChatEvents is
// the only thing that resolves the pending approval / pushes
// console_chat.approval_resolved, never ChatService.ReplyApproval directly.
func (f *fakeConsoleEngine) ReplyApproval(id string, decision spawner.ApprovalDecision) error {
	f.mu.Lock()
	if f.approveErr != nil {
		err := f.approveErr
		f.approveErr = nil
		f.mu.Unlock()
		return err
	}
	f.approvals = append(f.approvals, fakeApprovalCall{id: id, decision: decision})
	f.mu.Unlock()
	f.emit(spawner.EngineEvent{Type: spawner.EventApprovalResolved, ApprovalID: id, Decision: decision})
	return nil
}

func (f *fakeConsoleEngine) InterruptTurn(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.turnActive {
		return spawner.ErrNoActiveTurn
	}
	f.turnActive = false
	return nil
}

// Stop closes Events() exactly once, mirroring the real engines' contract
// that pumpChatEvents' range loop terminates on Stop.
func (f *fakeConsoleEngine) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return
	}
	f.stopped = true
	close(f.events)
}

func (f *fakeConsoleEngine) setSendErr(err error) {
	f.mu.Lock()
	f.sendErr = err
	f.mu.Unlock()
}

func (f *fakeConsoleEngine) turnCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.turns)
}

func (f *fakeConsoleEngine) approvalCalls() []fakeApprovalCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeApprovalCall, len(f.approvals))
	copy(out, f.approvals)
	return out
}

func (f *fakeConsoleEngine) isStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}

func (f *fakeConsoleEngine) spec() spawner.EngineSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startSpec
}

// apiDeps returns the EngineDeps.API this engine was constructed with.
func (f *fakeConsoleEngine) apiDeps() spawner.APIEngineDeps {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api
}

// emit pushes ev onto Events() for the pump goroutine to consume.
func (f *fakeConsoleEngine) emit(ev spawner.EngineEvent) {
	f.events <- ev
}

// simulateAssistantText mimics what a real engine does internally when it
// parses an assistant-text hook line: it calls the Sink directly (never
// through Events()) to persist the message and push messages.updated.
func (f *fakeConsoleEngine) simulateAssistantText(sessionID, projectID, text string) {
	_, _, _, _ = f.sink.RecordHookMessage(sessionID, text, "assistant", "")
	f.sink.BroadcastMessagesUpdated(projectID, "", "", sessionID)
}
