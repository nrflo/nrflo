package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// consoleApprovalTimeout is how long RequestApproval blocks waiting for a
// human decision before denying. It is the shortest rung of the timeout
// ladder: server-side wait (600s) < record-event --console deadline (630s)
// < the settings PreToolUse hook `timeout` (660s) — see REFERENCE.md.
const consoleApprovalTimeout = 600 * time.Second

// claudeDecisionWire maps ApprovalDecision to claude's PreToolUse
// permissionDecision wire vocabulary (verified against the installed CLI,
// 2.1.209): approve->allow, deny->deny, abort->deny (with a distinct reason —
// there is no separate "abort the turn" primitive on this hook).
// approve_for_session has no validated claude PreToolUse equivalent and is
// intentionally absent: ReplyApproval returns an error for it and leaves the
// id retryable.
var claudeDecisionWire = map[ApprovalDecision]string{
	ApprovalApprove: "allow",
	ApprovalDeny:    "deny",
	ApprovalAbort:   "deny",
}

// claudeApprovalResult is what ReplyApproval hands back to the RequestApproval
// caller blocked in its select: the already wire-mapped decision plus an
// optional reason.
type claudeApprovalResult struct {
	wire   string
	reason string
}

type pendingClaudeApproval struct {
	reply chan claudeApprovalResult
}

// claudeApprovals is a mutex-guarded id->pendingClaudeApproval table, mirroring
// codexEngine's pendingApprovals (console_engine_codex_approval.go).
type claudeApprovals struct {
	mu      sync.Mutex
	pending map[string]pendingClaudeApproval
}

func newClaudeApprovals() *claudeApprovals {
	return &claudeApprovals{pending: make(map[string]pendingClaudeApproval)}
}

func (p *claudeApprovals) register(id string, pa pendingClaudeApproval) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending[id] = pa
}

// peek looks up a pending approval WITHOUT removing it, so a caller whose
// reply fails validation or delivery can retry the same id.
func (p *claudeApprovals) peek(id string) (pendingClaudeApproval, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pa, ok := p.pending[id]
	return pa, ok
}

func (p *claudeApprovals) drop(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, id)
}

// RequestApproval registers a pending approval, flushes the transcript tail
// (so any assistant text preceding the tool call is persisted/emitted first),
// emits EventToolInvoke + EventApprovalRequest, then blocks on the human
// reply / consoleApprovalTimeout / engine stop / ctx cancellation — denying on
// every non-reply path. Called only from ConsoleHub.ApproveConsoleTool.
func (e *claudeEngine) RequestApproval(ctx context.Context, toolName string, toolInput map[string]any, toolUseID string) (decision, reason string) {
	e.flushTranscript()

	e.mu.Lock()
	sessionID := e.spec.SessionID
	workDir := e.spec.WorkDir
	e.mu.Unlock()

	e.emit(EngineEvent{
		Type:      EventToolInvoke,
		SessionID: sessionID,
		ToolName:  toolName,
		ToolInput: toolInput,
	})

	id := toolUseID
	if id == "" {
		id = fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
	}
	reply := make(chan claudeApprovalResult, 1)
	e.approvals.register(id, pendingClaudeApproval{reply: reply})

	raw, _ := json.Marshal(toolInput)
	e.emit(EngineEvent{
		Type:      EventApprovalRequest,
		SessionID: sessionID,
		ToolName:  toolName,
		Approval: &ApprovalRequest{
			ID:      id,
			Kind:    "PreToolUse",
			Command: FormatToolDetail(toolName, toolInput),
			Cwd:     workDir,
			Raw:     raw,
		},
	})

	timeout := e.approvalTimeout
	if timeout <= 0 {
		timeout = consoleApprovalTimeout
	}

	select {
	case r := <-reply:
		return r.wire, r.reason
	case <-time.After(timeout):
		e.approvals.drop(id)
		return "deny", "nrflo: approval timed out"
	case <-e.stopping:
		e.approvals.drop(id)
		return "deny", "nrflo: console session stopped"
	case <-ctx.Done():
		e.approvals.drop(id)
		return "deny", "nrflo: approval request cancelled"
	}
}

// ReplyApproval answers a pending approval by id, mapping decision to
// claude's PreToolUse wire vocabulary. The pending entry is dropped only
// AFTER the reply is delivered: an unmappable decision or a full/closed reply
// channel would otherwise consume the id while leaving the human-facing
// approval request unanswered forever, with no way to retry (mirrors
// codexEngine.ReplyApproval's drop-after-write rule).
func (e *claudeEngine) ReplyApproval(id string, decision ApprovalDecision) error {
	pa, ok := e.approvals.peek(id)
	if !ok {
		return fmt.Errorf("console engine: no pending approval %q", id)
	}
	wire, ok := claudeDecisionWire[decision]
	if !ok {
		return fmt.Errorf("console engine: decision %q has no claude PreToolUse equivalent (approve_for_session is unsupported)", decision)
	}
	reason := ""
	if decision == ApprovalAbort {
		reason = "aborted by user"
	}
	select {
	case pa.reply <- claudeApprovalResult{wire: wire, reason: reason}:
	default:
		return fmt.Errorf("console engine: approval %q already answered or timed out", id)
	}
	e.approvals.drop(id)
	return nil
}
