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
// approve_for_session also wires to "allow": claude has no native PreToolUse
// equivalent, so the session-scoped memory lives here — ReplyApproval records
// the tool name and RequestApproval auto-allows that tool for the rest of the
// engine's life (coarser than codex's native acceptForSession: it is keyed by
// tool name, so approving one Bash command approves Bash entirely).
var claudeDecisionWire = map[ApprovalDecision]string{
	ApprovalApprove:           "allow",
	ApprovalApproveForSession: "allow",
	ApprovalDeny:              "deny",
	ApprovalAbort:             "deny",
}

// claudeApprovalResult is what ReplyApproval hands back to the RequestApproval
// caller blocked in its select: the already wire-mapped decision plus an
// optional reason.
type claudeApprovalResult struct {
	wire   string
	reason string
}

type pendingClaudeApproval struct {
	reply    chan claudeApprovalResult
	toolName string
}

// claudeApprovals is a mutex-guarded id->pendingClaudeApproval table, mirroring
// codexEngine's pendingApprovals (console_engine_codex_approval.go), plus the
// session-scoped allowlist backing approve_for_session.
type claudeApprovals struct {
	mu      sync.Mutex
	pending map[string]pendingClaudeApproval
	allowed map[string]bool // tool name -> approved for the whole session
}

func newClaudeApprovals() *claudeApprovals {
	return &claudeApprovals{
		pending: make(map[string]pendingClaudeApproval),
		allowed: make(map[string]bool),
	}
}

// allowForSession records toolName as session-approved; subsequent
// RequestApproval calls for it auto-allow without a human round-trip.
func (p *claudeApprovals) allowForSession(toolName string) {
	if toolName == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowed[toolName] = true
}

func (p *claudeApprovals) allowedForSession(toolName string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return toolName != "" && p.allowed[toolName]
}

// listAllowed returns the session-approved tool names, sorted.
func (p *claudeApprovals) listAllowed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return sortedKeys(p.allowed)
}

// revoke removes toolName from the session allowlist (idempotent).
func (p *claudeApprovals) revoke(toolName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.allowed, toolName)
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

// drop removes a pending approval and reports whether it was still there. The
// bool is what makes resolution exactly-once: RequestApproval's non-reply
// branches and ReplyApproval race for the same id, and only the one that
// actually removed it may emit EventApprovalResolved.
func (p *claudeApprovals) drop(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.pending[id]
	delete(p.pending, id)
	return ok
}

// SessionApprovals / RevokeSessionApproval expose the allowlist to the chat
// service (ConsoleEngine interface).
func (e *claudeEngine) SessionApprovals() []string { return e.approvals.listAllowed() }

func (e *claudeEngine) RevokeSessionApproval(tool string) error {
	e.approvals.revoke(tool)
	return nil
}

// SetYolo mutates the mutex-guarded spec.Yolo the RequestApproval
// short-circuit already reads — the toggle is immediate.
func (e *claudeEngine) SetYolo(on bool) error {
	e.mu.Lock()
	e.spec.Yolo = on
	e.mu.Unlock()
	return nil
}

func (e *claudeEngine) Yolo() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spec.Yolo
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
	yolo := e.spec.Yolo
	e.mu.Unlock()

	e.emit(EngineEvent{
		Type:      EventToolInvoke,
		SessionID: sessionID,
		ToolName:  toolName,
		ToolInput: toolInput,
	})

	// A tool the human already approved for the whole session skips the
	// request/reply round-trip entirely — no EventApprovalRequest, so no
	// resolution to emit either. AskUserQuestion never short-circuits: an
	// allow opens an unanswerable TUI picker (console_engine_claude_question.go),
	// so it always goes to the human as a question card, yolo included.
	if toolName != AskUserQuestionTool {
		if e.approvals.allowedForSession(toolName) {
			return "allow", "nrflo: approved for session"
		}
		if yolo {
			return "allow", "nrflo: yolo"
		}
	}

	id := toolUseID
	if id == "" {
		id = fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
	}
	reply := make(chan claudeApprovalResult, 1)
	e.approvals.register(id, pendingClaudeApproval{reply: reply, toolName: toolName})

	raw, _ := json.Marshal(toolInput)
	e.emit(EngineEvent{
		Type:      EventApprovalRequest,
		SessionID: sessionID,
		ToolName:  toolName,
		Approval: &ApprovalRequest{
			ID:      id,
			Kind:    "PreToolUse",
			Tool:    toolName,
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
		return e.denyUnanswered(sessionID, id, "nrflo: approval timed out", reply)
	case <-e.stopping:
		return e.denyUnanswered(sessionID, id, "nrflo: console session stopped", reply)
	case <-ctx.Done():
		return e.denyUnanswered(sessionID, id, "nrflo: approval request cancelled", reply)
	}
}

// denyUnanswered is the shared tail of RequestApproval's three non-reply
// branches. Whoever drops the id owns the resolution: a failed drop means
// ReplyApproval won the race, and since it buffers its result before dropping,
// that decision is already sitting on reply — honor it rather than denying a
// tool the human allowed and emitting a second, contradicting resolution.
func (e *claudeEngine) denyUnanswered(sessionID, id, reason string, reply chan claudeApprovalResult) (string, string) {
	if !e.approvals.drop(id) {
		select {
		case r := <-reply:
			return r.wire, r.reason
		default:
			return "deny", reason
		}
	}
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: sessionID, ApprovalID: id, Decision: ApprovalDeny, Text: reason})
	return "deny", reason
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
		return fmt.Errorf("console engine: decision %q has no claude PreToolUse equivalent", decision)
	}
	if decision == ApprovalApproveForSession && pa.toolName != AskUserQuestionTool {
		e.approvals.allowForSession(pa.toolName)
	}
	reason := ""
	if decision == ApprovalAbort {
		reason = "aborted by user"
	}
	// An allow-shaped decision on a question (a consumer without a question
	// card) must not open the unanswerable picker — redirect the model to
	// plain text instead.
	if pa.toolName == AskUserQuestionTool && wire == "allow" {
		wire, reason = "deny", askUserQuestionRedirect
	}
	select {
	case pa.reply <- claudeApprovalResult{wire: wire, reason: reason}:
	default:
		return fmt.Errorf("console engine: approval %q already answered or timed out", id)
	}
	// Drop-wins (see claudeApprovals.drop): if RequestApproval's timeout/stop
	// branch removed the id first, it already emitted the resolution and this
	// reply lost the race — say so instead of emitting a contradicting one.
	if !e.approvals.drop(id) {
		return fmt.Errorf("console engine: approval %q already resolved", id)
	}
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: e.sessionID(), ApprovalID: id, Decision: decision, Text: reason})
	return nil
}
