package socket

import (
	"context"
	"sync"
)

// fakeConsoleHooks is a scripted ConsoleHooks double: tests configure the
// decision/handled values it returns and inspect the calls it recorded.
type fakeConsoleHooks struct {
	mu sync.Mutex

	approveDecision string
	approveReason   string
	approveHandled  bool
	approveCalls    []approveCall

	turnEndHandled bool
	turnEndCalls   []string

	sessionReadyHandled bool
	sessionReadyCalls   []string

	contextLeftHandled bool
	contextLeftCalls   []contextLeftCall

	userPromptOwn   bool
	userPromptCalls []string

	toolResultCalls []toolResultCall
}

type approveCall struct {
	sessionID, toolName, toolUseID string
	toolInput                      map[string]any
}

type contextLeftCall struct {
	sessionID string
	pct       int
}

func (f *fakeConsoleHooks) ApproveConsoleTool(_ context.Context, sessionID, toolName string, toolInput map[string]any, toolUseID string) (string, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approveCalls = append(f.approveCalls, approveCall{sessionID, toolName, toolUseID, toolInput})
	return f.approveDecision, f.approveReason, f.approveHandled
}

func (f *fakeConsoleHooks) ConsoleTurnEnd(sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turnEndCalls = append(f.turnEndCalls, sessionID)
	return f.turnEndHandled
}

func (f *fakeConsoleHooks) ConsoleSessionReady(sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessionReadyCalls = append(f.sessionReadyCalls, sessionID)
	return f.sessionReadyHandled
}

func (f *fakeConsoleHooks) ConsoleContextLeft(sessionID string, pct int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contextLeftCalls = append(f.contextLeftCalls, contextLeftCall{sessionID, pct})
	return f.contextLeftHandled
}

func (f *fakeConsoleHooks) ConsoleUserPrompt(sessionID, prompt string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userPromptCalls = append(f.userPromptCalls, prompt)
	return f.userPromptOwn
}
