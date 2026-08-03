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

	gotSID, text := n.resolve(&ws.Event{Type: ws.EventDelegateCompleted, Data: map[string]interface{}{
		"caller_session_id": sid, "delegation_id": "wfi-1.abc",
	}})
	if gotSID != sid || !strings.Contains(text, "get_delegation") || !strings.Contains(text, "wfi-1.abc") {
		t.Fatalf("resolve = (%q, %q), want sid + get_delegation hint", gotSID, text)
	}

	n.deliver(sid, text)
	eng := factory.last()
	if len(eng.turns) != 1 || eng.turns[0] != text {
		t.Fatalf("engine turns = %v, want the notification delivered as one turn", eng.turns)
	}

	// A second notification while that turn is still running queues instead
	// of erroring, coalescing until the turn ends.
	n.deliver(sid, "[nrflo] second")
	if sess, _ := svc.get(sid); len(sess.queuedPrompts()) != 1 {
		t.Fatalf("queuedPrompts = %v, want the mid-turn notification queued", sess.queuedPrompts())
	}
}

// Per-worker spawn failures (tier present) are skipped — the fanout-end
// delegate.completed follows and carries the collectable state; only a
// fanout-level failure (no tier) notifies. Unknown sessions are skipped.
func TestChatNotifier_Resolve_Filters(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	n := NewChatNotifier(svc, pool, svc.deps.Clock)

	if sid, _ := n.resolve(&ws.Event{Type: ws.EventDelegateFailed, Data: map[string]interface{}{
		"caller_session_id": "s1", "delegation_id": "d1", "tier": "extractor", "error": "boom",
	}}); sid != "" {
		t.Error("per-worker spawn failure resolved, want skipped")
	}
	if sid, text := n.resolve(&ws.Event{Type: ws.EventDelegateFailed, Data: map[string]interface{}{
		"caller_session_id": "s1", "delegation_id": "d1", "error": "boom",
	}}); sid != "s1" || !strings.Contains(text, "boom") {
		t.Errorf("fanout-level failure resolve = (%q, %q), want s1 + error text", sid, text)
	}
	if sid, _ := n.resolve(&ws.Event{Type: ws.EventPlanWaiting, Data: map[string]interface{}{
		"instance_id": "iid-1", "status": "planning",
	}}); sid != "" {
		t.Error("plan_waiting status=planning resolved, want skipped (only waiting_approval/waiting_input notify)")
	}

	// deliver to a session ChatService does not hold is a silent no-op.
	n.deliver("no-such-session", "[nrflo] hello")
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

	if gotSID, text := n.resolve(&ws.Event{Type: ws.EventOrchestrationCompleted, Data: map[string]interface{}{
		"instance_id": "wfi-console-origin",
	}}); gotSID != sid || !strings.Contains(text, "wfi-console-origin") {
		t.Errorf("resolve(console origin) = (%q, %q), want the launching chat", gotSID, text)
	}
	if gotSID, _ := n.resolve(&ws.Event{Type: ws.EventOrchestrationCompleted, Data: map[string]interface{}{
		"instance_id": "wfi-human-origin",
	}}); gotSID != "" {
		t.Error("resolve(human origin) resolved, want skipped")
	}
	if gotSID, text := n.resolve(&ws.Event{Type: ws.EventPlanWaiting, Data: map[string]interface{}{
		"instance_id": "wfi-console-origin", "status": string(model.WorkflowInstanceWaitingApproval),
	}}); gotSID != sid || !strings.Contains(text, "approve_plan") {
		t.Errorf("resolve(plan waiting_approval) = (%q, %q), want approve_plan hint", gotSID, text)
	}
}
