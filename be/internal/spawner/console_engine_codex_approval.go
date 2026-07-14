package spawner

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Codex app-server approval protocol has two generations with DIFFERENT
// decision vocabularies (validated against `codex app-server
// generate-json-schema`, codex-cli 0.144.1):
//
//   - v2 `item/commandExecution/requestApproval` + `item/fileChange/requestApproval`:
//     accept | acceptForSession | decline | cancel. decline = command not
//     executed, turn continues; cancel = also interrupt the turn.
//   - legacy `execCommandApproval` + `applyPatchApproval`:
//     approved | approved_for_session | denied | abort.
//
// approvalDecisionWire maps each approval-shaped server-request method to the
// ApprovalDecision -> wire-string table for that method. Every other server
// request (item/permissions/requestApproval — not decision-shaped, response
// is {permissions,scope,strictAutoReview} — item/tool/requestUserInput,
// mcpServer/elicitation/request, item/tool/call,
// account/chatgptAuthTokens/refresh, attestation/generate, ...) is rejected
// via replyError instead of guessed at.
var approvalDecisionWire = map[string]map[ApprovalDecision]string{
	"item/commandExecution/requestApproval": {
		ApprovalApprove:           "accept",
		ApprovalApproveForSession: "acceptForSession",
		ApprovalDeny:              "decline",
		ApprovalAbort:             "cancel",
	},
	"item/fileChange/requestApproval": {
		ApprovalApprove:           "accept",
		ApprovalApproveForSession: "acceptForSession",
		ApprovalDeny:              "decline",
		ApprovalAbort:             "cancel",
	},
	"execCommandApproval": {
		ApprovalApprove:           "approved",
		ApprovalApproveForSession: "approved_for_session",
		ApprovalDeny:              "denied",
		ApprovalAbort:             "abort",
	},
	"applyPatchApproval": {
		ApprovalApprove:           "approved",
		ApprovalApproveForSession: "approved_for_session",
		ApprovalDeny:              "denied",
		ApprovalAbort:             "abort",
	},
}

// pendingApproval is one outstanding server->client approval request awaiting
// a ReplyApproval call.
type pendingApproval struct {
	rawID  json.RawMessage
	method string
}

// pendingApprovals is a mutex-guarded id->pendingApproval table.
type pendingApprovals struct {
	mu      sync.Mutex
	pending map[string]pendingApproval
}

func newPendingApprovals() *pendingApprovals {
	return &pendingApprovals{pending: make(map[string]pendingApproval)}
}

func (p *pendingApprovals) register(id string, pa pendingApproval) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending[id] = pa
}

// peek looks up a pending approval WITHOUT removing it, so a caller whose
// reply fails validation or transport can retry the same id (see ReplyApproval).
func (p *pendingApprovals) peek(id string) (pendingApproval, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pa, ok := p.pending[id]
	return pa, ok
}

// drop removes a pending approval and reports whether it was still there. The
// bool is what makes resolution idempotent: only the caller that actually
// removed the entry may emit EventApprovalResolved for it.
func (p *pendingApprovals) drop(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.pending[id]
	delete(p.pending, id)
	return ok
}

// approvalRequestParams is the shared shape of the four approval-shaped
// server requests' params.
type approvalRequestParams struct {
	ItemID  string          `json:"itemId"`
	Command json.RawMessage `json:"command"`
	Cwd     string          `json:"cwd"`
	Reason  string          `json:"reason"`
}

// commandText renders an approval request's `command` field as display text.
// Codex versions disagree on shape: a plain string (v2) or an argv array
// (legacy).
func commandText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var argv []string
	if json.Unmarshal(raw, &argv) == nil {
		return strings.Join(argv, " ")
	}
	return string(raw)
}

// onServerRequest handles one server->client request: registers it as
// pending and emits an EventApprovalRequest when the method is one of the
// four approval-shaped requests, else rejects it via replyError so codex is
// never left blocked on a request the engine does not implement.
func (e *codexEngine) onServerRequest(env rpcEnvelope) {
	if env.ID == nil {
		return
	}
	if _, ok := approvalDecisionWire[env.Method]; !ok {
		_ = e.client.replyError(*env.ID, -32601, "console engine: unhandled server request: "+env.Method)
		return
	}
	var p approvalRequestParams
	_ = json.Unmarshal(env.Params, &p)

	id := string(*env.ID)
	e.approvals.register(id, pendingApproval{rawID: *env.ID, method: env.Method})
	e.emit(EngineEvent{
		Type:      EventApprovalRequest,
		SessionID: e.spec.SessionID,
		ItemID:    p.ItemID,
		Approval: &ApprovalRequest{
			ID:      id,
			Kind:    env.Method,
			Command: commandText(p.Command),
			Cwd:     p.Cwd,
			Reason:  p.Reason,
			Raw:     env.Params,
		},
	})
}

// onServerRequestResolved handles the `serverRequest/resolved` notification:
// the server resolved a pending request elsewhere (or it timed out), so drop
// it from the pending table without replying.
//
// Only a drop that actually removed a live entry resolves anything. The server
// also resolves ids we already answered (ReplyApproval emitted the real
// decision then) and ids that were never approvals at all; emitting on those
// would flip an allowed tool's card to "denied — timed out" and audit an
// approval that never existed.
func (e *codexEngine) onServerRequestResolved(params json.RawMessage) {
	var p struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(params, &p) != nil || len(p.ID) == 0 {
		return
	}
	id := string(p.ID)
	if !e.approvals.drop(id) {
		return
	}
	reason := "resolved by app-server (timed out)"
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: e.spec.SessionID, ApprovalID: id, Decision: ApprovalDeny, Text: reason})
}

// ReplyApproval answers a pending approval by id, mapping decision to the
// wire vocabulary for that request's method.
//
// The pending entry is dropped only AFTER the reply is written: an unmappable
// decision (ApprovalDecision is a bare string, so console-8 can hand us a
// user-supplied value) or a transport failure would otherwise consume the id
// while leaving codex blocked on it forever, with no way to retry.
func (e *codexEngine) ReplyApproval(id string, decision ApprovalDecision) error {
	pa, ok := e.approvals.peek(id)
	if !ok {
		return fmt.Errorf("console engine: no pending approval %q", id)
	}
	wire, ok := approvalDecisionWire[pa.method][decision]
	if !ok {
		return fmt.Errorf("console engine: unknown decision %q for method %q", decision, pa.method)
	}
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil {
		return fmt.Errorf("console engine: not started")
	}
	if err := client.reply(pa.rawID, map[string]any{"decision": wire}); err != nil {
		return err
	}
	e.approvals.drop(id)
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: e.spec.SessionID, ApprovalID: id, Decision: decision})
	return nil
}
