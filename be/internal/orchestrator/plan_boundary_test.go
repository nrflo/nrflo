package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// addFanoutTemplate inserts a fanout_template agent definition into
// workflowID, making service.IsPlanDriven true for it. Column defaults
// (model='sonnet-5', execution_mode='cli_interactive', consultant=0) apply.
func addFanoutTemplate(t *testing.T, env *testEnv, workflowID, templateID string) {
	t.Helper()
	now := clock.Real().Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	_, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, node_role, model, created_at, updated_at)
		 VALUES (?, ?, ?, 'fanout_template', 'sonnet-5', ?, ?)`,
		templateID, env.project, workflowID, now, now)
	if err != nil {
		t.Fatalf("insert fanout_template def %s: %v", templateID, err)
	}
}

// buildPlanReloadInputs resolves workflowID's static executable phases (the
// same way runLoop/ContinueWorkflow do) into the (svcWf, workflows, agents)
// triple reloadPlanLayers/materializeAndSplice expect.
func buildPlanReloadInputs(t *testing.T, env *testEnv, workflowID string) (service.SpawnerWorkflowDef, map[string]spawner.WorkflowDef, map[string]spawner.AgentConfig) {
	t.Helper()
	dbWorkflow, dbAgentDefs, _, err := env.orch.resolveWorkflowDef(env.pool, env.project, workflowID)
	if err != nil {
		t.Fatalf("resolveWorkflowDef: %v", err)
	}
	svcWorkflows, svcAgents := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)
	return svcWorkflows[workflowID], convertToSpawnerWorkflows(svcWorkflows), convertToSpawnerAgents(svcAgents)
}

// validManifest builds a minimal single-layer, single-node manifest that
// references templateID and validates cleanly against ValidatePlanManifest.
func validManifest(goal, templateID string) service.PlanManifest {
	return service.PlanManifest{
		Version: 1,
		Goal:    goal,
		Layers: []service.PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []service.PlanNode{
				{ID: "step1", Template: templateID, Instructions: "do the thing"},
			}},
		},
	}
}

// appendDraftPlan appends m as a caller-authored draft revision for wfiID and
// returns the new revision number.
func appendDraftPlan(t *testing.T, env *testEnv, wfiID string, m service.PlanManifest) int {
	t.Helper()
	canonical, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	rev, err := repo.NewPlanRepo(env.pool, clock.Real()).Append(
		wfiID, string(canonical), service.HashManifest(m), model.PlanAuthorCaller, "", m.Goal)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	return rev
}

// TestReloadPlanLayers_NotPlanDriven_NoOp: a workflow with no fanout_template
// def at all is untouched by the plan boundary — reloadPlanLayers must be a
// pure no-op so the caller falls through to normal completion.
func TestReloadPlanLayers_NotPlanDriven_NoOp(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PB-1", "not plan driven")
	wfiID := env.initWorkflow(t, "PB-1")

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	req := RunRequest{ProjectID: env.project, TicketID: "PB-1", WorkflowName: "test", ScopeType: "ticket"}

	newGroups, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, map[int]string{}, workflows, agents)

	if extended || terminal || worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want all false", extended, terminal, worktreeHandled)
	}
	if len(newGroups) != len(layerGroups) {
		t.Fatalf("newGroups len = %d, want %d (unchanged)", len(newGroups), len(layerGroups))
	}
	for i := range layerGroups {
		if newGroups[i].layer != layerGroups[i].layer || len(newGroups[i].phases) != len(layerGroups[i].phases) {
			t.Errorf("newGroups[%d] = %+v, want %+v", i, newGroups[i], layerGroups[i])
		}
	}
}

// TestReloadPlanLayers_NoPlanHead_DraftAttemptFailsWithoutPlannerCLI: DYNWF-6
// changed the "no head" branch from a plain suspend to an inline self-draft
// (draftPlanAndProceed), which first broadcasts+persists 'planning' and then
// BLOCKS this goroutine on a synchronous o.RunPlanner call. RunPlanner (unless
// the workflow carries a workflow-local node_role='planner' override) resolves
// the seeded system_agent_definitions 'planner-system' row, which is
// execution_mode='cli_interactive' — i.e. a real `claude` CLI subprocess, not
// an in-process API call. That must never actually happen in this suite (repo
// rule: never spawn real CLI binaries in tests — a dev machine running these
// tests plausibly HAS a real `claude` binary on PATH). PATH is masked to an
// empty dir so exec.Command's LookPath fails before any process forks,
// RunPlanner returns a fast synchronous spawn error, and draftPlanAndProceed
// takes its "planner error -> markFailed" path — exercising that failure
// mode deterministically and hermetically. (The success variants — draft
// succeeds and suspends at waiting_approval/waiting_input, or mode=auto
// materializes without suspending — need a real or mocked planner backend;
// RunPlanner has no injectable seam for that today, see be_coverage_notes.)
func TestReloadPlanLayers_NoPlanHead_DraftAttemptFailsWithoutPlannerCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // guarantee no `claude`/`codex` binary resolves

	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	env.createTicket(t, "PB-2", "no plan head")
	wfiID := env.initWorkflow(t, "PB-2")

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	req := RunRequest{ProjectID: env.project, TicketID: "PB-2", WorkflowName: "test", ScopeType: "ticket"}

	_, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, map[int]string{}, workflows, agents)

	if extended || !terminal || worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (false, true, false) — markFailed's own path handles cleanup", extended, terminal, worktreeHandled)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("status = %v, want failed (planner spawn error must markFailed, not leave the run stuck at planning)", wi.Status)
	}
}

// TestReloadPlanLayers_DraftPlan_SuspendsWaitingApprovalOrWaitingInput: a
// draft revision with no open questions suspends to 'waiting_approval'; one
// carrying Questions suspends to 'waiting_input' instead.
func TestReloadPlanLayers_DraftPlan_SuspendsWaitingApprovalOrWaitingInput(t *testing.T) {
	cases := []struct {
		name          string
		withQuestions bool
		wantStatus    model.WorkflowInstanceStatus
	}{
		{"no_questions", false, model.WorkflowInstanceWaitingApproval},
		{"with_questions", true, model.WorkflowInstanceWaitingInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			addFanoutTemplate(t, env, "test", "fanout-tmpl")
			ticketID := "PB-3-" + tc.name
			env.createTicket(t, ticketID, "draft plan")
			wfiID := env.initWorkflow(t, ticketID)
			ch := env.subscribeWSClient(t, "ws-"+ticketID, ticketID)

			m := validManifest("do the thing", "fanout-tmpl")
			if tc.withQuestions {
				m.Questions = []service.PlanQuestion{{ID: "q1", Question: "which approach?"}}
			}
			appendDraftPlan(t, env, wfiID, m)

			svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
			layerGroups := groupPhasesByLayer(svcWf.Phases)
			req := RunRequest{ProjectID: env.project, TicketID: ticketID, WorkflowName: "test", ScopeType: "ticket"}

			_, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
				context.Background(), wfiID, req, env.pool, svcWf, layerGroups, map[int]string{}, workflows, agents)

			if extended || !terminal || !worktreeHandled {
				t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (false, true, true)", extended, terminal, worktreeHandled)
			}

			wi := env.getWorkflowInstance(t, wfiID)
			if wi.Status != tc.wantStatus {
				t.Errorf("status = %v, want %v", wi.Status, tc.wantStatus)
			}
			event := expectEvent(t, ch, ws.EventPlanWaiting, 2*time.Second)
			if event.Data["status"] != string(tc.wantStatus) {
				t.Errorf("event status = %v, want %v", event.Data["status"], tc.wantStatus)
			}
		})
	}
}
