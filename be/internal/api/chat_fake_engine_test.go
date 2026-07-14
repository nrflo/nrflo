package api

import (
	"context"
	"sync"

	"be/internal/spawner"
)

// fakeConsoleEngine is the api package's own in-memory spawner.ConsoleEngine
// double (test files can't be imported across packages, so console's fake
// isn't reusable here) — no codex/claude binary, no PTY, no sleeps. It
// captures the spawner.Sink handed to it via EngineDeps so a test can
// simulate the Sink-level side effects (RecordHookMessage/
// BroadcastMessagesUpdated) a real engine performs when it parses an
// assistant-text hook line, which never goes through Events().
type fakeConsoleEngine struct {
	sink spawner.Sink

	events chan spawner.EngineEvent

	mu        sync.Mutex
	startSpec spawner.EngineSpec
	stopped   bool
	turns     []string
	approvals []struct {
		id       string
		decision spawner.ApprovalDecision
	}
}

func newFakeConsoleEngine(sink spawner.Sink) *fakeConsoleEngine {
	return &fakeConsoleEngine{sink: sink, events: make(chan spawner.EngineEvent, 32)}
}

// fakeEngineFactory adapts newFakeConsoleEngine to the
// Server.consoleChatEngineFunc seam, recording every engine it constructs so
// a test can grab the instance behind a session it just created via the
// POST /api/v1/console/chats handler.
type fakeEngineFactory struct {
	mu      sync.Mutex
	engines []*fakeConsoleEngine
}

func (f *fakeEngineFactory) factory(_ string, deps spawner.EngineDeps) (spawner.ConsoleEngine, error) {
	eng := newFakeConsoleEngine(deps.Sink)
	f.mu.Lock()
	f.engines = append(f.engines, eng)
	f.mu.Unlock()
	return eng, nil
}

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
	f.startSpec = spec
	f.mu.Unlock()
	return nil
}

func (f *fakeConsoleEngine) SendUserTurn(_ context.Context, text string) error {
	f.mu.Lock()
	f.turns = append(f.turns, text)
	f.mu.Unlock()
	return nil
}

func (f *fakeConsoleEngine) Events() <-chan spawner.EngineEvent { return f.events }

func (f *fakeConsoleEngine) ReplyApproval(id string, decision spawner.ApprovalDecision) error {
	f.mu.Lock()
	f.approvals = append(f.approvals, struct {
		id       string
		decision spawner.ApprovalDecision
	}{id, decision})
	f.mu.Unlock()
	return nil
}

func (f *fakeConsoleEngine) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return
	}
	f.stopped = true
	close(f.events)
}

func (f *fakeConsoleEngine) isStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}

// emit pushes ev onto Events() for chat_events.go's pump goroutine to consume.
func (f *fakeConsoleEngine) emit(ev spawner.EngineEvent) {
	f.events <- ev
}

// simulateAssistantText mimics what a real engine does internally on an
// assistant-text hook line: calling the Sink directly (never through
// Events()) to persist the message and push messages.updated.
func (f *fakeConsoleEngine) simulateAssistantText(sessionID, projectID, text string) {
	_, _, _, _ = f.sink.RecordHookMessage(sessionID, text, "assistant", "")
	f.sink.BroadcastMessagesUpdated(projectID, "", "", sessionID)
}
