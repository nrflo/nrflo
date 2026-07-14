package spawner

import (
	"context"
	"sync"
)

// consoleTarget is the minimal surface a live console engine exposes to the
// hub — kept internal so the socket package (ConsoleHooks) only ever crosses
// the primitives/json boundary already established by TerminalSignaler/
// ToolDispatcher (be/internal/socket/server_interfaces.go).
type consoleTarget interface {
	RequestApproval(ctx context.Context, toolName string, toolInput map[string]any, toolUseID string) (decision, reason string)
	NotifyTurnEnd()
	NotifySessionReady()
	NotifyContextLeft(pct int)
}

// ConsoleHub is a mutex-guarded sessionID -> live console engine registry. It
// is the bridge the socket agent.record_event/agent.context_update handlers
// use to reach an engine (satisfying socket.ConsoleHooks): every method
// returns handled=false for a session with no registered engine, so
// autonomous (non-console) sessions are completely untouched.
type ConsoleHub struct {
	mu      sync.Mutex
	targets map[string]consoleTarget
}

// NewConsoleHub creates an empty hub.
func NewConsoleHub() *ConsoleHub {
	return &ConsoleHub{targets: make(map[string]consoleTarget)}
}

// Register adds a live engine under sessionID. Called from an engine's Start.
func (h *ConsoleHub) Register(sessionID string, target consoleTarget) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.targets[sessionID] = target
}

// Unregister removes sessionID's engine. Called from an engine's Stop.
func (h *ConsoleHub) Unregister(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.targets, sessionID)
}

func (h *ConsoleHub) get(sessionID string) (consoleTarget, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.targets[sessionID]
	return t, ok
}

// ApproveConsoleTool routes a blocking PreToolUse approval to the live engine
// registered for sessionID. handled=false means no live console engine is
// registered for this session — the caller keeps today's autonomous behavior.
func (h *ConsoleHub) ApproveConsoleTool(ctx context.Context, sessionID, toolName string, toolInput map[string]any, toolUseID string) (decision, reason string, handled bool) {
	t, ok := h.get(sessionID)
	if !ok {
		return "", "", false
	}
	decision, reason = t.RequestApproval(ctx, toolName, toolInput, toolUseID)
	return decision, reason, true
}

// ConsoleTurnEnd notifies the live engine (if any) that a Stop hook fired.
func (h *ConsoleHub) ConsoleTurnEnd(sessionID string) (handled bool) {
	t, ok := h.get(sessionID)
	if !ok {
		return false
	}
	t.NotifyTurnEnd()
	return true
}

// ConsoleSessionReady notifies the live engine (if any) that SessionStart fired.
func (h *ConsoleHub) ConsoleSessionReady(sessionID string) (handled bool) {
	t, ok := h.get(sessionID)
	if !ok {
		return false
	}
	t.NotifySessionReady()
	return true
}

// ConsoleContextLeft forwards a context_update to the live engine (if any).
func (h *ConsoleHub) ConsoleContextLeft(sessionID string, pct int) (handled bool) {
	t, ok := h.get(sessionID)
	if !ok {
		return false
	}
	t.NotifyContextLeft(pct)
	return true
}
