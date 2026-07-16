package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_builtin"
)

// apiEngineApprovals is the mutex-guarded pending-approval table plus the
// approve_for_session allowlist — the api-engine analog of claudeApprovals.
type apiEngineApprovals struct {
	mu      sync.Mutex
	pending map[string]pendingAPIApproval
	allowed map[string]bool
}

type pendingAPIApproval struct {
	reply    chan ApprovalDecision
	toolName string
}

func newAPIEngineApprovals() *apiEngineApprovals {
	return &apiEngineApprovals{
		pending: make(map[string]pendingAPIApproval),
		allowed: make(map[string]bool),
	}
}

func (p *apiEngineApprovals) register(id string, reply chan ApprovalDecision, toolName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending[id] = pendingAPIApproval{reply: reply, toolName: toolName}
}

func (p *apiEngineApprovals) peek(id string) (chan ApprovalDecision, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pa, ok := p.pending[id]
	return pa.reply, pa.toolName, ok
}

// drop removes a pending id and reports whether it was still there — the
// exactly-once resolution rule (whoever drops emits EventApprovalResolved).
func (p *apiEngineApprovals) drop(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.pending[id]
	delete(p.pending, id)
	return ok
}

func (p *apiEngineApprovals) allowForSession(toolName string) {
	if toolName == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowed[toolName] = true
}

func (p *apiEngineApprovals) allowedForSession(toolName string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return toolName != "" && p.allowed[toolName]
}

// listAllowed returns the session-approved tool names, sorted.
func (p *apiEngineApprovals) listAllowed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return sortedKeys(p.allowed)
}

// revoke removes toolName from the session allowlist (idempotent).
func (p *apiEngineApprovals) revoke(toolName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.allowed, toolName)
}

// SessionApprovals / RevokeSessionApproval expose the allowlist to the chat
// service (ConsoleEngine interface).
func (e *apiConsoleEngine) SessionApprovals() []string { return e.approvals.listAllowed() }

func (e *apiConsoleEngine) RevokeSessionApproval(tool string) error {
	e.approvals.revoke(tool)
	return nil
}

// consoleAPIFSSystem replaces consoleAPISystem's "no local tools" paragraph
// when the native fs tools are injected (api_native_tools_enabled).
const consoleAPIFSSystem = `You are nrflo's console assistant, reached over a direct API connection with no local CLI. You help the user drive nrflo workflows, inspect projects/tickets, research topics via web_search/web_fetch, and work on files in the session's working directory via read_file, edit_file, and bash (one-shot shell; edit_file/bash require the user's approval). Use the tools available to you to answer the user's requests.`

// withFSTools returns copies of tools/handlers extended with the native fs
// tools (read_file/edit_file/bash), wrapping the mutating ones in the human
// approval gate. The shared console-profile registry is never mutated.
func (e *apiConsoleEngine) withFSTools(tools []provider.ToolSpec, handlers apirun.Registry) ([]provider.ToolSpec, apirun.Registry) {
	outTools := append([]provider.ToolSpec{}, tools...)
	outHandlers := make(apirun.Registry, len(handlers)+3)
	for name, h := range handlers {
		outHandlers[name] = h
	}
	for name, h := range tools_builtin.FSTools() {
		if _, exists := outHandlers[name]; exists {
			continue
		}
		handler := h
		if tools_builtin.FSApprovalRequired(name) {
			handler = approvalGatedHandler{inner: h, engine: e, name: name}
		}
		outHandlers[name] = handler
		outTools = append(outTools, h.Spec())
	}
	return outTools, outHandlers
}

// approvalGatedHandler blocks a mutating fs tool on a human approval
// round-trip before invoking it. A deny is a tool error result, never a Go
// error — the turn continues.
type approvalGatedHandler struct {
	inner  apirun.ToolHandler
	engine *apiConsoleEngine
	name   string
}

func (h approvalGatedHandler) Spec() provider.ToolSpec { return h.inner.Spec() }

func (h approvalGatedHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var detail map[string]interface{}
	_ = json.Unmarshal(input, &detail)
	if !h.engine.requestToolApproval(ctx, h.name, FormatToolDetail(h.name, detail)) {
		return "denied by user", true, nil
	}
	return h.inner.Invoke(ctx, env, input)
}

// requestToolApproval mirrors claudeEngine.RequestApproval's shape: register
// a pending id, emit EventApprovalRequest, block on the human reply /
// timeout / stop / ctx — denying on every non-reply path with a resolution
// owned by whoever drops the id (exactly-once, same as the claude engine).
func (e *apiConsoleEngine) requestToolApproval(ctx context.Context, toolName, detail string) bool {
	if e.approvals.allowedForSession(toolName) {
		return true
	}

	e.mu.Lock()
	sessionID, workDir := e.spec.SessionID, e.spec.WorkDir
	e.mu.Unlock()

	id := fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
	reply := make(chan ApprovalDecision, 1)
	e.approvals.register(id, reply, toolName)

	e.emit(EngineEvent{
		Type:      EventApprovalRequest,
		SessionID: sessionID,
		ToolName:  toolName,
		Approval: &ApprovalRequest{
			ID:      id,
			Kind:    "tool",
			Command: detail,
			Cwd:     workDir,
		},
	})

	timeout := e.approvalTimeout
	if timeout <= 0 {
		timeout = consoleApprovalTimeout
	}

	select {
	case d := <-reply:
		return d == ApprovalApprove || d == ApprovalApproveForSession
	case <-time.After(timeout):
		return e.denyUnansweredTool(sessionID, id, "nrflo: approval timed out", reply)
	case <-ctx.Done():
		return e.denyUnansweredTool(sessionID, id, "nrflo: approval request cancelled", reply)
	case <-e.stopping:
		return e.denyUnansweredTool(sessionID, id, "nrflo: console session stopped", reply)
	}
}

// denyUnansweredTool: drop-wins resolution for the three non-reply branches —
// if ReplyApproval already buffered a decision and dropped the id, honor it.
func (e *apiConsoleEngine) denyUnansweredTool(sessionID, id, reason string, reply chan ApprovalDecision) bool {
	if !e.approvals.drop(id) {
		select {
		case d := <-reply:
			return d == ApprovalApprove || d == ApprovalApproveForSession
		default:
			return false
		}
	}
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: sessionID, ApprovalID: id, Decision: ApprovalDeny, Text: reason})
	return false
}

// ReplyApproval answers a pending fs-tool approval. Drop-after-write, same
// rule as claudeEngine.ReplyApproval: an undeliverable reply must leave the
// id retryable. approve_for_session records the tool in the session
// allowlist before delivery so the auto-allow is visible to the very next
// call.
func (e *apiConsoleEngine) ReplyApproval(id string, decision ApprovalDecision) error {
	reply, toolName, ok := e.approvals.peek(id)
	if !ok {
		return fmt.Errorf("api console engine: no pending approval %q", id)
	}
	switch decision {
	case ApprovalApprove, ApprovalApproveForSession, ApprovalDeny, ApprovalAbort:
	default:
		return fmt.Errorf("api console engine: unknown decision %q", decision)
	}
	if decision == ApprovalApproveForSession {
		e.approvals.allowForSession(toolName)
	}
	select {
	case reply <- decision:
	default:
		return fmt.Errorf("api console engine: approval %q already answered or timed out", id)
	}
	if !e.approvals.drop(id) {
		return fmt.Errorf("api console engine: approval %q already resolved", id)
	}
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: e.currentSessionID(), ApprovalID: id, Decision: decision})
	return nil
}

func (e *apiConsoleEngine) currentSessionID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spec.SessionID
}
