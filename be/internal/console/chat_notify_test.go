package console

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// A delegate.completed event for a live chat's session resolves to a
// get_delegation notification and delivers it as a turn on the chat engine.
func TestChatNotifier_DelegateCompleted_DeliversTurn(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	n := NewChatNotifier(svc, pool, svc.deps.Clock)

	note := n.resolve(&ws.Event{Type: ws.EventDelegateCompleted, Data: map[string]interface{}{
		"caller_session_id": sid, "delegation_id": "wfi-1.abc",
	}})
	if note.sid != sid || note.delegationID != "wfi-1.abc" || !strings.Contains(note.text, "get_delegation") {
		t.Fatalf("resolve = %+v, want sid + delegation id + get_delegation hint", note)
	}

	n.deliver(note)
	eng := factory.last()
	if len(eng.turns) != 1 || eng.turns[0] != note.text {
		t.Fatalf("engine turns = %v, want the notification delivered as one turn", eng.turns)
	}

	// A second non-delegation notification while that turn is still running
	// is steered into it (the default fake supports steering, like the
	// claude/api engines); a non-steering engine would queue instead.
	n.deliver(chatNotification{sid: sid, text: "[nrflo] second"})
	if steers := eng.steerTexts(); len(steers) != 1 || steers[0] != "[nrflo] second" {
		t.Fatalf("steerTexts = %v, want the mid-turn notification steered", steers)
	}
}

// A delegation already consumed by an inline delegate wait (extractor default
// wait_sec 120) must not wake the chat: the caller got its results in the
// tool call, and get_delegation could only answer "already consumed".
func TestChatNotifier_ConsumedDelegation_Skipped(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	n := NewChatNotifier(svc, pool, svc.deps.Clock)

	dRepo := repo.NewDelegationRepo(pool, svc.deps.Clock)
	d := &model.Delegation{ID: "wfi-1.consumed", CallerSessionID: sid, WorkflowInstanceID: "wfi-1",
		ProjectID: chatTestProjectID, Tier: "extractor", Brief: "b", Fanout: 1, Depth: 1}
	if err := dRepo.Create(d); err != nil {
		t.Fatalf("Create delegation: %v", err)
	}
	if _, err := dRepo.MarkCompleted(d.ID, "completed"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if _, err := dRepo.MarkConsumed(d.ID); err != nil {
		t.Fatalf("MarkConsumed: %v", err)
	}

	n.deliver(n.resolve(&ws.Event{Type: ws.EventDelegateCompleted, Data: map[string]interface{}{
		"caller_session_id": sid, "delegation_id": d.ID,
	}}))
	if eng := factory.last(); len(eng.turns) != 0 {
		t.Fatalf("engine turns = %v, want consumed-delegation notification dropped", eng.turns)
	}

	// An unconsumed delegation still notifies (the async/executor contract).
	d2 := &model.Delegation{ID: "wfi-1.pending", CallerSessionID: sid, WorkflowInstanceID: "wfi-1",
		ProjectID: chatTestProjectID, Tier: "executor", Brief: "b", Fanout: 1, Depth: 1}
	if err := dRepo.Create(d2); err != nil {
		t.Fatalf("Create delegation: %v", err)
	}
	if _, err := dRepo.MarkCompleted(d2.ID, "completed"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	n.deliver(n.resolve(&ws.Event{Type: ws.EventDelegateCompleted, Data: map[string]interface{}{
		"caller_session_id": sid, "delegation_id": d2.ID,
	}}))
	if eng := factory.last(); len(eng.turns) != 1 {
		t.Fatalf("engine turns = %v, want unconsumed-delegation notification delivered", eng.turns)
	}
}

// Per-worker spawn failures (tier present) are skipped — the fanout-end
// delegate.completed follows and carries the collectable state; only a
// fanout-level failure (no tier) notifies. Unknown sessions are skipped.
func TestChatNotifier_Resolve_Filters(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	n := NewChatNotifier(svc, pool, svc.deps.Clock)

	if note := n.resolve(&ws.Event{Type: ws.EventDelegateFailed, Data: map[string]interface{}{
		"caller_session_id": "s1", "delegation_id": "d1", "tier": "extractor", "error": "boom",
	}}); note.sid != "" {
		t.Error("per-worker spawn failure resolved, want skipped")
	}
	if note := n.resolve(&ws.Event{Type: ws.EventDelegateFailed, Data: map[string]interface{}{
		"caller_session_id": "s1", "delegation_id": "d1", "error": "boom",
	}}); note.sid != "s1" || note.delegationID != "d1" || !strings.Contains(note.text, "boom") {
		t.Errorf("fanout-level failure resolve = %+v, want s1 + delegation id + error text", note)
	}
	if note := n.resolve(&ws.Event{Type: ws.EventPlanWaiting, Data: map[string]interface{}{
		"instance_id": "iid-1", "status": "planning",
	}}); note.sid != "" {
		t.Error("plan_waiting status=planning resolved, want skipped (only waiting_approval/waiting_input notify)")
	}

	// deliver to a session ChatService does not hold is a silent no-op.
	n.deliver(chatNotification{sid: "no-such-session", text: "[nrflo] hello"})
}

// orchestration.completed resolves through the instance's origin: only a
// console-launched instance whose origin session is a live chat notifies.
func TestChatNotifier_OrchestrationCompleted_OriginChat(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	n := NewChatNotifier(svc, pool, svc.deps.Clock)

	now := svc.deps.Clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, is_global, finding_schemas, created_at, updated_at)
		VALUES ('wf-notify', ?, '', 'project', '[]', 0, 0, 0, 0, '{}', ?, ?)`, chatTestProjectID, now, now)

	wiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
	for _, wi := range []*model.WorkflowInstance{
		{ID: "wfi-console-origin", ProjectID: chatTestProjectID, DefProjectID: chatTestProjectID, WorkflowID: "wf-notify", ScopeType: "project", Status: model.WorkflowInstanceCompleted, Origin: model.RunOriginConsole, OriginSessionID: sid},
		{ID: "wfi-human-origin", ProjectID: chatTestProjectID, DefProjectID: chatTestProjectID, WorkflowID: "wf-notify", ScopeType: "project", Status: model.WorkflowInstanceCompleted, Origin: model.RunOriginHuman},
	} {
		if err := wiRepo.Create(wi); err != nil {
			t.Fatalf("Create instance %s: %v", wi.ID, err)
		}
	}

	if note := n.resolve(&ws.Event{Type: ws.EventOrchestrationCompleted, Data: map[string]interface{}{
		"instance_id": "wfi-console-origin",
	}}); note.sid != sid || !strings.Contains(note.text, "wfi-console-origin") {
		t.Errorf("resolve(console origin) = %+v, want the launching chat", note)
	}
	if note := n.resolve(&ws.Event{Type: ws.EventOrchestrationCompleted, Data: map[string]interface{}{
		"instance_id": "wfi-human-origin",
	}}); note.sid != "" {
		t.Error("resolve(human origin) resolved, want skipped")
	}
	if note := n.resolve(&ws.Event{Type: ws.EventPlanWaiting, Data: map[string]interface{}{
		"instance_id": "wfi-console-origin", "status": string(model.WorkflowInstanceWaitingApproval),
	}}); note.sid != sid || !strings.Contains(note.text, "approve_plan") {
		t.Errorf("resolve(plan waiting_approval) = %+v, want approve_plan hint", note)
	}
}
