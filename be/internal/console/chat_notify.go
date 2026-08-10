package console

import (
	"context"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// ChatNotifier pushes delegation/sub-workflow lifecycle completions into the
// live console chat that launched them, as a queued user turn (SendMessage:
// delivered immediately when the chat is idle, coalesced into the next turn
// otherwise) — so a console-chat agent fires delegate/dynamic_workflow, ends
// its turn, and is woken by the server instead of polling
// workflow_wait/get_delegation. Registered once at server startup
// (Hub.RegisterListener is pre-Run only, like WaitBroker). Sessions the
// ChatService does not hold — mcp-external consoles, workflow agents — are
// silently skipped: they have no server-owned engine to wake and keep the
// polling contract.
type ChatNotifier struct {
	chats *ChatService
	pool  *db.Pool
	clock clock.Clock
}

var _ ws.Listener = (*ChatNotifier)(nil)

// NewChatNotifier creates a ChatNotifier over the server's ChatService.
func NewChatNotifier(chats *ChatService, pool *db.Pool, clk clock.Clock) *ChatNotifier {
	return &ChatNotifier{chats: chats, pool: pool, clock: clk}
}

// chatNotification is one resolved hub event: the target chat plus the turn
// text. A non-empty delegationID marks a delegation-lifecycle notification,
// whose delivery defers until the chat is idle and is dropped when the
// delegation is already consumed — an inline-waited delegate call (extractor
// default wait_sec 120) returned its results in the tool call, so the wake-up
// would only instruct a get_delegation that answers "already consumed".
type chatNotification struct {
	sid          string
	text         string
	delegationID string
}

// OnEvent implements ws.Listener. Delivery runs on its own goroutine — hub
// listeners are invoked sequentially per event, and a claude SendUserTurn
// briefly sleeps between the body write and the submit CR.
func (n *ChatNotifier) OnEvent(e *ws.Event) {
	if e == nil {
		return
	}
	note := n.resolve(e)
	if note.sid == "" || note.text == "" {
		return
	}
	go n.deliver(note)
}

// resolve maps one hub event to a chatNotification; a zero value means not
// notifiable.
func (n *ChatNotifier) resolve(e *ws.Event) chatNotification {
	switch e.Type {
	case ws.EventDelegateCompleted:
		sid, _ := e.Data["caller_session_id"].(string)
		id, _ := e.Data["delegation_id"].(string)
		return chatNotification{sid: sid, delegationID: id,
			text: fmt.Sprintf("[nrflo] Delegation %s finished — collect the results with get_delegation (delegation_id %q).", id, id)}
	case ws.EventDelegateFailed:
		// Per-worker spawn failures carry "tier" and are followed by the
		// fanout-end delegate.completed — only the fanout-level failure (no
		// tier) is terminal without a completed event.
		if _, hasTier := e.Data["tier"]; hasTier {
			return chatNotification{}
		}
		sid, _ := e.Data["caller_session_id"].(string)
		id, _ := e.Data["delegation_id"].(string)
		errMsg, _ := e.Data["error"].(string)
		return chatNotification{sid: sid, delegationID: id,
			text: fmt.Sprintf("[nrflo] Delegation %s failed: %s", id, errMsg)}
	case ws.EventOrchestrationCompleted:
		iid, _ := e.Data["instance_id"].(string)
		sid := n.originChatSession(iid)
		return chatNotification{sid: sid,
			text: fmt.Sprintf("[nrflo] Workflow run %s completed — read its final result with workflow_get or get_subworkflow (instance_id %q).", iid, iid)}
	case ws.EventOrchestrationFailed:
		iid, _ := e.Data["instance_id"].(string)
		sid := n.originChatSession(iid)
		return chatNotification{sid: sid,
			text: fmt.Sprintf("[nrflo] Workflow run %s failed — inspect it with workflow_get (instance_id %q).", iid, iid)}
	case ws.EventPlanWaiting:
		iid, _ := e.Data["instance_id"].(string)
		switch status, _ := e.Data["status"].(string); status {
		case string(model.WorkflowInstanceWaitingApproval):
			return chatNotification{sid: n.originChatSession(iid),
				text: fmt.Sprintf("[nrflo] Run %s parked at waiting_approval — review the plan with get_subworkflow, then approve_plan (or revise_plan first).", iid)}
		case string(model.WorkflowInstanceWaitingInput):
			return chatNotification{sid: n.originChatSession(iid),
				text: fmt.Sprintf("[nrflo] Run %s parked at waiting_input — the planner has questions; answer them via revise_plan, then approve_plan.", iid)}
		}
		return chatNotification{}
	}
	return chatNotification{}
}

// originChatSession resolves iid's origin_session_id when the instance was
// console-launched AND that session is a live chat this service holds; ""
// otherwise. The liveCount guard keeps servers with no open chats from doing
// a DB read per orchestration event.
func (n *ChatNotifier) originChatSession(iid string) string {
	if iid == "" || n.chats.liveCount() == 0 {
		return ""
	}
	wi, err := repo.NewWorkflowInstanceRepo(n.pool, n.clock).Get(iid)
	if err != nil || wi.Origin != model.RunOriginConsole || wi.OriginSessionID == "" {
		return ""
	}
	if _, ok := n.chats.get(wi.OriginSessionID); !ok {
		return ""
	}
	return wi.OriginSessionID
}

// notifyIdleWaitCap bounds how long a delegation notification defers on an
// in-flight turn — likely the very inline delegate wait that will consume the
// delegation; past the cap the consumed-check runs anyway and an unconsumed
// notification queues via SendMessage as before.
const notifyIdleWaitCap = 10 * time.Minute

// notifyIdlePoll paces awaitIdle's turn-state checks.
const notifyIdlePoll = 500 * time.Millisecond

// awaitIdle blocks until the chat has no turn in flight, the session closes,
// or notifyIdleWaitCap elapses.
func (n *ChatNotifier) awaitIdle(sid string) {
	deadline := n.clock.Now().Add(notifyIdleWaitCap)
	for {
		sess, ok := n.chats.get(sid)
		if !ok || sess.guardIdle() == nil || !n.clock.Now().Before(deadline) {
			return
		}
		<-n.clock.After(notifyIdlePoll)
	}
}

// delegationConsumed reports whether the delegation's results were already
// handed back (GetDelegation's read-once terminal read). An unknown id reads
// as not consumed, keeping the pre-check behavior for rows the poller has not
// tracked.
func (n *ChatNotifier) delegationConsumed(id string) bool {
	d, err := repo.NewDelegationRepo(n.pool, n.clock).Get(id)
	return err == nil && d.ConsumedAt != nil
}

// deliver hands the notification to the chat via the normal SendMessage path
// (idle → immediate turn; mid-turn → queued, coalesced with anything else
// that lands before the turn ends). A delegation notification first waits out
// the in-flight turn and is dropped when the delegation was consumed inline.
// A session that closed between resolve and delivery is a silent no-op.
func (n *ChatNotifier) deliver(note chatNotification) {
	if note.delegationID != "" {
		n.awaitIdle(note.sid)
		if n.delegationConsumed(note.delegationID) {
			return
		}
	}
	if _, ok := n.chats.get(note.sid); !ok {
		return
	}
	if _, err := n.chats.SendMessage(note.sid, note.text); err != nil {
		logger.Error(context.Background(), "console chat: notification delivery failed", "session_id", note.sid, "error", err)
	}
}

// liveCount reports how many chat sessions this service currently holds.
func (s *ChatService) liveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}
